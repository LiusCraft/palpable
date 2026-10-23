// Package device 管理设备档案与自动开通：新设备先绑定引导智能体，起名后交棒给按皮套选定的风格智能体，
// 换皮套时切换智能体；记录小伙伴与孩子的名字；在监护人同意后登记孩子的声纹。
package device

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	maxNameRunes = 12
	eventBuffer  = 64
	// 声纹样本要在这个时间窗内，才认为是孩子刚在引导里说的那句话。
	sampleWindow = 10 * time.Minute

	attDollName = "小伙伴的名字"
	attKidName  = "孩子的称呼"
)

var (
	ErrNotFound      = errors.New("device: not found")
	ErrNotNamed      = errors.New("device: not named yet")
	ErrBadName       = errors.New("device: name must be 1-12 characters")
	ErrNoConsent     = errors.New("device: guardian has not consented to voiceprint enrollment")
	ErrNoSample      = errors.New("device: no voice sample from this device")
	ErrNotOnboarding = errors.New("device: voiceprint enrollment only during onboarding")
)

type DeviceInfo struct{ MAC, Skin, Firmware string } // 来自 hello

type Style struct {
	ID           string // "dragon" ...；引导智能体为 "welcome"
	Skin         string // 引导智能体为空
	AgentName    string
	SystemPrompt string
	VoiceID      string
	AgentID      string // EnsureAll 填充
}

type Stage int

const (
	StageFresh Stage = iota
	StageNamed
	StageReady
)

type Profile struct {
	ID           id.DeviceID // = MAC
	Skin         string
	StyleID      string // fresh / named 时为 "welcome"，ready 后为风格
	AgentID      string // 当前绑定的智能体
	DollName     string
	KidName      string
	Guardian     id.GuardianID
	VoiceConsent bool
	SpeakerID    string
	Stage        Stage
	CreatedAt    time.Time
}

type Named struct {
	Device            id.DeviceID
	DollName, KidName string
}

// ---- Catalog ----

type Catalog struct {
	repo store.StyleRepo
	linx linxapi.Client

	mu      sync.RWMutex
	welcome Style
	bySkin  map[string]Style
}

func NewCatalog(repo store.StyleRepo, linx linxapi.Client) *Catalog {
	c := &Catalog{repo: repo, linx: linx, welcome: onboarding, bySkin: map[string]Style{}}
	for _, s := range styles {
		c.bySkin[s.Skin] = s
	}
	return c
}

