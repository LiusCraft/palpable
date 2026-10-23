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

func TestProximityProposesAfterHold(t *testing.T) {
	r, clk, _ := newRelations(t)
	bothNear(t, r, clk, 3, 0.9) // 0、0.5、1.0 秒：还不够
	noEvent(t, r)

	bothNear(t, r, clk, 2, 0.9) // 到 1.5 秒
	e := nextEvent(t, r)
	if e.Kind != Proposed || e.Friendship.A != a || e.Friendship.B != b || e.Friendship.Status != StatusPending {
		t.Fatalf("event = %+v", e)
	}
	if friends(t, r, a, b) {
		t.Fatal("pending request must not make them friends")
	}

	bothNear(t, r, clk, 4, 0.9) // 已有未处理请求：不重复提议
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
	if e := nextEvent(t, r); e.Kind != Proposed {
		t.Fatalf("event = %+v", e)
	}
}

func TestConfirmRemoveAndRepropose(t *testing.T) {
	r, clk, s := newRelations(t)
	ctx := context.Background()
	bothNear(t, r, clk, 5, 0.9)
	req := nextEvent(t, r).Friendship

	if err := r.Confirm(ctx, req.ID, "g1"); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, r); e.Kind != Confirmed || e.Friendship.ConfirmedBy != "g1" {
		t.Fatalf("event = %+v", e)
	}
	if !friends(t, r, a, b) || !friends(t, r, b, a) {
		t.Fatal("confirmed pair must be friends both ways")
	}
	if !errors.Is(r.Confirm(ctx, req.ID, "g1"), ErrNotPending) {
		t.Fatal("confirming twice must fail")
	}
	if !errors.Is(r.Confirm(ctx, "nope", "g1"), ErrNotFound) {
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
	fs, _ := r.Friends(ctx, a)
	if len(fs) != 0 {
		t.Fatalf("Friends after remove = %+v", fs)
	}

	// 解除后再次贴近，可以重新发起请求。
	bothNear(t, r, clk, 5, 0.9)
	if e := nextEvent(t, r); e.Kind != Proposed || e.Friendship.ID != req.ID {
		t.Fatalf("event = %+v", e)
	}
}

func TestFilterFriendsAndAgents(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()
	bothNear(t, r, clk, 5, 0.9)
	_ = r.Confirm(ctx, nextEvent(t, r).Friendship.ID, "g1")
	nextEvent(t, r)

	got := r.FilterFriends(ctx, id.Device(a), []id.ParticipantID{id.Device(a), id.Device(b), id.Device(c)})
	if len(got) != 1 || got[0] != id.Device(b) {
		t.Fatalf("FilterFriends = %v", got)
	}
	// 智能体参与者按所属设备判断。
	if ok, _ := r.AreFriends(ctx, "agt:"+id.ParticipantID(b), id.Device(a)); !ok {
		t.Fatal("agent of a friend should count as friend")
	}
}
