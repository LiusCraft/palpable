package relation

import (
	"context"
	"errors"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

func TestRequestFriendsAndRemoveEdges(t *testing.T) {
	r, clk, _ := newRelations(t)
	ctx := context.Background()

	if _, err := r.Request(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Request unknown: %v", err)
	}
	if err := r.Remove(ctx, a, c); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remove unknown pair: %v", err)
	}

	bothNear(t, r, clk, 5, 0.9)
	req := nextEvent(t, r).Friendship
	got, err := r.Request(ctx, req.ID)
	if err != nil || got.ID != req.ID || got.Other(a) != b || got.Other(b) != a {
		t.Fatalf("Request = %+v %v", got, err)
	}
	fs, err := r.Friends(ctx, b)
	if err != nil || len(fs) != 1 || fs[0].Status != StatusPending {
		t.Fatalf("Friends with pending = %+v %v", fs, err)
	}

	// 拒绝待确认请求：Remove 也适用，且再次 Remove 不重复发事件。
	if err := r.Remove(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	nextEvent(t, r)
	if err := r.Remove(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	noEvent(t, r)

	// 自己与自己、无法识别的参与者都不是好友。
	if ok, _ := r.AreFriends(ctx, id.Device(a), id.Device(a)); ok {
		t.Fatal("self is not a friend")
	}
	if ok, _ := r.AreFriends(ctx, "garbage", id.Device(a)); ok {
		t.Fatal("unknown participant is not a friend")
	}
}

func TestEmitDropsWhenSubscriberIsSlow(t *testing.T) {
	r, _, _ := newRelations(t)
	for range eventBuffer + 3 {
		r.emit(Event{Kind: Proposed})
	}
	if len(r.events) != eventBuffer {
		t.Fatalf("buffered = %d", len(r.events))
	}
}

// 存储出错时向上返回，不吞错误。
type failingRepo struct{ store.FriendshipRepo }

var errBoom = errors.New("boom")

func (failingRepo) ListByStatus(context.Context, int) ([]store.Friendship, error) {
	return nil, errBoom
}
func (failingRepo) ListByDevice(context.Context, id.DeviceID) ([]store.Friendship, error) {
	return nil, errBoom
}
func (failingRepo) Get(context.Context, string) (store.Friendship, error) {
	return store.Friendship{}, errBoom
}
func (failingRepo) ByPair(context.Context, id.DeviceID, id.DeviceID) (store.Friendship, error) {
	return store.Friendship{}, errBoom
}

func TestStoreErrorsPropagate(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, failingRepo{}); !errors.Is(err, errBoom) {
		t.Fatalf("New: %v", err)
	}
	r, _, _ := newRelations(t)
	r.repo = failingRepo{FriendshipRepo: r.repo}
	if _, err := r.Friends(ctx, a); !errors.Is(err, errBoom) {
		t.Fatalf("Friends: %v", err)
	}
	if _, err := r.Request(ctx, "x"); !errors.Is(err, errBoom) {
		t.Fatalf("Request: %v", err)
	}
	if err := r.Confirm(ctx, "x", "g"); !errors.Is(err, errBoom) {
		t.Fatalf("Confirm: %v", err)
	}
	if err := r.Remove(ctx, a, b); !errors.Is(err, errBoom) {
		t.Fatalf("Remove: %v", err)
	}
	if err := r.propose(ctx, a, b); !errors.Is(err, errBoom) {
		t.Fatalf("propose: %v", err)
	}
}
