package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
)

// Memory 是给测试用的假实现，行为与 SQLite 一致（同一套契约测试覆盖两者）。
// 一把锁保护全部数据，删除设备时的级联与 SQLite 事务一样是原子的。
type Memory struct {
	mu          sync.Mutex
	devices     map[id.DeviceID]Device
	keys        map[id.DeviceID]DeviceKey
	blocks      map[id.DeviceID]DeviceBlock
	styles      map[string]Style
	guardians   map[id.GuardianID]Guardian
	sessions    map[string]Session
	friendships map[string]Friendship
	groups      map[id.ChannelID]Group
	members     map[id.ChannelID]map[id.ParticipantID]Member
	history     map[string]HistoryEvent
}

var _ Store = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{
		devices:     map[id.DeviceID]Device{},
		keys:        map[id.DeviceID]DeviceKey{},
		blocks:      map[id.DeviceID]DeviceBlock{},
		styles:      map[string]Style{},
		guardians:   map[id.GuardianID]Guardian{},
		sessions:    map[string]Session{},
		friendships: map[string]Friendship{},
		groups:      map[id.ChannelID]Group{},
		members:     map[id.ChannelID]map[id.ParticipantID]Member{},
		history:     map[string]HistoryEvent{},
	}
}

func (m *Memory) Close() error { return nil }

func (m *Memory) Devices() DeviceRepo           { return memDevices{m} }
func (m *Memory) DeviceKeys() DeviceKeyRepo     { return memDeviceKeys{m} }
func (m *Memory) DeviceBlocks() DeviceBlockRepo { return memDeviceBlocks{m} }
func (m *Memory) Styles() StyleRepo             { return memStyles{m} }
func (m *Memory) Guardians() GuardianRepo       { return memGuardians{m} }
func (m *Memory) Sessions() SessionRepo         { return memSessions{m} }
func (m *Memory) Friendships() FriendshipRepo   { return memFriendships{m} }
func (m *Memory) Groups() GroupRepo             { return memGroups{m} }
func (m *Memory) History() HistoryRepo          { return memHistory{m} }

// ---- device ----

type memDevices struct{ m *Memory }

func (r memDevices) Get(_ context.Context, d id.DeviceID) (Device, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	dev, ok := r.m.devices[d]
	if !ok {
		return Device{}, fmt.Errorf("get device %s: %w", d, ErrNotFound)
	}
	dev.BlockedAgents = slices.Clone(dev.BlockedAgents)
	return dev, nil
}

func (r memDevices) Create(_ context.Context, d Device) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.devices[d.ID]; ok {
		return fmt.Errorf("create device %s: %w", d.ID, ErrConflict)
	}
	d.CreatedAt = roundMillis(d.CreatedAt)
	d.BlockedAgents = slices.Clone(d.BlockedAgents)
	r.m.devices[d.ID] = d
	return nil
}

func (r memDevices) Update(_ context.Context, d Device) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	old, ok := r.m.devices[d.ID]
	if !ok {
		return fmt.Errorf("update device %s: %w", d.ID, ErrNotFound)
	}
	d.CreatedAt = old.CreatedAt
	d.BlockedAgents = slices.Clone(d.BlockedAgents)
	r.m.devices[d.ID] = d
	return nil
}

func (r memDevices) ListByGuardian(_ context.Context, g id.GuardianID) ([]Device, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	var out []Device
	for _, d := range r.m.devices {
		if d.Guardian == g {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b Device) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(string(a.ID), string(b.ID))
	})
	return out, nil
}

func (r memDevices) Delete(_ context.Context, d id.DeviceID) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.devices[d]; !ok {
		return fmt.Errorf("delete device %s: %w", d, ErrNotFound)
	}
	delete(r.m.devices, d)
	for k, f := range r.m.friendships {
		if f.A == d || f.B == d {
			delete(r.m.friendships, k)
		}
	}
	for _, ms := range r.m.members {
		delete(ms, id.Device(d))
	}
	for k, e := range r.m.history {
		if e.Device == d {
			delete(r.m.history, k)
		}
	}
	return nil
}

// ---- style ----

type memStyles struct{ m *Memory }

