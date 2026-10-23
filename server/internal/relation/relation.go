// Package relation 管理好友关系：状态机、"两台设备靠近"转为待确认请求、好友查询。
// 谁有权确认由 parentapi 鉴权后决定；这里只保证只有 Confirm 能让两台设备成为好友。
package relation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	// 靠近判定：信号强度达到阈值，且双方都持续上报对方这么久。
	nearStrength = 0.8
	nearHold     = 1500 * time.Millisecond
	// 两次上报间隔超过这个值视为中断，重新计时。
	reportGap = 3 * time.Second

	eventBuffer = 256
)

var (
	ErrNotFound   = errors.New("relation: friend request not found")
	ErrNotPending = errors.New("relation: friend request is not pending")
)

type Status int

const (
	StatusPending Status = iota
	StatusConfirmed
	StatusRemoved
)

type Friendship struct {
	ID          string
	A, B        id.DeviceID // A < B，一对只有一条
	Status      Status
	ConfirmedBy id.GuardianID
	CreatedAt   time.Time
}

// Other 返回关系中 d 之外的那台设备。
func (f Friendship) Other(d id.DeviceID) id.DeviceID {
	if f.A == d {
		return f.B
	}
	return f.A
}

type Peer struct {
	Device   id.DeviceID
	Strength float64 // 0..1，越大越近
}

type EventKind int

const (
	Proposed EventKind = iota
	Confirmed
	Removed
)

type Event struct {
	Kind       EventKind
	Friendship Friendship
}

type pair struct{ a, b id.DeviceID } // 有序：a < b

func makePair(x, y id.DeviceID) pair {
	if x > y {
		x, y = y, x
	}
	return pair{x, y}
}

// near 记录 from 对 to 的持续靠近。
type near struct {
	since, last time.Time
}

type Relations struct {
	repo   store.FriendshipRepo
	now    func() time.Time
	events chan Event

	// 好友快照：AreFriends 在热路径上，读不加锁，写时整份替换。
	friends atomic.Pointer[map[pair]struct{}]

	mu    sync.Mutex // 保护 near 与写路径
	close map[[2]id.DeviceID]near
}

// New 从存储装载已确认的好友。
func New(ctx context.Context, repo store.FriendshipRepo) (*Relations, error) {
	r := &Relations{
		repo:   repo,
		now:    time.Now,
		events: make(chan Event, eventBuffer),
		close:  map[[2]id.DeviceID]near{},
	}
	confirmed, err := repo.ListByStatus(ctx, int(StatusConfirmed))
	if err != nil {
		return nil, fmt.Errorf("load friendships: %w", err)
	}
	m := make(map[pair]struct{}, len(confirmed))
	for _, f := range confirmed {
		m[pair{f.A, f.B}] = struct{}{}
	}
	r.friends.Store(&m)
	return r, nil
}

func (r *Relations) Subscribe() <-chan Event { return r.events }

// AreFriends 按参与者所属设备判断；热路径，不访问存储。
func (r *Relations) AreFriends(_ context.Context, a, b id.ParticipantID) (bool, error) {
	da, db := id.OwnerDevice(a), id.OwnerDevice(b)
	if da == "" || db == "" || da == db {
		return false, nil
	}
	_, ok := (*r.friends.Load())[makePair(da, db)]
	return ok, nil
}

// FilterFriends 只留下 viewer 的好友，供在线列表与成员展示使用，陌生人不出现。
func (r *Relations) FilterFriends(ctx context.Context, viewer id.ParticipantID, ps []id.ParticipantID) []id.ParticipantID {
	var out []id.ParticipantID
	for _, p := range ps {
		if ok, _ := r.AreFriends(ctx, viewer, p); ok {
			out = append(out, p)
		}
	}
	return out
}

// Friends 返回设备的好友与待确认请求，不含已解除的。
func (r *Relations) Friends(ctx context.Context, d id.DeviceID) ([]Friendship, error) {
	all, err := r.repo.ListByDevice(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("friends of %s: %w", d, err)
	}
	var out []Friendship
	for _, f := range all {
		if Status(f.Status) != StatusRemoved {
			out = append(out, fromRecord(f))
		}
	}
	return out, nil
}

// Request 返回一条好友请求，供 parentapi 校验归属。
func (r *Relations) Request(ctx context.Context, reqID string) (Friendship, error) {
	f, err := r.repo.Get(ctx, reqID)
	if errors.Is(err, store.ErrNotFound) {
		return Friendship{}, ErrNotFound
	}
	if err != nil {
		return Friendship{}, err
	}
	return fromRecord(f), nil
}

