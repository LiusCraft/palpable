package factory

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxNoteRunes = 200
	maxBatchSize = 2000
	defaultLimit = 50
	maxLimit     = 200
)

var (
	batchCodeRE = regexp.MustCompile(`^[A-Z0-9]{3,12}$`)
	skinRE      = regexp.MustCompile(`^[a-z]{2,16}$`)
	macRE       = regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)
	orderRE     = regexp.MustCompile(`^[A-Za-z0-9._/\-]{1,40}$`)
	ownerRefRE  = regexp.MustCompile(`^[A-Za-z0-9_\-:.]{8,128}$`)
	channelRE   = regexp.MustCompile(`^[a-z0-9_\-]{2,24}$`)
)

type Actor struct {
	Name string
	Role Role
}

type Options struct {
	NewWarrantyMonths    int // 新品，默认 12
	RefurbWarrantyMonths int // 翻新品，默认 6
	Now                  func() time.Time
}

type Service struct {
	repo Repo
	plat Platform
	opt  Options
}

// NewService 的 plat 可以为 nil：需要擦除、吊销、挂失或登记公钥的动作会返回 ErrNoPlatform，而不是跳过。
func NewService(repo Repo, plat Platform, opt Options) *Service {
	if opt.NewWarrantyMonths <= 0 {
		opt.NewWarrantyMonths = 12
	}
	if opt.RefurbWarrantyMonths <= 0 {
		opt.RefurbWarrantyMonths = 6
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	return &Service{repo: repo, plat: plat, opt: opt}
}

// Params 是动作的输入。
type Params struct {
	Channel    string
	OrderRef   string
	Note       string
	Grade      Grade
	LockReason LockReason
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func (s *Service) checkParams(ctx context.Context, n need, p *Params) error {
	p.Note = strings.TrimSpace(p.Note)
	if utf8.RuneCountInString(p.Note) > maxNoteRunes || !utf8.ValidString(p.Note) {
		return invalid("note must be at most %d characters", maxNoteRunes)
	}
	if n&needNote != 0 && p.Note == "" {
		return invalid("note is required")
	}
	if n&needChannel != 0 {
		if _, err := s.repo.Channel(ctx, p.Channel); errors.Is(err, ErrNotFound) {
			return invalid("unknown channel %q", p.Channel)
		} else if err != nil {
			return fmt.Errorf("check channel: %w", err)
		}
	}
	if n&needOrder != 0 && !orderRE.MatchString(p.OrderRef) {
		return invalid("order_ref must be 1-40 letters, digits or . _ / -")
	}
	if n&needGrade != 0 && p.Grade != GradeA && p.Grade != GradeB {
		return invalid("grade must be A or B")
	}
	if n&needLockReason != 0 {
		switch p.LockReason {
		case LockLost, LockStolen, LockOther:
		default:
			return invalid("lock reason must be lost, stolen or other")
		}
	}
	return nil
}

// Apply 对一台设备执行一个动作：校验角色、状态与输入 → 调 Platform 的副作用 → 在一个事务里落库。
func (s *Service) Apply(ctx context.Context, a Actor, serial string, act Action, p Params) (Unit, error) {
	r, ok := rules[act]
	if !ok {
		return Unit{}, invalid("unknown action %q", act)
	}
	if !a.Role.Can(act) {
		return Unit{}, ErrForbidden
	}
	if err := s.checkParams(ctx, r.need, &p); err != nil {
		return Unit{}, err
	}
	u, err := s.repo.Unit(ctx, serial)
	if err != nil {
		return Unit{}, fmt.Errorf("apply %s to %s: %w", act, serial, err)
	}
	if err := stateCheck(u, act, r); err != nil {
		return Unit{}, fmt.Errorf("apply %s to %s (state %s): %w", act, serial, u.State, err)
	}
	if err := s.sideEffects(ctx, u, act, r); err != nil {
		return Unit{}, fmt.Errorf("apply %s to %s: %w", act, serial, err)
	}
	now := s.opt.Now()
	from := u.State
	s.mutate(&u, act, p, now)
	note := p.Note
	if act == ActLock {
		note = strings.TrimSpace(string(p.LockReason) + " " + p.Note)
	}
	ev := Event{Serial: u.Serial, At: now, Actor: a.Name, Action: string(act), From: from, To: u.State, Note: note}
	u.UpdatedAt = now
	if err := s.repo.SaveUnit(ctx, u, ev); err != nil {
		return Unit{}, fmt.Errorf("apply %s to %s: %w", act, serial, err)
	}
	u.Version++
	return u, nil
}

func stateCheck(u Unit, act Action, r rule) error {
	if !contains(r.from, u.State) {
		return ErrBadTransition
	}
	switch act {
	case ActLock:
		if u.Locked {
			return fmt.Errorf("%w: already locked", ErrBadTransition)
		}
	case ActUnlock:
		if !u.Locked {
			return fmt.Errorf("%w: not locked", ErrBadTransition)
		}
	case ActScrap:
	default:
		if u.Locked {
			return ErrLocked
		}
	}
	return nil
}

func contains(l []State, s State) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

func (s *Service) sideEffects(ctx context.Context, u Unit, act Action, r rule) error {
	if !r.wipe && !r.revoke && act != ActLock && act != ActUnlock {
		return nil
	}
	if s.plat == nil {
		return ErrNoPlatform
	}
	if r.wipe {
		if err := s.plat.Wipe(ctx, u.MAC); err != nil {
			return fmt.Errorf("%w: wipe %s: %w", ErrPlatform, u.MAC, err)
		}
	}
	if r.revoke {
		if err := s.plat.Revoke(ctx, u.MAC); err != nil {
			return fmt.Errorf("%w: revoke %s: %w", ErrPlatform, u.MAC, err)
		}
	}
	if act == ActLock || act == ActUnlock {
		if err := s.plat.SetLock(ctx, u.MAC, act == ActLock); err != nil {
			return fmt.Errorf("%w: set lock %s: %w", ErrPlatform, u.MAC, err)
		}
	}
	return nil
}

func (s *Service) mutate(u *Unit, act Action, p Params, now time.Time) {
	r := rules[act]
	prev := u.State
	if r.to != nil {
		next := r.to(*u)
		if next == "" { // 回到进入前的状态
			next = u.ResumeState
			if !next.Valid() {
				next = Sold
			}
			u.ResumeState = ""
		}
		u.State = next
	}
	switch act {
	case ActShip:
		u.Channel = p.Channel
	case ActUnshelf:
		u.Channel = ""
	case ActSell:
		u.OrderRef = p.OrderRef
		u.SoldAt = now
		months := s.opt.NewWarrantyMonths
		if u.Grade != GradeNew && u.Grade != "" {
			months = s.opt.RefurbWarrantyMonths
		}
		u.WarrantyUntil = now.AddDate(0, months, 0)
	case ActReturnRequest, ActRepair:
		u.ResumeState = prev
	case ActReceiveReturn:
		u.OwnerRef, u.PrevOwnerRef = "", ""
		u.OrderRef, u.Channel = "", ""
		u.SoldAt, u.WarrantyUntil = time.Time{}, time.Time{}
		u.ResumeState = ""
	case ActInspectPass, ActRefurbishDone:
		u.Grade = p.Grade
		u.RefurbCount++
	case ActScrap:
		u.Channel = ""
		u.ResumeState = ""
	case ActLock:
		u.Locked, u.LockReason = true, p.LockReason
	case ActUnlock:
		u.Locked, u.LockReason = false, ""
	}
}

// ---- 批次导入 ----

type UnitKey struct {
	MAC       string
	PublicKey string // base64（标准编码）的 SPKI DER，P-256
}

type BatchInput struct {
	Code, Model, Skin, Firmware, Note string
	Units                             []UnitKey
}

func NormalizeMAC(m string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(m), "-", ":"))
}

