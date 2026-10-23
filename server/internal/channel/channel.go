// Package channel 回答"谁在哪个频道"：区域频道与群的成员、4 位码入群与过期、每台设备的前台频道。
// 音频与发言权在 voice，好友在 relation，下行消息由 app 订阅事件后发送。
package channel

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	tripTTL         = 6 * time.Hour
	badCodeLimit    = 5
	badCodeWindow   = time.Minute
	codeAttempts    = 50
	eventBuffer     = 256
	groupIDPrefix   = "group:"
	zoneIDPrefix    = "zone:"
	groupNamePrefix = "群聊"
)

var (
	ErrNotInChannel     = errors.New("channel: not in any channel")
	ErrNoSuchChannel    = errors.New("channel: no such channel")
	ErrBadCode          = errors.New("channel: wrong or expired code")
	ErrTooManyAttempts  = errors.New("channel: too many wrong codes, try again later")
	ErrNoCodesAvailable = errors.New("channel: no free group code")
)

type Kind int

const (
	KindZone Kind = iota
	KindGroup
)

type GroupTTL int

const (
	TTLTrip GroupTTL = iota // 本次行程
	TTLToday
	TTLForever
)

type Channel struct {
	ID        id.ChannelID
	Kind      Kind
	Name      string
	Code      string      // 仅群：4 位数字
	Owner     id.DeviceID // 仅群
	ExpiresAt time.Time   // 仅群；零值表示长期
}

type ChannelInfo struct {
	Channel    Channel
	Members    []id.ParticipantID
	Foreground bool
}

type EventKind int

const (
	Joined   EventKind = iota // 加入并成为前台
	Left                      // 离开（含过期）
	Switched                  // 前台切换到已加入的频道
)

type Event struct {
	Kind        EventKind
	Participant id.ParticipantID
	Channel     Channel
}

// ZoneID 把区域 ID 映射为区域频道 ID。
func ZoneID(zone string) id.ChannelID { return id.ChannelID(zoneIDPrefix + zone) }

type chanState struct {
	ch      Channel
	members map[id.ParticipantID]time.Time // 加入时间
}

type participant struct {
	channels   []id.ChannelID // 按加入顺序，供 next / prev
	foreground id.ChannelID
}

type Channels struct {
	repo   store.GroupRepo
	now    func() time.Time
	events chan Event

	mu      sync.RWMutex
	chans   map[id.ChannelID]*chanState
	parts   map[id.ParticipantID]*participant
	badCode map[id.DeviceID][]time.Time
}

// New 装载未过期的群与成员；区域频道成员只在内存，启动时为空。
func New(ctx context.Context, repo store.GroupRepo) (*Channels, error) {
	c := &Channels{
		repo:    repo,
		now:     time.Now,
		events:  make(chan Event, eventBuffer),
		chans:   map[id.ChannelID]*chanState{},
		parts:   map[id.ParticipantID]*participant{},
		badCode: map[id.DeviceID][]time.Time{},
	}
	if _, err := repo.DeleteExpired(ctx, c.now()); err != nil {
		return nil, fmt.Errorf("load groups: %w", err)
	}
	groups, err := repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load groups: %w", err)
	}
	for _, g := range groups {
		cs := &chanState{ch: fromGroup(g), members: map[id.ParticipantID]time.Time{}}
		c.chans[g.ID] = cs
		ms, err := repo.Members(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("load members of %s: %w", g.ID, err)
		}
		for _, m := range ms {
			cs.members[m.Participant] = m.JoinedAt
			p := c.part(m.Participant)
			p.channels = append(p.channels, g.ID)
		}
	}
	return c, nil
}

func (c *Channels) Subscribe() <-chan Event { return c.events }

// DefineZone 由 zone 在启动时登记区域频道及其名字。
func (c *Channels) DefineZone(ch id.ChannelID, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cs, ok := c.chans[ch]; ok {
		cs.ch.Name = name
		return
	}
	c.chans[ch] = &chanState{ch: Channel{ID: ch, Kind: KindZone, Name: name}, members: map[id.ParticipantID]time.Time{}}
}