// OnProximity 处理设备 d 的一次完整上报：peers 是它此刻看到的全部设备。
func (r *Relations) OnProximity(ctx context.Context, d id.DeviceID, peers []Peer) error {
	now := r.now()
	var ready []id.DeviceID

	r.mu.Lock()
	seen := map[id.DeviceID]bool{}
	for _, p := range peers {
		if p.Device == d || p.Strength < nearStrength {
			continue
		}
		seen[p.Device] = true
		k := [2]id.DeviceID{d, p.Device}
		n, ok := r.close[k]
		if !ok || now.Sub(n.last) > reportGap {
			n.since = now
		}
		n.last = now
		r.close[k] = n

		back, ok := r.close[[2]id.DeviceID{p.Device, d}]
		if ok && now.Sub(back.last) <= reportGap && now.Sub(n.since) >= nearHold && now.Sub(back.since) >= nearHold {
			ready = append(ready, p.Device)
		}
	}
	// 这次没出现或不够近的设备，计时中断。
	for k := range r.close {
		if k[0] == d && !seen[k[1]] {
			delete(r.close, k)
		}
	}
	r.mu.Unlock()

	for _, p := range ready {
		if err := r.propose(ctx, d, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *Relations) propose(ctx context.Context, x, y id.DeviceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := r.repo.ByPair(ctx, x, y)
	switch {
	case errors.Is(err, store.ErrNotFound):
		f = store.Friendship{ID: id.New(), A: x, B: y, Status: int(StatusPending), CreatedAt: r.now()}
		if err := r.repo.Create(ctx, f); err != nil {
			return fmt.Errorf("propose %s/%s: %w", x, y, err)
		}
		f.A, f.B = makePair(x, y).a, makePair(x, y).b
	case err != nil:
		return fmt.Errorf("propose %s/%s: %w", x, y, err)
	case Status(f.Status) != StatusRemoved:
		return nil // 已是好友或已有未处理请求
	default:
		f.Status, f.ConfirmedBy = int(StatusPending), ""
		if err := r.repo.Update(ctx, f); err != nil {
			return fmt.Errorf("propose %s/%s: %w", x, y, err)
		}
	}
	r.emit(Event{Kind: Proposed, Friendship: fromRecord(f)})
	return nil
}

// Confirm 让待确认请求生效；调用方须先确认 by 有权代表其中一台设备。
func (r *Relations) Confirm(ctx context.Context, reqID string, by id.GuardianID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := r.repo.Get(ctx, reqID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("confirm %s: %w", reqID, err)
	}
	if Status(f.Status) != StatusPending {
		return ErrNotPending
	}
	f.Status, f.ConfirmedBy = int(StatusConfirmed), by
	if err := r.repo.Update(ctx, f); err != nil {
		return fmt.Errorf("confirm %s: %w", reqID, err)
	}
	r.setFriend(pair{f.A, f.B}, true)
	r.emit(Event{Kind: Confirmed, Friendship: fromRecord(f)})
	return nil
}

// Remove 立即解除好友（或拒绝待确认请求）并发事件，voice 据此中断正在进行的转发。
func (r *Relations) Remove(ctx context.Context, a, b id.DeviceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := r.repo.ByPair(ctx, a, b)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("remove %s/%s: %w", a, b, err)
	}
	if Status(f.Status) == StatusRemoved {
		return nil
	}
	f.Status = int(StatusRemoved)
	if err := r.repo.Update(ctx, f); err != nil {
		return fmt.Errorf("remove %s/%s: %w", a, b, err)
	}
	r.setFriend(pair{f.A, f.B}, false)
	r.emit(Event{Kind: Removed, Friendship: fromRecord(f)})
	return nil
}

func (r *Relations) setFriend(p pair, on bool) {
	old := *r.friends.Load()
	m := make(map[pair]struct{}, len(old)+1)
	for k := range old {
		m[k] = struct{}{}
	}
	if on {
		m[p] = struct{}{}
	} else {
		delete(m, p)
	}
	r.friends.Store(&m)
}

func (r *Relations) emit(e Event) {
	select {
	case r.events <- e:
	default:
		slog.Error("relation event dropped: subscriber too slow", "kind", e.Kind, "friendship", e.Friendship.ID)
	}
}

func fromRecord(f store.Friendship) Friendship {
	return Friendship{ID: f.ID, A: f.A, B: f.B, Status: Status(f.Status), ConfirmedBy: f.ConfirmedBy, CreatedAt: f.CreatedAt}
}
