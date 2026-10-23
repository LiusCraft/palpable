// Package app 是编排层：创建各模块、组装端口，把设备消息分发到对应模块。
// 它只做"收到 X 就调用 Y"，不持有业务状态；出现业务判断说明规则放错了地方。
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/LiusCraft/palpable/server/internal/bridge/linx"
	"github.com/LiusCraft/palpable/server/internal/bridge/mcp"
	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/config"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/gateway"
	"github.com/LiusCraft/palpable/server/internal/history"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/parentapi"
	"github.com/LiusCraft/palpable/server/internal/protocol"
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/share"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/voice"
	"github.com/LiusCraft/palpable/server/internal/world"
	"github.com/LiusCraft/palpable/server/internal/zone"
)

// ai 是 app 用到的 bridge/linx 能力；音频、收音与打断由 voice 经 voice.AI 调用。
type ai interface {
	voice.AI
	Open(ctx context.Context, dev linx.DeviceLink, p linx.Persona) error
	Close(d id.DeviceID)
	Say(ctx context.Context, d id.DeviceID, text string) error
	OnAIState(fn func(d id.DeviceID, s linx.AIState))
	SendMCP(d id.DeviceID, payload json.RawMessage) error
	OnMCP(fn func(d id.DeviceID, payload json.RawMessage))
}

// noAI 在未配置灵矽时顶替 voice.AI：不可用，音频不去 AI。
type noAI struct{}

func (noAI) Forward(id.DeviceID, []byte) {}
func (noAI) Interrupt(id.DeviceID)       {}
func (noAI) Available(id.DeviceID) bool  { return false }
func (noAI) Listen(id.DeviceID, bool)    {}

type App struct {
	demo    config.Demo
	store   store.Store
	catalog *device.Catalog
	devices *device.Devices
	ai      ai // 未配置灵矽时为 nil
	rel     *relation.Relations
	chans   *channel.Channels
	zones   *zone.Zones
	voice   *voice.Router
	mcp     *mcp.Proxy
	history *history.History
	world   *world.World // 仅演示模式
	gw      *gateway.Gateway

	speaking sync.Map // id.DeviceID → bool：AI 是否正在说话，交棒前等它说完
}

var _ gateway.Handler = (*App)(nil)

// deps 是外部依赖；测试用假实现替换。
type deps struct {
	store store.Store
	ai    ai
	linx  linxapi.Client
	demo  config.Demo
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, err
	}
	d := deps{store: db, demo: cfg.Demo}
	if cfg.Linx.Enabled() {
		d.ai = linx.New(linx.Config{URL: cfg.Linx.WSURL, Token: cfg.Linx.Token})
		d.linx = linxapi.New(cfg.Linx.APIBase, cfg.Linx.Token).WithOTA(cfg.Linx.OTAURL)
		slog.Info("linx bridge enabled")
	} else {
		// 不接灵矽时设备档案在本地开通，频道对讲照常可用。
		d.linx = linxapi.NewFake()
		slog.Warn("linx.ws_url not set, AI conversation disabled")
	}
	a, err := newApp(ctx, d)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return a, nil
}