func (c *Channels) JoinZone(_ context.Context, d id.DeviceID, ch id.ChannelID) error {
	c.mu.Lock()
	cs, ok := c.chans[ch]
	if !ok || cs.ch.Kind != KindZone {
		c.mu.Unlock()
		return fmt.Errorf("join zone %s: %w", ch, ErrNoSuchChannel)
	}
	// 同时只在一个区域频道。
	var evs []Event
	for _, other := range c.zonesOf(id.Device(d)) {
		if other != ch {
			evs = append(evs, c.leaveLocked(id.Device(d), other)...)
		}
	}
	evs = append(evs, c.joinLocked(id.Device(d), cs)...)
	c.mu.Unlock()
	c.emit(evs...)
	return nil
}

func (c *Channels) LeaveZone(_ context.Context, d id.DeviceID) error {
	c.mu.Lock()
	var evs []Event
	for _, ch := range c.zonesOf(id.Device(d)) {
		evs = append(evs, c.leaveLocked(id.Device(d), ch)...)
	}
	c.mu.Unlock()
	c.emit(evs...)
	return nil
}

func (c *Channels) CreateGroup(ctx context.Context, owner id.DeviceID, ttl GroupTTL) (string, error) {
	now := c.now()
	c.mu.Lock()
	code, err := c.freeCodeLocked(now)
	c.mu.Unlock()
	if err != nil {
		return "", err
	}
	g := store.Group{ID: id.ChannelID(groupIDPrefix + id.New()), Name: groupNamePrefix + code, Owner: owner, Code: code, ExpiresAt: expiry(now, ttl)}
	if err := c.repo.Create(ctx, g); err != nil {
		return "", fmt.Errorf("create group: %w", err)
	}
	p := id.Device(owner)
	if err := c.repo.AddMember(ctx, g.ID, store.Member{Participant: p, JoinedAt: now}); err != nil {
		return "", fmt.Errorf("create group: %w", err)
	}
	c.mu.Lock()
	cs := &chanState{ch: fromGroup(g), members: map[id.ParticipantID]time.Time{}}
	c.chans[g.ID] = cs
	evs := c.joinLocked(p, cs)
	c.mu.Unlock()
	c.emit(evs...)
	return code, nil
}

func (c *Channels) JoinGroup(ctx context.Context, d id.DeviceID, code string) error {
	now := c.now()
	c.mu.Lock()
	if c.limitedLocked(d, now) {
		c.mu.Unlock()
		return ErrTooManyAttempts
	}
	var cs *chanState
	for _, s := range c.chans {
		if s.ch.Kind == KindGroup && s.ch.Code == code && active(s.ch, now) {
			cs = s
			break
		}
	}
	if cs == nil {
		c.badCode[d] = append(c.badCode[d], now)
		c.mu.Unlock()
		return ErrBadCode
	}
	delete(c.badCode, d)
	ch := cs.ch.ID
	c.mu.Unlock()

	p := id.Device(d)
	if err := c.repo.AddMember(ctx, ch, store.Member{Participant: p, JoinedAt: now}); err != nil {
		return fmt.Errorf("join group %s: %w", ch, err)
	}
	c.mu.Lock()
	evs := c.joinLocked(p, cs)
	c.mu.Unlock()
	c.emit(evs...)
	return nil
}

// Switch 把前台切到 target：群名、4 位码、"next" 或 "prev"。
func (c *Channels) Switch(_ context.Context, d id.DeviceID, target string) error {
	p := id.Device(d)
	c.mu.Lock()
	pt := c.parts[p]
	if pt == nil || len(pt.channels) == 0 {
		c.mu.Unlock()
		return ErrNotInChannel
	}
	var to id.ChannelID
	switch target {
	case "next", "prev":
		i := slices.Index(pt.channels, pt.foreground)
		step := 1
		if target == "prev" {
			step = -1
		}
		n := len(pt.channels)
		to = pt.channels[((i+step)%n+n)%n]
	default:
		t := strings.TrimSpace(target)
		for _, ch := range pt.channels {
			if cs := c.chans[ch]; cs != nil && (cs.ch.Name == t || cs.ch.Code == t || string(ch) == t) {
				to = ch
				break
			}
		}
	}
	if to == "" {
		c.mu.Unlock()
		return fmt.Errorf("switch to %q: %w", target, ErrNoSuchChannel)
	}
	var evs []Event
	if to != pt.foreground {
		pt.foreground = to
		evs = append(evs, Event{Kind: Switched, Participant: p, Channel: c.chans[to].ch})
	}
	c.mu.Unlock()
	c.emit(evs...)
	return nil
}

