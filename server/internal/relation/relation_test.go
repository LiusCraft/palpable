package relation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	a id.DeviceID = "02:00:00:00:00:0a"
	b id.DeviceID = "02:00:00:00:00:0b"
	c id.DeviceID = "02:00:00:00:00:0c"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newRelations(t *testing.T) (*Relations, *clock, store.Store) {
	t.Helper()
	s := store.NewMemory()
	r, err := New(context.Background(), s.Friendships())
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)}
	r.now = clk.now
	return r, clk, s
}

func nextEvent(t *testing.T, r *Relations) Event {
	t.Helper()
	select {
	case e := <-r.Subscribe():
		return e
	default:
		t.Fatal("expected an event")
		return Event{}
	}
}

func noEvent(t *testing.T, r *Relations) {
	t.Helper()
	select {
	case e := <-r.Subscribe():
		t.Fatalf("unexpected event %+v", e)
	default:
	}
}

// bothNear 让 a、b 每 500ms 互相上报一次，共 n 次。
func bothNear(t *testing.T, r *Relations, clk *clock, n int, strength float64) {
	t.Helper()
	ctx := context.Background()
	for range n {
		if err := r.OnProximity(ctx, a, []Peer{{b, strength}}); err != nil {
			t.Fatal(err)
		}
		if err := r.OnProximity(ctx, b, []Peer{{a, strength}}); err != nil {
			t.Fatal(err)
		}
		clk.advance(500 * time.Millisecond)
	}
}

func friends(t *testing.T, r *Relations, x, y id.DeviceID) bool {
	t.Helper()
	ok, err := r.AreFriends(context.Background(), id.Device(x), id.Device(y))
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// agree 让两个孩子都说"好"，返回进入家长关卡（或直接成为好友）的事件。
func agree(t *testing.T, r *Relations) Event {
	t.Helper()
	ctx := context.Background()
	if others, err := r.Assent(ctx, a); err != nil || len(others) != 1 || others[0] != b {
		t.Fatalf("assent a: %v %v", others, err)
	}
	noEvent(t, r) // 只有一个孩子说"好"：还不够
	if _, err := r.Assent(ctx, b); err != nil {
		t.Fatal(err)
	}
	return nextEvent(t, r)
}

// pendingRequest 走完靠近 → 两个孩子同意，停在等家长。
func pendingRequest(t *testing.T, r *Relations, clk *clock) Friendship {
	t.Helper()
	bothNear(t, r, clk, 5, 0.9)
	if e := nextEvent(t, r); e.Kind != Asked {
		t.Fatalf("want Asked, got %+v", e)
	}
	e := agree(t, r)
	if e.Kind != Proposed || e.Friendship.Status != StatusPending {
		t.Fatalf("want Proposed, got %+v", e)
	}
	return e.Friendship
}

func TestProximityAsksKidsAfterHold(t *testing.T) {
	r, clk, _ := newRelations(t)
	bothNear(t, r, clk, 3, 0.9) // 0、0.5、1.0 秒：还不够
	noEvent(t, r)

	bothNear(t, r, clk, 2, 0.9) // 到 1.5 秒
	e := nextEvent(t, r)
	if e.Kind != Asked || e.Friendship.A != a || e.Friendship.B != b || e.Friendship.Status != StatusAsking {
		t.Fatalf("event = %+v", e)
	}
	bothNear(t, r, clk, 4, 0.9) // 正在问：不重复问
	noEvent(t, r)
	if friends(t, r, a, b) {
		t.Fatal("asking must not make them friends")
	}
	f := agree(t, r)
	if f.Kind != Proposed || len(f.Friendship.WaitingFor()) != 2 {
		t.Fatalf("both guardians should be waited for: %+v", f)
	}
	bothNear(t, r, clk, 4, 0.9) // 已在等家长：不再问
	noEvent(t, r)
}

func TestProximityNeedsBothSidesAndStrength(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	for range 6 { // 只有 a 看到 b
		_ = r.OnProximity(ctx, a, []Peer{{b, 1}})
		clk.advance(500 * time.Millisecond)
	}
	noEvent(t, r)

	bothNear(t, r, clk, 6, 0.5) // 不够近
	noEvent(t, r)
}

func TestProximityInterruptionRestartsTimer(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	bothNear(t, r, clk, 3, 0.9)
	_ = r.OnProximity(ctx, a, nil) // a 这次没看到 b
	bothNear(t, r, clk, 3, 0.9)
	noEvent(t, r)
	bothNear(t, r, clk, 1, 0.9)
	if e := nextEvent(t, r); e.Kind != Asked {
		t.Fatalf("event = %+v", e)
	}
}

func TestKidDeclineOrTimeoutDropsAsk(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	bothNear(t, r, clk, 5, 0.9)
	nextEvent(t, r)
	if others := r.Decline(ctx, b); len(others) != 1 || others[0] != a {
		t.Fatalf("decline: %v", others)
	}
	if e := nextEvent(t, r); e.Kind != Dropped {
		t.Fatalf("want Dropped, got %+v", e)
	}
	// 说"不要"之后再说"好"，没有可同意的询问。
	if others, _ := r.Assent(ctx, a); len(others) != 0 {
		t.Fatalf("assent after decline: %v", others)
	}

	// 再靠近会重新问；这次没人回答，超时作罢。
	bothNear(t, r, clk, 5, 0.9)
	nextEvent(t, r)
	_, _ = r.Assent(ctx, a)
	clk.advance(askWindow)
	if others, _ := r.Assent(ctx, b); len(others) != 0 {
		t.Fatalf("assent after expiry: %v", others)
	}
	if err := r.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, r); e.Kind != Dropped {
		t.Fatalf("want Dropped after timeout, got %+v", e)
	}
	noEvent(t, r)
}