func newApp(ctx context.Context, d deps) (*App, error) {
	rel, err := relation.New(ctx, d.store.Friendships())
	if err != nil {
		return nil, err
	}
	chans, err := channel.New(ctx, d.store.Groups())
	if err != nil {
		return nil, err
	}
	if d.linx == nil {
		d.linx = linxapi.NewFake()
	}
	catalog := device.NewCatalog(d.store.Styles(), d.linx)
	a := &App{demo: d.demo, store: d.store, catalog: catalog, devices: device.New(d.store.Devices(), catalog, d.linx),
		ai: d.ai, rel: rel, chans: chans, zones: zone.New(chans, zone.Park)}
	var aiPort voice.AI = noAI{}
	if d.ai != nil {
		aiPort = d.ai
	}
	a.voice = voice.New(chans, rel, aiPort, a, func(dev id.DeviceID) string { return a.dollName(context.Background(), dev) })
	a.history = history.New(d.store.History(), d.linx, func(ctx context.Context, dev id.DeviceID) (string, error) {
		p, err := a.devices.Profile(ctx, dev)
		return p.AgentID, err
	})
	a.mcp = mcp.New(a.toLinx, a.toDevice, a.hasMCP)
	services := mcp.Services{Devices: a.devices, Zones: a.zones, Channels: chans, Relations: rel}
	if d.ai != nil {
		services.Share = share.New(chans, rel, d.ai, a.history, a.dollName)
	}
	for _, t := range mcp.BusinessTools(services) {
		a.mcp.Register(t)
	}
	if d.ai != nil {
		d.ai.OnAIState(func(dev id.DeviceID, s linx.AIState) {
			a.speaking.Store(dev, s == linx.AISpeaking)
			if s == linx.AISpeaking {
				a.voice.OnAIState(dev, voice.AISpeaking)
			} else {
				a.voice.OnAIState(dev, voice.AIIdle)
			}
		})
		d.ai.OnMCP(a.mcp.OnLinxMCP)
	}
	a.gw = gateway.New(a, allowDevOrigin)
	if d.demo.World {
		if err := a.startWorld(ctx); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// Start 确保灵矽侧的智能体，订阅领域事件并翻译成下行消息，直到 ctx 结束。
func (a *App) Start(ctx context.Context) {
	go a.ensureCatalog(ctx)
	go a.relationEvents(ctx)
	go a.channelEvents(ctx)
	go a.zoneEvents(ctx)
	go a.voice.Run(ctx)
	go a.voiceEvents(ctx)
	go a.deviceEvents(ctx)
	go a.sweepGroups(ctx)
}

func (a *App) Close() error { return a.store.Close() }

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/ws", a.gw)
	if a.world != nil {
		mux.Handle("/world", a.world)
	}
	if a.demo.ParentToken != "" && a.demo.Guardian != "" {
		online := func(d id.DeviceID) bool { _, ok := a.gw.Session(d); return ok }
		mux.Handle("/api/", parentapi.New(parentapi.FixedToken(a.demo.ParentToken, id.GuardianID(a.demo.Guardian)),
			a.devices, a.rel, a.chans, a.history, online))
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// Run 组装并运行 palpabled，直到 ctx 结束。
func Run(ctx context.Context, cfg config.Config) error {
	a, err := New(ctx, cfg)
	if err != nil {
		if ctx.Err() != nil {
			return nil // 启动中途就被要求退出，不算失败
		}
		return err
	}
	defer a.Close()
	a.Start(ctx)

	srv := &http.Server{Addr: cfg.Addr, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("palpabled starting", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// OnConnect 在后台开通设备并接上 AI，不阻塞设备读循环。开通失败时设备仍可进频道对讲。
func (a *App) OnConnect(s *gateway.Session) {
	a.publish(world.Event{Kind: "device", Device: s.DeviceID(), Skin: s.Skin(), Online: world.Bool(true)})
	go a.connect(s)
}

func (a *App) connect(s *gateway.Session) {
	ctx := s.Context()
	d := s.DeviceID()
	p, err := a.devices.Ensure(ctx, device.DeviceInfo{MAC: string(d), Skin: s.Skin(), Firmware: s.Firmware()})
	if err != nil {
		slog.Warn("provision device", "device_id", d, "error", err)
		_ = s.Send(&protocol.Error{Type: protocol.TypeError, Code: protocol.CodeInternal, Message: "provisioning failed"})
		_ = s.Send(&protocol.Light{Type: protocol.TypeLight, State: "muted"})
		return
	}
	p = a.applyDemo(ctx, p)
	a.publish(world.Event{Kind: "device", Device: d, Name: p.DollName, Skin: p.Skin})
	if p.Stage == device.StageNamed {
		// 上次起好名字但交棒没完成：重试，不让孩子重新起名。
		if hp, err := a.devices.Handoff(ctx, d); err != nil {
			slog.Warn("retry handoff", "device_id", d, "error", err)
		} else {
			p = hp
		}
	}
	if a.ai == nil {
		return
	}
	if err := a.ai.Open(ctx, s, persona(p)); err != nil {
		slog.Warn("open ai session", "device_id", d, "error", err)
		return
	}
	if p.Stage == device.StageFresh {
		if err := a.ai.Say(ctx, d, sayWelcome); err != nil {
			slog.Warn("welcome", "device_id", d, "error", err)
		}
	}
}

// applyDemo 演示环境里新设备自动绑定预置家长与同意标记；产品里由家长端完成。
func (a *App) applyDemo(ctx context.Context, p device.Profile) device.Profile {
	if a.demo.Guardian == "" || p.Guardian != "" {
		return p
	}
	g := id.GuardianID(a.demo.Guardian)
	if err := a.devices.Bind(ctx, p.ID, g); err != nil {
		slog.Warn("bind demo guardian", "device_id", p.ID, "error", err)
		return p
	}
	p.Guardian = g
	if a.demo.VoiceConsent {
		if err := a.devices.SetVoiceConsent(ctx, p.ID, true); err == nil {
			p.VoiceConsent = true
		}
	}
	return p
}

// persona 把档案映射成 AI 会话参数；声音登记只在引导期且监护人同意时进行。
func persona(p device.Profile) linx.Persona {
	return linx.Persona{AgentID: p.AgentID, DollName: p.DollName, KidName: p.KidName,
		VoiceStep: p.Stage == device.StageFresh && p.VoiceConsent}
}

// ensureCatalog 确保引导智能体与风格智能体在灵矽存在；失败时稍后重试，期间新设备开通会失败。
func (a *App) ensureCatalog(ctx context.Context) {
	if a.demo.Guardian != "" {
		_ = a.store.Guardians().Put(ctx, store.Guardian{ID: id.GuardianID(a.demo.Guardian), DisplayName: a.demo.GuardianName})
	}
	for delay := 5 * time.Second; ; delay = min(delay*2, 5*time.Minute) {
		err := a.catalog.EnsureAll(ctx)
		if err == nil {
			slog.Info("linx agents ready")
			return
		}
		slog.Error("ensure linx agents", "error", err, "retry_in", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (a *App) OnMessage(s *gateway.Session, msg protocol.Message) {
	switch m := msg.(type) {
	case *protocol.MicState:
		a.voice.OnMicState(s.DeviceID(), m.State == "start")
	case *protocol.Wake:
		a.voice.OnWake(s.DeviceID())
	case *protocol.Proximity:
		peers := make([]relation.Peer, 0, len(m.Peers))
		for _, p := range m.Peers {
			peers = append(peers, relation.Peer{Device: id.DeviceID(p.DeviceID), Strength: p.Strength})
		}
		if err := a.rel.OnProximity(s.Context(), s.DeviceID(), peers); err != nil {
			slog.Warn("proximity", "device_id", s.DeviceID(), "error", err)
		}
	case *protocol.Mcp:
		a.mcp.OnDeviceMCP(s.DeviceID(), m.Payload)
	case *protocol.Location:
		a.publish(world.Event{Kind: "pos", Device: s.DeviceID(), Pos: &world.LatLng{Lat: m.Lat, Lng: m.Lng}})
		if err := a.zones.OnLocation(s.Context(), s.DeviceID(), m.Lat, m.Lng); err != nil {
			slog.Warn("location", "device_id", s.DeviceID(), "error", err)
		}
	default:
		slog.Debug("device message not handled", "device_id", s.DeviceID(), "type", msg.MsgType())
	}
}

func (a *App) OnAudio(s *gateway.Session, frame []byte) {
	a.voice.OnMicFrame(s.DeviceID(), frame)
}

func (a *App) OnDisconnect(s *gateway.Session) {
	a.voice.OnDisconnect(s.DeviceID())
	a.mcp.Forget(s.DeviceID())
	a.speaking.Delete(s.DeviceID())
	if a.ai != nil {
		a.ai.Close(s.DeviceID())
	}
	// 区域频道成员只在线时有效；群成员身份保留。
	_ = a.chans.LeaveZone(context.WithoutCancel(s.Context()), s.DeviceID())
	a.zones.Forget(s.DeviceID())
	a.publish(world.Event{Kind: "device", Device: s.DeviceID(), Online: world.Bool(false)})
}

// allowDevOrigin 开发期只放行本机前端；上线前改为配置白名单。
func allowDevOrigin(r *http.Request) bool {
	switch r.Header.Get("Origin") {
	case "", "http://localhost:5173", "http://127.0.0.1:5173":
		return true
	}
	return false
}

// send 向在线设备下发消息；设备不在线时丢弃，它上线后会从档案与频道状态重新得到所需信息。
func (a *App) send(d id.DeviceID, msgs ...protocol.Message) {
	sess, ok := a.gw.Session(d)
	if !ok {
		return
	}
	for _, m := range msgs {
		if err := sess.Send(m); err != nil {
			return
		}
	}
}

// dollName 返回设备小伙伴的名字，供提示与展示；还没起名时为空。
func (a *App) dollName(ctx context.Context, d id.DeviceID) string {
	p, err := a.devices.Profile(ctx, d)
	if err != nil {
		return ""
	}
	return p.DollName
}

func (a *App) relationEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.rel.Subscribe():
			a.onRelation(ctx, e)
		}
	}
}

func (a *App) onRelation(ctx context.Context, e relation.Event) {
	f := e.Friendship
	a.publish(world.Event{Kind: "friend", Device: f.A, Friend: &f.B, Status: map[relation.EventKind]string{
		relation.Proposed: "pending", relation.Confirmed: "confirmed", relation.Removed: "removed"}[e.Kind]})
	for _, d := range []id.DeviceID{f.A, f.B} {
		other := f.Other(d)
		state := &protocol.FriendState{Type: protocol.TypeFriendState, DeviceID: other, Name: a.dollName(ctx, other)}
		switch e.Kind {
		case relation.Proposed:
			state.Event = "pending"
			a.send(d, state,
				&protocol.Haptic{Type: protocol.TypeHaptic, Pattern: "double_tap"},
				&protocol.Sound{Type: protocol.TypeSound, Name: "friend"})
		case relation.Confirmed:
			state.Event = "confirmed"
			a.send(d, state, &protocol.Haptic{Type: protocol.TypeHaptic, Pattern: "long"})
			a.recordMet(ctx, d, other, "")
		case relation.Removed:
			state.Event = "removed"
			a.send(d, state)
		}
	}
	if e.Kind == relation.Removed {
		a.voice.OnFriendRemoved(f.A, f.B)
	}
}

func (a *App) sweepGroups(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.chans.Sweep(ctx); err != nil {
				slog.Warn("sweep groups", "error", err)
			}
		}
	}
}

func (a *App) channelEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.chans.Subscribe():
			a.onChannel(ctx, e)
		}
	}
}

// onChannel 告诉本设备前台频道的变化，并只让同频道的好友知道它来了或走了。
func (a *App) onChannel(ctx context.Context, e channel.Event) {
	d := id.OwnerDevice(e.Participant)
	ch := e.Channel
	state := &protocol.ChannelState{Type: protocol.TypeChannelState, Channel: string(ch.ID), Name: ch.Name, Color: channelColor(ch)}
	switch e.Kind {
	case channel.Joined:
		a.publish(world.Event{Kind: "member", Device: d, Channel: ch.ID, Joined: world.Bool(true)})
		state.Event = "joined"
		a.send(d, state, &protocol.Sound{Type: protocol.TypeSound, Name: "joined"})
		others := a.rel.FilterFriends(ctx, e.Participant, a.chans.Members(ch.ID))
		for _, f := range others {
			fd := id.OwnerDevice(f)
			a.send(fd, a.presence("join", d, ch))
			a.send(d, a.presence("join", fd, ch)) // 新来的也知道哪些好友已经在
			a.recordMet(ctx, d, fd, string(ch.ID))
			a.recordMet(ctx, fd, d, string(ch.ID))
		}
	case channel.Left:
		a.publish(world.Event{Kind: "member", Device: d, Channel: ch.ID, Joined: world.Bool(false)})
		a.voice.OnLeave(d)
		state.Event = "left"
		a.send(d, state, &protocol.Sound{Type: protocol.TypeSound, Name: "left"})
		for _, f := range a.rel.FilterFriends(ctx, e.Participant, a.chans.Members(ch.ID)) {
			a.send(id.OwnerDevice(f), a.presence("leave", d, ch))
		}
	case channel.Switched:
		a.voice.OnLeave(d)
		state.Event = "switched"
		a.send(d, state, &protocol.Sound{Type: protocol.TypeSound, Name: "switched"})
	}
}

func (a *App) presence(event string, d id.DeviceID, ch channel.Channel) *protocol.Presence {
	return &protocol.Presence{Type: protocol.TypePresence, Event: event, DeviceID: string(d), Name: a.dollName(context.Background(), d), Channel: string(ch.ID)}
}

var groupColors = []string{"#ff9f43", "#54a0ff", "#ff6b9d", "#a55eea", "#26c6da", "#feca57"}

// channelColor 给每个频道一个稳定的灯色：区域为绿色，群按 ID 取色。
func channelColor(ch channel.Channel) string {
	if ch.Kind == channel.KindZone {
		return "#4caf50"
	}
	var h uint32
	for _, r := range ch.ID {
		h = h*31 + uint32(r)
	}
	return groupColors[h%uint32(len(groupColors))]
}

func (a *App) zoneEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.zones.Subscribe():
			a.onZone(ctx, e)
		}
	}
}

