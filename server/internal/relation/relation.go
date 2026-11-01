// Package relation 管理好友关系：靠近 → 两个孩子都说"好" → 两家监护人各自放行 → 好友。
// 规则见 docs/trust.md §4。谁有权放行由 parentapi 鉴权后决定；每家是否自动放行由 AutoApprove 端口回答。
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

	// 两个孩子要在这段时间内都说"好"。
	askWindow = 60 * time.Second
	// 家长拒绝后，同一对设备这么久内不再询问。
	declineCooldown = 24 * time.Hour
	// 等家长放行的请求过期时间。
	pendingTTL = 7 * 24 * time.Hour
)

// AutoGuardian 记在 ConfirmedBy 与 Approved* 上，表示这一侧（或整条关系）按家长"孩子自己交友"的设置自动放行。
const AutoGuardian id.GuardianID = "auto"

var (
	ErrNotFound   = errors.New("relation: friend request not found")
	ErrNotPending = errors.New("relation: friend request is not pending")
)

type Status int

// 数值存进库里，只能追加。
const (
	StatusPending   Status = iota // 两个孩子都同意了，等家长放行
	StatusConfirmed               // 好友
	StatusRemoved                 // 已解除（或请求过期）
	StatusDeclined                // 家长拒绝；冷却期内不再询问
	StatusAsking                  // 只在事件里出现：正在问两个孩子，不入库
)

type Friendship struct {
	ID                   string
	A, B                 id.DeviceID // A < B，一对只有一条
	Status               Status
	ConfirmedBy          id.GuardianID
	ApprovedA, ApprovedB id.GuardianID // 每一侧由谁放行；空表示在等这一侧的家长
	CreatedAt            time.Time
}