func (r memStyles) Get(_ context.Context, styleID string) (Style, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	s, ok := r.m.styles[styleID]
	if !ok {
		return Style{}, fmt.Errorf("get style %s: %w", styleID, ErrNotFound)
	}
	return s, nil
}

func (r memStyles) Put(_ context.Context, s Style) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	r.m.styles[s.ID] = s
	return nil
}

func (r memStyles) List(_ context.Context) ([]Style, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := sortedValues(r.m.styles, func(a, b Style) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// ---- guardian ----

type memGuardians struct{ m *Memory }

func (r memGuardians) Get(_ context.Context, g id.GuardianID) (Guardian, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out, ok := r.m.guardians[g]
	if !ok {
		return Guardian{}, fmt.Errorf("get guardian %s: %w", g, ErrNotFound)
	}
	return out, nil
}

func (r memGuardians) ByWxOpenID(_ context.Context, openid string) (Guardian, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if g, ok := r.m.guardianByOpenID(openid); ok {
		return g, nil
	}
	return Guardian{}, fmt.Errorf("guardian by openid: %w", ErrNotFound)
}

func (m *Memory) guardianByOpenID(openid string) (Guardian, bool) {
	if openid == "" {
		return Guardian{}, false
	}
	for _, g := range m.guardians {
		if g.WxOpenID == openid {
			return g, true
		}
	}
	return Guardian{}, false
}

func (r memGuardians) Create(_ context.Context, g Guardian) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.guardians[g.ID]; ok {
		return fmt.Errorf("create guardian %s: %w", g.ID, ErrConflict)
	}
	if _, ok := r.m.guardianByOpenID(g.WxOpenID); ok {
		return fmt.Errorf("create guardian %s: %w", g.ID, ErrConflict)
	}
	g.CreatedAt = fromMillis(toMillis(g.CreatedAt))
	r.m.guardians[g.ID] = g
	return nil
}

func (r memGuardians) Put(_ context.Context, g Guardian) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if other, ok := r.m.guardianByOpenID(g.WxOpenID); ok && other.ID != g.ID {
		return fmt.Errorf("put guardian %s: %w", g.ID, ErrConflict)
	}
	if old, ok := r.m.guardians[g.ID]; ok {
		g.CreatedAt = old.CreatedAt
	} else {
		g.CreatedAt = fromMillis(toMillis(g.CreatedAt))
	}
	r.m.guardians[g.ID] = g
	return nil
}

func (r memGuardians) Delete(_ context.Context, g id.GuardianID) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.guardians[g]; !ok {
		return fmt.Errorf("delete guardian %s: %w", g, ErrNotFound)
	}
	delete(r.m.guardians, g)
	for h, s := range r.m.sessions {
		if s.Guardian == g {
			delete(r.m.sessions, h)
		}
	}
	return nil
}

// ---- session ----

type memSessions struct{ m *Memory }

func (r memSessions) Create(_ context.Context, s Session) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.sessions[s.TokenHash]; ok {
		return fmt.Errorf("create session: %w", ErrConflict)
	}
	s.CreatedAt, s.ExpiresAt = fromMillis(toMillis(s.CreatedAt)), fromMillis(toMillis(s.ExpiresAt))
	r.m.sessions[s.TokenHash] = s
	return nil
}

func (r memSessions) Get(_ context.Context, tokenHash string) (Session, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	s, ok := r.m.sessions[tokenHash]
	if !ok {
		return Session{}, fmt.Errorf("get session: %w", ErrNotFound)
	}
	return s, nil
}

func (r memSessions) Delete(_ context.Context, tokenHash string) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.sessions[tokenHash]; !ok {
		return fmt.Errorf("delete session: %w", ErrNotFound)
	}
	delete(r.m.sessions, tokenHash)
	return nil
}

func (r memSessions) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	n := 0
	for h, s := range r.m.sessions {
		if !s.ExpiresAt.After(now) {
			delete(r.m.sessions, h)
			n++
		}
	}
	return n, nil
}

// ---- friendship ----

type memFriendships struct{ m *Memory }