// onZone 进入区域时让小伙伴问孩子要不要加入；孩子同意后 AI 调用 self.zone.join。
func (a *App) onZone(ctx context.Context, e zone.Event) {
	if e.Kind != zone.Entered || a.ai == nil || !a.ready(ctx, e.Device) {
		return
	}
	go func() {
		sayCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := a.ai.Say(sayCtx, e.Device, fmt.Sprintf(sayZoneEntered, e.Zone.Name, e.Zone.Name)); err != nil {
			slog.Warn("ask to join zone", "device_id", e.Device, "error", err)
		}
	}()
}

// ready 表示设备已完成起名与交棒；引导期间不打扰孩子问是否加入区域频道。
func (a *App) ready(ctx context.Context, d id.DeviceID) bool {
	p, err := a.devices.Profile(ctx, d)
	return err == nil && p.Stage == device.StageReady
}

// Send 与 SendAudio 实现 voice.Sink：参与者现在都是设备，经 gateway 会话下发。
func (a *App) Send(p id.ParticipantID, m protocol.Message) error {
	sess, ok := a.gw.Session(id.OwnerDevice(p))
	if !ok {
		return nil
	}
	return sess.Send(m)
}

func (a *App) SendAudio(p id.ParticipantID, f []byte) error {
	sess, ok := a.gw.Session(id.OwnerDevice(p))
	if !ok {
		return nil
	}
	return sess.SendAudio(f)
}