// WaitingFor 返回还在等哪一侧的家长放行。
func (f Friendship) WaitingFor() []id.DeviceID {
	var out []id.DeviceID
	if f.Status != StatusPending {
		return nil
	}
	if f.ApprovedA == "" {
		out = append(out, f.A)
	}
	if f.ApprovedB == "" {
		out = append(out, f.B)
	}
	return out
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
	Proposed  EventKind = iota // 两个孩子都同意了，等家长
	Confirmed                  // 成为好友
	Removed                    // 解除、家长拒绝或请求过期
	Asked                      // 靠近了，问两个孩子要不要交朋友
	Dropped                    // 询问作罢：有孩子说"不要"或超时
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

// AutoApprove 回答设备 d 的家长是否设为"孩子自己交友、事后通知"。
type AutoApprove func(ctx context.Context, d id.DeviceID) bool

type asking struct {
	a, b    bool // 两个孩子是否已说"好"
	expires time.Time
}

type Relations struct {
	repo        store.FriendshipRepo
	now         func() time.Time
	events      chan Event
	autoApprove AutoApprove

	// 好友快照：AreFriends 在热路径上，读不加锁，写时整份替换。
	friends atomic.Pointer[map[pair]struct{}]

	mu    sync.Mutex // 保护 near、asks 与写路径
	close map[[2]id.DeviceID]near
	asks  map[pair]*asking
}

// New 从存储装载已确认的好友。
func New(ctx context.Context, repo store.FriendshipRepo) (*Relations, error) {
	r := &Relations{
		repo:        repo,
		now:         time.Now,
		events:      make(chan Event, eventBuffer),
		close:       map[[2]id.DeviceID]near{},
		asks:        map[pair]*asking{},
		autoApprove: func(context.Context, id.DeviceID) bool { return false },
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

// SetAutoApprove 须在开始上报靠近之前调用；默认每一侧都要家长放行。
func (r *Relations) SetAutoApprove(fn AutoApprove) { r.autoApprove = fn }

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

// Friends 返回设备的好友与等家长放行的请求，不含已解除、已拒绝的。
func (r *Relations) Friends(ctx context.Context, d id.DeviceID) ([]Friendship, error) {
	all, err := r.repo.ListByDevice(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("friends of %s: %w", d, err)
	}
	var out []Friendship
	for _, f := range all {
		if st := Status(f.Status); st == StatusPending || st == StatusConfirmed {
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
		if err := r.ask(ctx, d, p); err != nil {
			return err
		}
	}
	return nil
}

// ask 双方靠近后先问两个孩子；已是好友、已在等家长、刚被家长拒绝或正在问的，不再问。
func (r *Relations) ask(ctx context.Context, x, y id.DeviceID) error {
	p := makePair(x, y)
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if q, ok := r.asks[p]; ok && now.Before(q.expires) {
		return nil
	}
	f, err := r.repo.ByPair(ctx, p.a, p.b)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return fmt.Errorf("ask %s/%s: %w", x, y, err)
	case Status(f.Status) == StatusPending || Status(f.Status) == StatusConfirmed:
		return nil
	case Status(f.Status) == StatusDeclined && now.Sub(f.UpdatedAt) < declineCooldown:
		return nil
	}
	r.asks[p] = &asking{expires: now.Add(askWindow)}
	r.emit(Event{Kind: Asked, Friendship: Friendship{A: p.a, B: p.b, Status: StatusAsking, CreatedAt: now}})
	return nil
}

// Assent 记下孩子 d 说"好"：对 d 所有还在询问中的配对生效。两个孩子都说"好"后进入家长关卡。
// 返回这次说"好"涉及的对方设备。
func (r *Relations) Assent(ctx context.Context, d id.DeviceID) ([]id.DeviceID, error) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	var others []id.DeviceID
	for p, q := range r.asks {
		if p.a != d && p.b != d {
			continue
		}
		if !now.Before(q.expires) {
			continue // 由 Sweep 清理并通知
		}
		if p.a == d {
			q.a = true
		} else {
			q.b = true
		}
		others = append(others, otherOf(p, d))
		if q.a && q.b {
			delete(r.asks, p)
			if err := r.promote(ctx, p, now); err != nil {
				return others, err
			}
		}
	}
	return others, nil
}

// Decline 孩子 d 说"不要"：作罢 d 所有还在询问中的配对，不打扰家长。
func (r *Relations) Decline(_ context.Context, d id.DeviceID) []id.DeviceID {
	r.mu.Lock()
	defer r.mu.Unlock()
	var others []id.DeviceID
	for p := range r.asks {
		if p.a == d || p.b == d {
			delete(r.asks, p)
			others = append(others, otherOf(p, d))
			r.emit(Event{Kind: Dropped, Friendship: Friendship{A: p.a, B: p.b, Status: StatusAsking}})
		}
	}
	return others
}

// promote 两个孩子都同意后：每一侧按自家家长的设置自动放行或等待批准；两侧都放行即成为好友。
func (r *Relations) promote(ctx context.Context, p pair, now time.Time) error {
	f, err := r.repo.ByPair(ctx, p.a, p.b)
	fresh := errors.Is(err, store.ErrNotFound)
	if err != nil && !fresh {
		return fmt.Errorf("promote %s/%s: %w", p.a, p.b, err)
	}
	if fresh {
		f = store.Friendship{ID: id.New(), A: p.a, B: p.b, CreatedAt: now}
	}
	f.ApprovedA, f.ApprovedB, f.ConfirmedBy, f.UpdatedAt = "", "", "", now
	if r.autoApprove(ctx, p.a) {
		f.ApprovedA = AutoGuardian
	}
	if r.autoApprove(ctx, p.b) {
		f.ApprovedB = AutoGuardian
	}
	kind := Proposed
	f.Status = int(StatusPending)
	if f.ApprovedA != "" && f.ApprovedB != "" {
		kind, f.Status, f.ConfirmedBy = Confirmed, int(StatusConfirmed), AutoGuardian
	}
	if fresh {
		err = r.repo.Create(ctx, f)
	} else {
		err = r.repo.Update(ctx, f)
	}
	if err != nil {
		return fmt.Errorf("promote %s/%s: %w", p.a, p.b, err)
	}
	if kind == Confirmed {
		r.setFriend(p, true)
	}
	r.emit(Event{Kind: kind, Friendship: fromRecord(f)})
	return nil
}

// Confirm 由 side 的监护人 by 放行这一侧；调用方须先确认 by 是 side 的监护人。两侧都放行后成为好友。
func (r *Relations) Confirm(ctx context.Context, reqID string, side id.DeviceID, by id.GuardianID) error {
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
	switch side {
	case f.A:
		f.ApprovedA = by
	case f.B:
		f.ApprovedB = by
	default:
		return ErrNotFound
	}
	f.UpdatedAt = r.now()
	kind := Event{Kind: Proposed}
	if f.ApprovedA != "" && f.ApprovedB != "" {
		f.Status, f.ConfirmedBy = int(StatusConfirmed), by
		kind.Kind = Confirmed
	}
	if err := r.repo.Update(ctx, f); err != nil {
		return fmt.Errorf("confirm %s: %w", reqID, err)
	}
	if kind.Kind == Confirmed {
		r.setFriend(pair{f.A, f.B}, true)
		kind.Friendship = fromRecord(f)
		r.emit(kind)
	}
	return nil
}

// Sweep 作罢超时的询问、让过期的家长请求失效；由 app 定时调用。
func (r *Relations) Sweep(ctx context.Context) error {
	now := r.now()
	r.mu.Lock()
	for p, q := range r.asks {
		if !now.Before(q.expires) {
			delete(r.asks, p)
			r.emit(Event{Kind: Dropped, Friendship: Friendship{A: p.a, B: p.b, Status: StatusAsking}})
		}
	}
	r.mu.Unlock()
	pending, err := r.repo.ListByStatus(ctx, int(StatusPending))
	if err != nil {
		return fmt.Errorf("sweep friend requests: %w", err)
	}
	for _, f := range pending {
		if since := f.UpdatedAt; !since.IsZero() && now.Sub(since) >= pendingTTL || since.IsZero() && now.Sub(f.CreatedAt) >= pendingTTL {
			if err := r.remove(ctx, f.A, f.B, false); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}
	return nil
}

func otherOf(p pair, d id.DeviceID) id.DeviceID {
	if p.a == d {
		return p.b
	}
	return p.a
}

// Remove 立即解除好友并发事件，voice 据此中断正在进行的转发；对等家长放行的请求，等同家长拒绝。
func (r *Relations) Remove(ctx context.Context, a, b id.DeviceID) error {
	return r.remove(ctx, a, b, true)
}

// remove 中 decline 为 false 表示请求过期：不进入冷却期。
func (r *Relations) remove(ctx context.Context, a, b id.DeviceID, decline bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := r.repo.ByPair(ctx, a, b)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("remove %s/%s: %w", a, b, err)
	}
	switch Status(f.Status) {
	case StatusRemoved, StatusDeclined:
		return nil
	case StatusPending:
		f.Status = int(StatusRemoved)
		if decline {
			f.Status = int(StatusDeclined)
		}
	default:
		f.Status = int(StatusRemoved)
	}
	f.UpdatedAt = r.now()
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
	return Friendship{ID: f.ID, A: f.A, B: f.B, Status: Status(f.Status), ConfirmedBy: f.ConfirmedBy,
		ApprovedA: f.ApprovedA, ApprovedB: f.ApprovedB, CreatedAt: f.CreatedAt}
}

// RemoveAll 解除设备的全部好友与请求，设备解绑（恢复出厂）时用；每一条都发 Removed，voice 据此中断转发。
func (r *Relations) RemoveAll(ctx context.Context, d id.DeviceID) error {
	fs, err := r.Friends(ctx, d)
	if err != nil {
		return err
	}
	for _, f := range fs {
		if err := r.Remove(ctx, f.A, f.B); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}