func parsePublicKey(b64 string) ([]byte, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, invalid("public key is not valid base64")
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, invalid("public key is not SPKI DER")
	}
	if ek, ok := k.(*ecdsa.PublicKey); !ok || ek.Curve != elliptic.P256() {
		return nil, invalid("public key must be ECDSA P-256")
	}
	return der, nil
}

// ImportBatch 登记一个批次：先把公钥写入 palpabled 的出厂清单（幂等），再一次性入库。
// 序列号 PP<批次>-<5 位序号>。任何一条不合法或重复，整批失败、库里不留任何设备。
func (s *Service) ImportBatch(ctx context.Context, a Actor, in BatchInput) ([]Unit, error) {
	if !a.Role.CanImport() {
		return nil, ErrForbidden
	}
	in.Note = strings.TrimSpace(in.Note)
	switch {
	case !batchCodeRE.MatchString(in.Code):
		return nil, invalid("batch code must be 3-12 uppercase letters or digits")
	case !skinRE.MatchString(in.Skin):
		return nil, invalid("skin must be 2-16 lowercase letters")
	case strings.TrimSpace(in.Model) == "" || utf8.RuneCountInString(in.Model) > 40:
		return nil, invalid("model is required, at most 40 characters")
	case utf8.RuneCountInString(in.Firmware) > 40 || utf8.RuneCountInString(in.Note) > maxNoteRunes:
		return nil, invalid("firmware or note too long")
	case len(in.Units) == 0 || len(in.Units) > maxBatchSize:
		return nil, invalid("a batch must have 1-%d units", maxBatchSize)
	}
	if s.plat == nil {
		return nil, ErrNoPlatform
	}
	now := s.opt.Now()
	units := make([]Unit, 0, len(in.Units))
	events := make([]Event, 0, len(in.Units))
	seen := make(map[string]bool, len(in.Units))
	for i, k := range in.Units {
		mac := NormalizeMAC(k.MAC)
		if !macRE.MatchString(mac) {
			return nil, invalid("unit %d: bad MAC %q", i+1, k.MAC)
		}
		if seen[mac] {
			return nil, invalid("unit %d: duplicate MAC %s", i+1, mac)
		}
		seen[mac] = true
		der, err := parsePublicKey(k.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("unit %d (%s): %w", i+1, mac, err)
		}
		serial := fmt.Sprintf("PP%s-%05d", in.Code, i+1)
		units = append(units, Unit{Serial: serial, MAC: mac, PublicKey: der, BatchCode: in.Code, Skin: in.Skin,
			Firmware: in.Firmware, State: Produced, Grade: GradeNew, CreatedAt: now, UpdatedAt: now})
		events = append(events, Event{Serial: serial, At: now, Actor: a.Name, Action: evImported, To: Produced, Note: in.Code})
	}
	for _, u := range units {
		if err := s.plat.Enroll(ctx, u.MAC, u.PublicKey); err != nil {
			return nil, fmt.Errorf("%w: enroll %s: %w", ErrPlatform, u.MAC, err)
		}
	}
	b := Batch{Code: in.Code, Model: strings.TrimSpace(in.Model), Skin: in.Skin, Firmware: in.Firmware, Note: in.Note, CreatedBy: a.Name, CreatedAt: now}
	if err := s.repo.CreateBatch(ctx, b, units, events); err != nil {
		return nil, fmt.Errorf("create batch %s: %w", in.Code, err)
	}
	return units, nil
}