// Leave 离开指定频道；群成员身份同时从存储删除。
func (c *Channels) Leave(ctx context.Context, d id.DeviceID, ch id.ChannelID) error {
	p := id.Device(d)
	c.mu.Lock()
	cs, ok := c.chans[ch]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("leave %s: %w", ch, ErrNoSuchChannel)
	}
	if _, member := cs.members[p]; !member {
		c.mu.Unlock()
		return nil
	}
	evs := c.leaveLocked(p, ch)
	kind := cs.ch.Kind
	c.mu.Unlock()
	if kind == KindGroup {
		if err := c.repo.RemoveMember(ctx, ch, p); err != nil {
			return fmt.Errorf("leave %s: %w", ch, err)
		}
	}
	c.emit(evs...)
	return nil
}

// Current 返回设备的前台频道。
func (c *Channels) Current(_ context.Context, d id.DeviceID) (ChannelInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pt := c.parts[id.Device(d)]
	if pt == nil || pt.foreground == "" {
		return ChannelInfo{}, ErrNotInChannel
	}
	return c.infoLocked(pt.foreground, true), nil
}

// List 返回设备所在的全部频道，前台在前。
func (c *Channels) List(_ context.Context, d id.DeviceID) []ChannelInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pt := c.parts[id.Device(d)]
	if pt == nil {
		return nil
	}
	var out []ChannelInfo
	for _, ch := range pt.channels {
		out = append(out, c.infoLocked(ch, ch == pt.foreground))
	}
	slices.SortStableFunc(out, func(a, b ChannelInfo) int {
		switch {
		case a.Foreground == b.Foreground:
			return 0
		case a.Foreground:
			return -1
		}
		return 1
	})
	return out
}

// ForegroundOf 热路径。
func (c *Channels) ForegroundOf(p id.ParticipantID) (id.ChannelID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pt := c.parts[p]
	if pt == nil || pt.foreground == "" {
		return "", false
	}
	return pt.foreground, true
}

// Members 热路径。
func (c *Channels) Members(ch id.ChannelID) []id.ParticipantID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cs, ok := c.chans[ch]
	if !ok {
		return nil
	}
	return sortedMembers(cs)
}

// Sweep 删除过期的群并让成员离开；app 定期调用。
func (c *Channels) Sweep(ctx context.Context) error {
	now := c.now()
	if _, err := c.repo.DeleteExpired(ctx, now); err != nil {
		return fmt.Errorf("sweep groups: %w", err)
	}
	c.mu.Lock()
	var evs []Event
	for ch, cs := range c.chans {
		if cs.ch.Kind != KindGroup || active(cs.ch, now) {
			continue
		}
		for p := range cs.members {
			evs = append(evs, c.leaveLocked(p, ch)...)
		}
		delete(c.chans, ch)
	}
	for d, ts := range c.badCode {
		if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > badCodeWindow {
			delete(c.badCode, d)
		}
	}
	c.mu.Unlock()
	c.emit(evs...)
	return nil
}

func (c *Channels) part(p id.ParticipantID) *participant {
	pt := c.parts[p]
	if pt == nil {
		pt = &participant{}
		c.parts[p] = pt
	}
	return pt
}

func (c *Channels) zonesOf(p id.ParticipantID) []id.ChannelID {
	pt := c.parts[p]
	if pt == nil {
		return nil
	}
	var out []id.ChannelID
	for _, ch := range pt.channels {
		if cs := c.chans[ch]; cs != nil && cs.ch.Kind == KindZone {
			out = append(out, ch)
		}
	}
	return out
}