func TestEachGuardianApprovesOwnSide(t *testing.T) {
	r, clk, s := newRelations(t)
	ctx := context.Background()
	req := pendingRequest(t, r, clk)

	if err := r.Confirm(ctx, req.ID, a, "ga"); err != nil {
		t.Fatal(err)
	}
	noEvent(t, r) // 还在等 b 的家长
	if friends(t, r, a, b) {
		t.Fatal("one side approved is not enough")
	}
	got, _ := r.Request(ctx, req.ID)
	if w := got.WaitingFor(); len(w) != 1 || w[0] != b {
		t.Fatalf("waiting for %v", w)
	}
	if !errors.Is(r.Confirm(ctx, req.ID, c, "gc"), ErrNotFound) {
		t.Fatal("a device outside the request cannot approve")
	}
	if err := r.Confirm(ctx, req.ID, b, "gb"); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, r); e.Kind != Confirmed || e.Friendship.ConfirmedBy != "gb" {
		t.Fatalf("event = %+v", e)
	}
	if !friends(t, r, a, b) || !friends(t, r, b, a) {
		t.Fatal("confirmed pair must be friends both ways")
	}
	if !errors.Is(r.Confirm(ctx, req.ID, a, "ga"), ErrNotPending) {
		t.Fatal("confirming twice must fail")
	}
	if !errors.Is(r.Confirm(ctx, "nope", a, "ga"), ErrNotFound) {
		t.Fatal("unknown request")
	}

	// 重启后从存储恢复好友缓存。
	r2, err := New(ctx, s.Friendships())
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := r2.AreFriends(ctx, id.Device(a), id.Device(b)); !ok {
		t.Fatal("friend cache not restored")
	}

	if err := r.Remove(ctx, b, a); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, r); e.Kind != Removed {
		t.Fatalf("event = %+v", e)
	}
	if friends(t, r, a, b) {
		t.Fatal("removed pair must not be friends")
	}
	if fs, _ := r.Friends(ctx, a); len(fs) != 0 {
		t.Fatalf("Friends after remove = %+v", fs)
	}

	// 解除后再次贴近，可以重新问；沿用同一条记录，放行要重新来。
	again := pendingRequest(t, r, clk)
	if again.ID != req.ID || len(again.WaitingFor()) != 2 {
		t.Fatalf("re-request = %+v", again)
	}
}

func TestGuardianDeclineCoolsDown(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	pendingRequest(t, r, clk)
	if err := r.Remove(ctx, a, b); err != nil { // 家长拒绝
		t.Fatal(err)
	}
	if e := nextEvent(t, r); e.Kind != Removed {
		t.Fatalf("event = %+v", e)
	}
	bothNear(t, r, clk, 5, 0.9)
	noEvent(t, r) // 冷却期内不再问
	clk.advance(declineCooldown)
	bothNear(t, r, clk, 5, 0.9)
	if e := nextEvent(t, r); e.Kind != Asked {
		t.Fatalf("after cooldown: %+v", e)
	}
}

func TestPendingRequestExpires(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	pendingRequest(t, r, clk)
	clk.advance(pendingTTL)
	if err := r.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, r); e.Kind != Removed {
		t.Fatalf("event = %+v", e)
	}
	// 过期不算拒绝：再靠近马上能问。
	bothNear(t, r, clk, 5, 0.9)
	if e := nextEvent(t, r); e.Kind != Asked {
		t.Fatalf("after expiry: %+v", e)
	}
}

func TestAutoApprovePerSide(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	// a 的家长设为"自己交友"，b 的家长要批准。
	r.SetAutoApprove(func(_ context.Context, d id.DeviceID) bool { return d == a })
	req := pendingRequest(t, r, clk)
	if req.ApprovedA != AutoGuardian || req.ApprovedB != "" {
		t.Fatalf("approvals %+v", req)
	}
	if err := r.Confirm(ctx, req.ID, b, "gb"); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, r); e.Kind != Confirmed {
		t.Fatalf("event = %+v", e)
	}

	// 两家都设为"自己交友"：两个孩子都说"好"就成为好友，ConfirmedBy 记 auto。
	r2, clk2, _ := newRelations(t)
	r2.SetAutoApprove(func(context.Context, id.DeviceID) bool { return true })
	bothNear(t, r2, clk2, 5, 0.9)
	nextEvent(t, r2)
	e := agree(t, r2)
	if e.Kind != Confirmed || e.Friendship.ConfirmedBy != AutoGuardian || !friends(t, r2, a, b) {
		t.Fatalf("event = %+v", e)
	}
}

func TestFilterFriendsAndAgents(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	r.SetAutoApprove(func(context.Context, id.DeviceID) bool { return true })
	bothNear(t, r, clk, 5, 0.9)
	nextEvent(t, r)
	agree(t, r)

	got := r.FilterFriends(ctx, id.Device(a), []id.ParticipantID{id.Device(a), id.Device(b), id.Device(c)})
	if len(got) != 1 || got[0] != id.Device(b) {
		t.Fatalf("FilterFriends = %v", got)
	}
	// 智能体参与者按所属设备判断。
	if ok, _ := r.AreFriends(ctx, "agt:"+id.ParticipantID(b), id.Device(a)); !ok {
		t.Fatal("agent of a friend should count as friend")
	}
}