// ---- palpabled 推来的事件 ----

// OnPlatformEvent 处理绑定与解绑。重复推送是安全的：同一状态下重复的事件不产生新记录。
func (s *Service) OnPlatformEvent(ctx context.Context, ev PlatformEvent) error {
	mac := NormalizeMAC(ev.MAC)
	if !macRE.MatchString(mac) {
		return invalid("bad MAC")
	}
	if ev.Kind != evActivated && ev.Kind != evUnbound {
		return invalid("unknown event %q", ev.Kind)
	}
	if ev.Kind == evActivated && !ownerRefRE.MatchString(ev.OwnerRef) {
		return invalid("bad owner_ref")
	}
	u, err := s.repo.UnitByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("platform event %s %s: %w", ev.Kind, mac, err)
	}
	now := s.opt.Now()
	var evs []Event
	if ev.Kind == evActivated {
		evs = s.onActivated(&u, ev.OwnerRef, now)
	} else {
		evs = s.onUnbound(&u, now)
	}
	if len(evs) == 0 {
		return nil
	}
	u.UpdatedAt = now
	if err := s.repo.SaveUnit(ctx, u, evs...); err != nil {
		return fmt.Errorf("platform event %s %s: %w", ev.Kind, mac, err)
	}
	if u.State == Scrapped && s.plat != nil {
		if err := s.plat.Revoke(ctx, u.MAC); err != nil {
			slog.Warn("re-revoke scrapped unit failed", "serial", u.Serial, "error", err)
		}
	}
	return nil
}