// EnsureAll 确保引导智能体与每个风格的智能体在灵矽存在，agent_id 存进 style 表避免重复查找。
func (c *Catalog) EnsureAll(ctx context.Context) error {
	all := append([]Style{c.Onboarding()}, styles...)
	for i := range all {
		agent, err := c.ensure(ctx, all[i])
		if err != nil {
			return fmt.Errorf("ensure style %s: %w", all[i].ID, err)
		}
		all[i].AgentID = agent
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.welcome = all[0]
	for _, s := range all[1:] {
		c.bySkin[s.Skin] = s
	}
	return nil
}

func (c *Catalog) ensure(ctx context.Context, s Style) (string, error) {
	mem := linxapi.MemoryLongTerm
	if s.ID == onboarding.ID {
		mem = linxapi.MemoryNone // 引导期的对话不留记忆
	}
	spec := linxapi.AgentSpec{Name: agentName(s), AssistantName: s.AgentName, SystemPrompt: s.SystemPrompt, MemoryModel: mem, VoiceID: s.VoiceID}

	agent := ""
	if rec, err := c.repo.Get(ctx, s.ID); err == nil {
		agent = rec.AgentID
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if agent == "" {
		id, found, err := c.linx.FindAgentByName(ctx, spec.Name)
		if err != nil {
			return "", err
		}
		agent = id
		if !found {
			if agent, err = c.linx.CreateAgent(ctx, spec); err != nil {
				return "", err
			}
			slog.Info("linx agent created", "style", s.ID, "agent_id", agent)
		}
	}
	// 每次启动都把提示词与音色同步到灵矽：代码里改了提示词，已有的智能体也要生效。
	if err := c.linx.UpdateAgent(ctx, agent, spec); err != nil {
		return "", fmt.Errorf("sync agent %s: %w", agent, err)
	}
	if err := c.repo.Put(ctx, store.Style{ID: s.ID, Skin: s.Skin, AgentID: agent}); err != nil {
		return "", err
	}
	return agent, nil
}

func (c *Catalog) Onboarding() Style {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.welcome
}

// ForSkin 未知皮套返回默认风格。
func (c *Catalog) ForSkin(skin string) Style {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if s, ok := c.bySkin[skin]; ok {
		return s
	}
	return c.bySkin[defaultSkin]
}

// ---- Devices ----

type Devices struct {
	repo    store.DeviceRepo
	catalog *Catalog
	linx    linxapi.Client
	now     func() time.Time
	events  chan Named

	mu sync.Mutex // 串行化同一进程里的开通与交棒，避免重复调用灵矽
}

func New(repo store.DeviceRepo, catalog *Catalog, linx linxapi.Client) *Devices {
	return &Devices{repo: repo, catalog: catalog, linx: linx, now: time.Now, events: make(chan Named, eventBuffer)}
}

func (d *Devices) Subscribe() <-chan Named { return d.events }

// Ensure 幂等：没有档案时在灵矽预注册并绑定引导智能体；已就绪的设备换了皮套时切换风格智能体。
func (d *Devices) Ensure(ctx context.Context, info DeviceInfo) (Profile, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	mac := id.DeviceID(strings.ToLower(info.MAC))

	rec, err := d.repo.Get(ctx, mac)
	if errors.Is(err, store.ErrNotFound) {
		return d.provision(ctx, mac, info)
	}
	if err != nil {
		return Profile{}, err
	}
	p := fromRecord(rec)
	if p.Skin == info.Skin || info.Skin == "" {
		return p, nil
	}
	p.Skin = info.Skin
	if p.Stage == StageReady {
		if style := d.catalog.ForSkin(p.Skin); style.ID != p.StyleID {
			if err := d.bindStyle(ctx, &p, style); err != nil {
				return Profile{}, fmt.Errorf("switch skin of %s: %w", mac, err)
			}
		}
	}
	if err := d.repo.Update(ctx, toRecord(p)); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (d *Devices) provision(ctx context.Context, mac id.DeviceID, info DeviceInfo) (Profile, error) {
	welcome := d.catalog.Onboarding()
	if welcome.AgentID == "" {
		return Profile{}, fmt.Errorf("provision %s: onboarding agent not ready", mac)
	}
	err := d.linx.PreregisterDevice(ctx, welcome.AgentID, string(mac))
	if errors.Is(err, linxapi.ErrAlreadyRegistered) {
		// 已在灵矽登记过（比如本地库被清空）：按已开通处理，并把绑定核对到引导智能体。
		err = d.linx.SwitchAgent(ctx, string(mac), welcome.AgentID, false)
	}
	if err != nil {
		return Profile{}, fmt.Errorf("provision %s: %w", mac, err)
	}
	// 预注册只是登记；设备以自己的身份访问一次 OTA 才完成绑定。失败不阻塞开通：可在灵矽控制台手动绑定。
	if err := d.linx.ActivateDevice(ctx, string(mac)); err != nil {
		slog.Warn("activate device via ota", "device_id", mac, "error", err)
	}
	p := Profile{ID: mac, Skin: info.Skin, StyleID: welcome.ID, AgentID: welcome.AgentID, Stage: StageFresh, CreatedAt: d.now()}
	if err := d.repo.Create(ctx, toRecord(p)); err != nil {
		return Profile{}, fmt.Errorf("provision %s: %w", mac, err)
	}
	slog.Info("device provisioned", "device_id", mac, "agent_id", welcome.AgentID)
	return p, nil
}

func (d *Devices) Profile(ctx context.Context, dev id.DeviceID) (Profile, error) {
	rec, err := d.repo.Get(ctx, dev)
	if errors.Is(err, store.ErrNotFound) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	return fromRecord(rec), nil
}

// SetNames 记下名字；在 fresh 时进入 named 并发 Named 事件，之后只更新名字。
func (d *Devices) SetNames(ctx context.Context, dev id.DeviceID, doll, kid string) error {
	doll, kid = strings.TrimSpace(doll), strings.TrimSpace(kid)
	for _, n := range []string{doll, kid} {
		if n == "" || !utf8.ValidString(n) || utf8.RuneCountInString(n) > maxNameRunes {
			return ErrBadName
		}
	}
	d.mu.Lock()
	p, err := d.Profile(ctx, dev)
	if err != nil {
		d.mu.Unlock()
		return err
	}
	p.DollName, p.KidName = doll, kid
	first := p.Stage == StageFresh
	if first {
		p.Stage = StageNamed
	}
	err = d.repo.Update(ctx, toRecord(p))
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if first {
		select {
		case d.events <- Named{Device: dev, DollName: doll, KidName: kid}:
		default:
			slog.Error("device Named event dropped", "device_id", dev)
		}
	} else if p.Stage == StageReady {
		d.writeMemory(ctx, p)
	}
	return nil
}

// Handoff 幂等：把起好名字的设备切到皮套对应的风格智能体，写记忆焦点，进入 ready。
func (d *Devices) Handoff(ctx context.Context, dev id.DeviceID) (Profile, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, err := d.Profile(ctx, dev)
	if err != nil {
		return Profile{}, err
	}
	switch p.Stage {
	case StageReady:
		return p, nil
	case StageFresh:
		return Profile{}, ErrNotNamed
	}
	if err := d.bindStyle(ctx, &p, d.catalog.ForSkin(p.Skin)); err != nil {
		return Profile{}, fmt.Errorf("handoff %s: %w", dev, err)
	}
	p.Stage = StageReady
	if err := d.repo.Update(ctx, toRecord(p)); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (d *Devices) bindStyle(ctx context.Context, p *Profile, style Style) error {
	if style.AgentID == "" {
		return fmt.Errorf("style %s agent not ready", style.ID)
	}
	// 引导期的对话不迁移到风格智能体。
	if err := d.linx.SwitchAgent(ctx, string(p.ID), style.AgentID, false); err != nil {
		return err
	}
	p.StyleID, p.AgentID = style.ID, style.AgentID
	d.writeMemory(ctx, *p)
	return nil
}

// writeMemory 把名字写进风格智能体的记忆焦点；失败只记日志：名字每次会话也会经 agent_params 注入。
func (d *Devices) writeMemory(ctx context.Context, p Profile) {
	if p.DollName == "" || p.AgentID == "" {
		return
	}
	mem, err := d.linx.EnableMemory(ctx, p.AgentID)
	if err != nil {
		slog.Warn("enable memory", "device_id", p.ID, "error", err)
		return
	}
	for _, kv := range [][3]string{
		{attDollName, "小朋友给玩偶小伙伴起的名字", p.DollName},
		{attKidName, "小伙伴称呼小朋友的方式（小名或昵称）", p.KidName},
	} {
		att, err := d.linx.UpsertAttention(ctx, mem, kv[0], kv[1])
		if err == nil {
			err = d.linx.SetAttentionValue(ctx, mem, string(p.ID), att, kv[2])
		}
		if err != nil {
			slog.Warn("write memory focus", "device_id", p.ID, "attention", kv[0], "error", err)
		}
	}
}

// ByGuardian 返回家长名下的设备。
func (d *Devices) ByGuardian(ctx context.Context, g id.GuardianID) ([]Profile, error) {
	recs, err := d.repo.ListByGuardian(ctx, g)
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(recs))
	for _, r := range recs {
		out = append(out, fromRecord(r))
	}
	return out, nil
}

func (d *Devices) Bind(ctx context.Context, dev id.DeviceID, g id.GuardianID) error {
	return d.update(ctx, dev, func(p *Profile) { p.Guardian = g })
}

// SetVoiceConsent 记录监护人对声纹登记的单独同意。
func (d *Devices) SetVoiceConsent(ctx context.Context, dev id.DeviceID, on bool) error {
	return d.update(ctx, dev, func(p *Profile) { p.VoiceConsent = on })
}

// EnrollVoice 只在引导期且监护人同意时，用这台设备刚说的一句话登记声纹；失败不改变 Stage。
// 档案只存 speaker_id，不存音频与声纹特征。
func (d *Devices) EnrollVoice(ctx context.Context, dev id.DeviceID) error {
	p, err := d.Profile(ctx, dev)
	if err != nil {
		return err
	}
	if !p.VoiceConsent {
		return ErrNoConsent
	}
	if p.StyleID != onboarding.ID {
		return ErrNotOnboarding
	}
	samples, err := d.linx.RecentVoiceSamples(ctx, p.AgentID)
	if err != nil {
		return fmt.Errorf("enroll voice %s: %w", dev, err)
	}
	// 引导智能体被所有新设备共用：只用明确标了本机 MAC、且是刚刚录下的样本。
	var audio string
	var at time.Time
	for _, s := range samples {
		if s.MAC == string(dev) && d.now().Sub(s.At) <= sampleWindow && s.At.After(at) {
			audio, at = s.AudioURL, s.At
		}
	}
	if audio == "" {
		return ErrNoSample
	}
	speaker := p.SpeakerID
	if speaker == "" {
		if speaker, err = d.linx.CreateSpeaker(ctx, "可感-"+string(dev)); err != nil {
			return fmt.Errorf("enroll voice %s: %w", dev, err)
		}
	}
	if err := d.linx.EnrollVoiceprint(ctx, speaker, audio); err != nil {
		return fmt.Errorf("enroll voice %s: %w", dev, err)
	}
	return d.update(ctx, dev, func(p *Profile) { p.SpeakerID = speaker })
}

func (d *Devices) update(ctx context.Context, dev id.DeviceID, fn func(*Profile)) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, err := d.Profile(ctx, dev)
	if err != nil {
		return err
	}
	fn(&p)
	return d.repo.Update(ctx, toRecord(p))
}

// Delete 删除设备档案及其好友、群成员与历史（同一事务）。声纹在灵矽侧，由调用方另行清理。
func (d *Devices) Delete(ctx context.Context, dev id.DeviceID) error {
	return d.repo.Delete(ctx, dev)
}

func fromRecord(r store.Device) Profile {
	return Profile{ID: r.ID, Skin: r.Skin, StyleID: r.StyleID, AgentID: r.AgentID, DollName: r.DollName, KidName: r.KidName,
		Guardian: r.Guardian, VoiceConsent: r.VoiceConsent, SpeakerID: r.SpeakerID, Stage: Stage(r.Stage), CreatedAt: r.CreatedAt}
}

func toRecord(p Profile) store.Device {
	return store.Device{ID: p.ID, Skin: p.Skin, StyleID: p.StyleID, AgentID: p.AgentID, DollName: p.DollName, KidName: p.KidName,
		Stage: int(p.Stage), Guardian: p.Guardian, VoiceConsent: p.VoiceConsent, SpeakerID: p.SpeakerID, CreatedAt: p.CreatedAt}
}