func byCreated(a, b Friendship) int {
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

func (r memFriendships) Get(_ context.Context, friendshipID string) (Friendship, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	f, ok := r.m.friendships[friendshipID]
	if !ok {
		return Friendship{}, fmt.Errorf("get friendship %s: %w", friendshipID, ErrNotFound)
	}
	return f, nil
}

func (r memFriendships) ByPair(_ context.Context, a, b id.DeviceID) (Friendship, error) {
	a, b = orderPair(a, b)
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for _, f := range r.m.friendships {
		if f.A == a && f.B == b {
			return f, nil
		}
	}
	return Friendship{}, fmt.Errorf("get friendship %s/%s: %w", a, b, ErrNotFound)
}

func (r memFriendships) Create(_ context.Context, f Friendship) error {
	if f.A == f.B {
		return fmt.Errorf("create friendship %s: %w", f.ID, errSelfFriendship)
	}
	f.normalize()
	f.CreatedAt, f.UpdatedAt = roundMillis(f.CreatedAt), roundMillis(f.UpdatedAt)
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for _, old := range r.m.friendships {
		if old.ID == f.ID || (old.A == f.A && old.B == f.B) {
			return fmt.Errorf("create friendship %s: %w", f.ID, ErrConflict)
		}
	}
	r.m.friendships[f.ID] = f
	return nil
}

func (r memFriendships) Update(_ context.Context, f Friendship) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	old, ok := r.m.friendships[f.ID]
	if !ok {
		return fmt.Errorf("update friendship %s: %w", f.ID, ErrNotFound)
	}
	old.Status, old.ConfirmedBy, old.UpdatedAt = f.Status, f.ConfirmedBy, roundMillis(f.UpdatedAt)
	old.ApprovedA, old.ApprovedB = f.ApprovedA, f.ApprovedB
	if f.A != old.A {
		old.ApprovedA, old.ApprovedB = f.ApprovedB, f.ApprovedA
	}
	r.m.friendships[f.ID] = old
	return nil
}

func (r memFriendships) ListByDevice(_ context.Context, d id.DeviceID) ([]Friendship, error) {
	return r.list(func(f Friendship) bool { return f.A == d || f.B == d }), nil
}

func (r memFriendships) ListByStatus(_ context.Context, status int) ([]Friendship, error) {
	return r.list(func(f Friendship) bool { return f.Status == status }), nil
}

func (r memFriendships) list(keep func(Friendship) bool) []Friendship {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	var out []Friendship
	for _, f := range r.m.friendships {
		if keep(f) {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, byCreated)
	return out
}

// ---- group ----

type memGroups struct{ m *Memory }

func byChannelID(a, b Group) int { return strings.Compare(string(a.ID), string(b.ID)) }

func (r memGroups) Create(_ context.Context, g Group) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.groups[g.ID]; ok {
		return fmt.Errorf("create group %s: %w", g.ID, ErrConflict)
	}
	g.ExpiresAt = roundMillis(g.ExpiresAt)
	r.m.groups[g.ID] = g
	return nil
}

func (r memGroups) Get(_ context.Context, ch id.ChannelID) (Group, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	g, ok := r.m.groups[ch]
	if !ok {
		return Group{}, fmt.Errorf("get group %s: %w", ch, ErrNotFound)
	}
	return g, nil
}

func (r memGroups) ByCode(_ context.Context, code string, now time.Time) (Group, error) {
	now = roundMillis(now)
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	var hits []Group
	for _, g := range r.m.groups {
		if g.Code == code && g.Active(now) {
			hits = append(hits, g)
		}
	}
	if len(hits) == 0 {
		return Group{}, fmt.Errorf("get group by code: %w", ErrNotFound)
	}
	return slices.MinFunc(hits, byChannelID), nil
}

func (r memGroups) List(_ context.Context) ([]Group, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return sortedValues(r.m.groups, byChannelID), nil
}

func (r memGroups) Delete(_ context.Context, ch id.ChannelID) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.groups[ch]; !ok {
		return fmt.Errorf("delete group %s: %w", ch, ErrNotFound)
	}
	delete(r.m.groups, ch)
	delete(r.m.members, ch)
	return nil
}

func (r memGroups) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	now = roundMillis(now)
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	n := 0
	for ch, g := range r.m.groups {
		if !g.Active(now) {
			delete(r.m.groups, ch)
			delete(r.m.members, ch)
			n++
		}
	}
	return n, nil
}