func (s *Service) onActivated(u *Unit, owner string, now time.Time) []Event {
	ev := Event{Serial: u.Serial, At: now, Actor: "palpabled", Action: evActivated, From: u.State, To: u.State}
	switch u.State {
	case Sold, Activated:
		if u.State == Activated && u.OwnerRef == owner {
			return nil
		}
		prev := u.PrevOwnerRef
		if u.State == Activated {
			prev = u.OwnerRef
		}
		var evs []Event
		if prev != "" && prev != owner {
			u.TransferCount++
			evs = append(evs, Event{Serial: u.Serial, At: now, Actor: "palpabled", Action: evTransferred, From: u.State, To: u.State,
				Note: fmt.Sprintf("换主人，第 %d 次", u.TransferCount)})
		}
		u.State, u.OwnerRef = Activated, owner
		ev.To = Activated
		return append(evs, ev)
	case InStock, InTransit, OnShelf, Refurbished:
		u.State, u.OwnerRef = Activated, owner
		ev.To, ev.Anomaly, ev.Note = Activated, true, "未登记售出就被激活"
		return []Event{ev}
	case Returning, Repairing:
		if u.OwnerRef == owner {
			return nil
		}
		u.OwnerRef = owner
		if u.ResumeState == Sold {
			u.ResumeState = Activated
		}
		return []Event{ev}
	case Scrapped:
		ev.Anomaly, ev.Note = true, "已报废的设备被激活"
		return []Event{ev}
	default: // 产线与售后测试：下次擦除会清掉
		ev.Note = "测试绑定，下次擦除时清除"
		return []Event{ev}
	}
}

func (s *Service) onUnbound(u *Unit, now time.Time) []Event {
	if u.OwnerRef == "" {
		return nil
	}
	ev := Event{Serial: u.Serial, At: now, Actor: "palpabled", Action: evUnbound, From: u.State, To: u.State}
	u.PrevOwnerRef, u.OwnerRef = u.OwnerRef, ""
	if u.State == Activated {
		u.State, ev.To = Sold, Sold
	}
	if u.ResumeState == Activated {
		u.ResumeState = Sold
	}
	return []Event{ev}
}

// ---- 查询 ----

func clampLimit(n int) int {
	switch {
	case n <= 0:
		return defaultLimit
	case n > maxLimit:
		return maxLimit
	}
	return n
}

func (s *Service) List(ctx context.Context, f Filter) ([]Unit, int, error) {
	f.Limit = clampLimit(f.Limit)
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.State != "" && !f.State.Valid() {
		return nil, 0, invalid("unknown state %q", f.State)
	}
	return s.repo.ListUnits(ctx, f)
}

func (s *Service) Detail(ctx context.Context, serial string) (Unit, []Event, error) {
	u, err := s.repo.Unit(ctx, serial)
	if err != nil {
		return Unit{}, nil, fmt.Errorf("unit %s: %w", serial, err)
	}
	evs, err := s.repo.Events(ctx, serial)
	if err != nil {
		return Unit{}, nil, fmt.Errorf("events of %s: %w", serial, err)
	}
	return u, evs, nil
}

func (s *Service) Stats(ctx context.Context) (Stats, error) { return s.repo.Stats(ctx) }

func (s *Service) Events(ctx context.Context, f EventFilter) ([]Event, error) {
	f.Limit = clampLimit(f.Limit)
	return s.repo.RecentEvents(ctx, f)
}

func (s *Service) Batches(ctx context.Context) ([]BatchSummary, error) { return s.repo.Batches(ctx) }

func (s *Service) Channels(ctx context.Context) ([]Channel, error) { return s.repo.Channels(ctx) }

// ResolveAnomaly 把一条异常标为已核对。
func (s *Service) ResolveAnomaly(ctx context.Context, a Actor, eventID int64) error {
	if !a.Role.CanResolve() {
		return ErrForbidden
	}
	return s.repo.ResolveEvent(ctx, eventID, a.Name, s.opt.Now())
}

func (s *Service) PutChannel(ctx context.Context, a Actor, c Channel) error {
	if a.Role != RoleAdmin {
		return ErrForbidden
	}
	c.Name = strings.TrimSpace(c.Name)
	if !channelRE.MatchString(c.Code) || c.Name == "" || utf8.RuneCountInString(c.Name) > 40 || !c.Kind.Valid() {
		return invalid("channel needs a code (2-24 of a-z 0-9 _ -), a name and a kind")
	}
	c.CreatedAt = s.opt.Now()
	return s.repo.PutChannel(ctx, c)
}