func (a *App) voiceEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.voice.Subscribe():
			a.onVoice(e)
		}
	}
}

// onVoice 把谁在说话转给演示地图。
func (a *App) onVoice(e voice.Event) {
	a.publish(world.Event{Kind: "burst", Device: id.OwnerDevice(e.Burst.Speaker), Channel: e.Burst.Channel,
		Speaking: world.Bool(e.Kind == voice.BurstStarted)})
}

func (a *App) toLinx(d id.DeviceID, payload json.RawMessage) error {
	if a.ai == nil {
		return errors.New("ai disabled")
	}
	return a.ai.SendMCP(d, payload)
}

func (a *App) toDevice(d id.DeviceID, payload json.RawMessage) error {
	sess, ok := a.gw.Session(d)
	if !ok {
		return errors.New("device offline")
	}
	return sess.Send(&protocol.Mcp{Type: protocol.TypeMcp, Payload: payload})
}

func (a *App) hasMCP(d id.DeviceID) bool {
	sess, ok := a.gw.Session(d)
	return ok && slices.Contains(sess.Capabilities(), "mcp")
}

func (a *App) deviceEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.devices.Subscribe():
			go a.handoff(e)
		}
	}
}

// handoff 起名后交棒：等小可说完告别 → 切到风格智能体 → 换会话 → 新小伙伴用名字打招呼。
// 失败时设备停在 named，下次上线重试。
func (a *App) handoff(e device.Named) {
	sess, ok := a.gw.Session(e.Device)
	if !ok {
		return
	}
	ctx := sess.Context()
	a.waitAISilent(ctx, e.Device)
	a.publish(world.Event{Kind: "device", Device: e.Device, Name: e.DollName})
	p, err := a.devices.Handoff(ctx, e.Device)
	if err != nil {
		slog.Warn("handoff", "device_id", e.Device, "error", err)
		return
	}
	if a.ai == nil {
		return
	}
	a.ai.Close(e.Device)
	if err := a.ai.Open(ctx, sess, persona(p)); err != nil {
		slog.Warn("reopen ai after handoff", "device_id", e.Device, "error", err)
		return
	}
	if err := a.ai.Say(ctx, e.Device, fmt.Sprintf(sayHandoff, p.KidName, p.DollName)); err != nil {
		slog.Warn("greet after handoff", "device_id", e.Device, "error", err)
	}
}