func (r memGroups) AddMember(_ context.Context, ch id.ChannelID, m Member) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.groups[ch]; !ok {
		return fmt.Errorf("add %s to group %s: %w", m.Participant, ch, ErrNotFound)
	}
	ms := r.m.members[ch]
	if ms == nil {
		ms = map[id.ParticipantID]Member{}
		r.m.members[ch] = ms
	}
	if _, ok := ms[m.Participant]; !ok {
		m.JoinedAt = roundMillis(m.JoinedAt)
		ms[m.Participant] = m
	}
	return nil
}

func (r memGroups) RemoveMember(_ context.Context, ch id.ChannelID, p id.ParticipantID) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	delete(r.m.members[ch], p)
	return nil
}

func (r memGroups) Members(_ context.Context, ch id.ChannelID) ([]Member, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return sortedValues(r.m.members[ch], func(a, b Member) int {
		if c := a.JoinedAt.Compare(b.JoinedAt); c != 0 {
			return c
		}
		return strings.Compare(string(a.Participant), string(b.Participant))
	}), nil
}

func (r memGroups) ListByMember(_ context.Context, p id.ParticipantID) ([]Group, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	type joined struct {
		g  Group
		at time.Time
	}
	var hits []joined
	for ch, ms := range r.m.members {
		if m, ok := ms[p]; ok {
			hits = append(hits, joined{r.m.groups[ch], m.JoinedAt})
		}
	}
	slices.SortFunc(hits, func(a, b joined) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}
		return byChannelID(a.g, b.g)
	})
	var out []Group
	for _, h := range hits {
		out = append(out, h.g)
	}
	return out, nil
}

// ---- history ----

type memHistory struct{ m *Memory }

func (r memHistory) Append(_ context.Context, e HistoryEvent) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.history[e.ID]; ok {
		return fmt.Errorf("append history %s: %w", e.ID, ErrConflict)
	}
	if len(e.Payload) == 0 {
		e.Payload = []byte("{}")
	} else {
		e.Payload = slices.Clone(e.Payload)
	}
	e.At = roundMillis(e.At)
	r.m.history[e.ID] = e
	return nil
}

func (r memHistory) ListByDevice(_ context.Context, d id.DeviceID, from, to time.Time) ([]HistoryEvent, error) {
	from, to = roundMillis(from), roundMillis(to)
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	var out []HistoryEvent
	for _, e := range r.m.history {
		if e.Device == d && !e.At.Before(from) && e.At.Before(to) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b HistoryEvent) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func sortedValues[K comparable, V any](m map[K]V, cmp func(a, b V) int) []V {
	var out []V
	for _, v := range m {
		out = append(out, v)
	}
	slices.SortFunc(out, cmp)
	return out
}

// ---- device_key ----

type memDeviceKeys struct{ m *Memory }

func (r memDeviceKeys) Get(_ context.Context, d id.DeviceID) (DeviceKey, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	k, ok := r.m.keys[d]
	if !ok {
		return DeviceKey{}, fmt.Errorf("get device key %s: %w", d, ErrNotFound)
	}
	k.PublicKey = slices.Clone(k.PublicKey)
	return k, nil
}

func (r memDeviceKeys) Create(_ context.Context, k DeviceKey) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.keys[k.Device]; ok {
		return fmt.Errorf("create device key %s: %w", k.Device, ErrConflict)
	}
	k.PublicKey = slices.Clone(k.PublicKey)
	k.CreatedAt = roundMillis(k.CreatedAt)
	r.m.keys[k.Device] = k
	return nil
}

func (r memDeviceKeys) Delete(_ context.Context, d id.DeviceID) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	delete(r.m.keys, d)
	return nil
}

// ---- device_block ----

type memDeviceBlocks struct{ m *Memory }

func (r memDeviceBlocks) Put(_ context.Context, b DeviceBlock) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	b.CreatedAt = roundMillis(b.CreatedAt)
	r.m.blocks[b.Device] = b
	return nil
}

func (r memDeviceBlocks) Get(_ context.Context, d id.DeviceID) (DeviceBlock, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	b, ok := r.m.blocks[d]
	if !ok {
		return DeviceBlock{}, fmt.Errorf("get device block %s: %w", d, ErrNotFound)
	}
	return b, nil
}

func (r memDeviceBlocks) Delete(_ context.Context, d id.DeviceID) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	delete(r.m.blocks, d)
	return nil
}