// joinLocked 加入并设为前台；已在频道里时只切前台。
func (c *Channels) joinLocked(p id.ParticipantID, cs *chanState) []Event {
	pt := c.part(p)
	if _, ok := cs.members[p]; !ok {
		cs.members[p] = c.now()
	}
	if !slices.Contains(pt.channels, cs.ch.ID) {
		pt.channels = append(pt.channels, cs.ch.ID)
	}
	if pt.foreground == cs.ch.ID {
		return nil
	}
	pt.foreground = cs.ch.ID
	return []Event{{Kind: Joined, Participant: p, Channel: cs.ch}}
}

// leaveLocked 离开；离开的是前台时，前台回到最近加入的另一个频道。
func (c *Channels) leaveLocked(p id.ParticipantID, ch id.ChannelID) []Event {
	cs := c.chans[ch]
	if cs == nil {
		return nil
	}
	delete(cs.members, p)
	pt := c.parts[p]
	if pt == nil {
		return nil
	}
	pt.channels = slices.DeleteFunc(pt.channels, func(x id.ChannelID) bool { return x == ch })
	evs := []Event{{Kind: Left, Participant: p, Channel: cs.ch}}
	if pt.foreground == ch {
		pt.foreground = ""
		if n := len(pt.channels); n > 0 {
			pt.foreground = pt.channels[n-1]
			evs = append(evs, Event{Kind: Switched, Participant: p, Channel: c.chans[pt.foreground].ch})
		}
	}
	if len(pt.channels) == 0 {
		delete(c.parts, p)
	}
	return evs
}

func (c *Channels) infoLocked(ch id.ChannelID, fg bool) ChannelInfo {
	cs := c.chans[ch]
	return ChannelInfo{Channel: cs.ch, Members: sortedMembers(cs), Foreground: fg}
}

func (c *Channels) limitedLocked(d id.DeviceID, now time.Time) bool {
	ts := slices.DeleteFunc(c.badCode[d], func(t time.Time) bool { return now.Sub(t) > badCodeWindow })
	c.badCode[d] = ts
	return len(ts) >= badCodeLimit
}

func (c *Channels) freeCodeLocked(now time.Time) (string, error) {
	used := map[string]bool{}
	for _, cs := range c.chans {
		if cs.ch.Kind == KindGroup && active(cs.ch, now) {
			used[cs.ch.Code] = true
		}
	}
	for range codeAttempts {
		n, err := rand.Int(rand.Reader, big.NewInt(9000))
		if err != nil {
			return "", err
		}
		code := fmt.Sprintf("%04d", n.Int64()+1000)
		if !used[code] {
			return code, nil
		}
	}
	return "", ErrNoCodesAvailable
}

func (c *Channels) emit(evs ...Event) {
	for _, e := range evs {
		select {
		case c.events <- e:
		default:
			slog.Error("channel event dropped: subscriber too slow", "kind", e.Kind, "channel", e.Channel.ID)
		}
	}
}

func sortedMembers(cs *chanState) []id.ParticipantID {
	out := make([]id.ParticipantID, 0, len(cs.members))
	for p := range cs.members {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b id.ParticipantID) int {
		if c := cs.members[a].Compare(cs.members[b]); c != 0 {
			return c
		}
		return strings.Compare(string(a), string(b))
	})
	return out
}

func active(ch Channel, now time.Time) bool { return ch.ExpiresAt.IsZero() || now.Before(ch.ExpiresAt) }

func expiry(now time.Time, ttl GroupTTL) time.Time {
	switch ttl {
	case TTLTrip:
		return now.Add(tripTTL)
	case TTLToday:
		y, m, d := now.Date()
		return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
	}
	return time.Time{}
}

func fromGroup(g store.Group) Channel {
	return Channel{ID: g.ID, Kind: KindGroup, Name: g.Name, Code: g.Code, Owner: g.Owner, ExpiresAt: g.ExpiresAt}
}