// waitAISilent 等引导智能体说完最后一句：先给它一点时间开口，再等它说完；最长 20 秒。
func (a *App) waitAISilent(ctx context.Context, d id.DeviceID) {
	speaking := func() bool { v, _ := a.speaking.Load(d); b, _ := v.(bool); return b }
	start := time.Now()
	started := speaking()
	for time.Since(start) < 20*time.Second {
		if s := speaking(); s {
			started = true
		} else if started || time.Since(start) > 4*time.Second {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// recordMet 记一笔"今天遇到了好朋友"，家长端的今日回顾会用到。
func (a *App) recordMet(ctx context.Context, d, friend id.DeviceID, ch string) {
	m := history.MetFriend{Friend: friend, Name: a.dollName(ctx, friend), Channel: ch}
	if err := a.history.RecordMet(ctx, d, m); err != nil {
		slog.Warn("record met friend", "device_id", d, "error", err)
	}
}

// startWorld 开放演示地图：区域轮廓来自 zone，已有的好友关系先装进快照。
func (a *App) startWorld(ctx context.Context) error {
	var zones []world.Zone
	for _, z := range a.zones.All() {
		wz := world.Zone{ID: string(z.ID), Name: z.Name, Channel: string(z.Channel())}
		for _, p := range z.Polygon {
			wz.Polygon = append(wz.Polygon, world.LatLng{Lat: p.Lat, Lng: p.Lng})
		}
		zones = append(zones, wz)
	}
	a.world = world.New(zones, allowDevOrigin)
	for _, st := range []relation.Status{relation.StatusPending, relation.StatusConfirmed} {
		fs, err := a.store.Friendships().ListByStatus(ctx, int(st))
		if err != nil {
			return err
		}
		for _, f := range fs {
			b := f.B
			a.world.Publish(world.Event{Kind: "friend", Device: f.A, Friend: &b, Status: map[relation.Status]string{
				relation.StatusPending: "pending", relation.StatusConfirmed: "confirmed"}[st]})
		}
	}
	slog.Warn("demo world map enabled: /world exposes every device's position")
	return nil
}

func (a *App) publish(e world.Event) {
	if a.world != nil {
		a.world.Publish(e)
	}
}
