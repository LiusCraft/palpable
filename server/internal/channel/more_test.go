package channel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

func TestLeaveListMembersEdges(t *testing.T) {
	c, _ := newChannels(t, store.NewMemory())
	ctx := context.Background()
	if err := c.Leave(ctx, a, "group:nope"); !errors.Is(err, ErrNoSuchChannel) {
		t.Fatalf("leave unknown: %v", err)
	}
	if err := c.Leave(ctx, a, park); err != nil {
		t.Fatalf("leaving a channel you are not in is a no-op: %v", err)
	}
	if c.Members("zone:nope") != nil || c.List(ctx, a) != nil {
		t.Fatal("unknown channel / participant")
	}
	if err := c.Switch(ctx, a, "next"); !errors.Is(err, ErrNotInChannel) {
		t.Fatalf("switch without channels: %v", err)
	}

	must(t, c.JoinZone(ctx, a, park))
	must(t, c.Leave(ctx, a, park))
	if _, ok := c.ForegroundOf(id.Device(a)); ok {
		t.Fatal("left zone via Leave")
	}

	// 重新定义已存在的区域只改名字。
	c.DefineZone(park, "大公园")
	must(t, c.JoinZone(ctx, a, park))
	if info, _ := c.Current(ctx, a); info.Channel.Name != "大公园" {
		t.Fatalf("renamed zone = %+v", info)
	}

	// prev 在只有一个频道时回到自己。
	drain(c)
	must(t, c.Switch(ctx, a, "prev"))
	if len(drain(c)) != 0 {
		t.Fatal("switching to the current channel emits nothing")
	}
}

func TestEmitDropsAndCodeExhaustion(t *testing.T) {
	c, clk := newChannels(t, store.NewMemory())
	for range eventBuffer + 2 {
		c.emit(Event{Kind: Joined})
	}
	if len(c.events) != eventBuffer {
		t.Fatalf("buffered = %d", len(c.events))
	}

	// 9000 个码全被占用时拿不到新码。
	for i := range 9000 {
		code := string(rune('0'+(i+1000)/1000%10)) + string(rune('0'+(i+1000)/100%10)) + string(rune('0'+(i+1000)/10%10)) + string(rune('0'+(i+1000)%10))
		ch := id.ChannelID("group:" + code)
		c.chans[ch] = &chanState{ch: Channel{ID: ch, Kind: KindGroup, Code: code}, members: map[id.ParticipantID]time.Time{}}
	}
	if _, err := c.freeCodeLocked(clk.now()); !errors.Is(err, ErrNoCodesAvailable) {
		t.Fatalf("expected no codes: %v", err)
	}
}

type brokenGroups struct{ store.GroupRepo }

var errBoom = errors.New("boom")

func (brokenGroups) DeleteExpired(context.Context, time.Time) (int, error) { return 0, errBoom }
func (brokenGroups) Create(context.Context, store.Group) error             { return errBoom }
func (brokenGroups) AddMember(context.Context, id.ChannelID, store.Member) error {
	return errBoom
}
func (brokenGroups) RemoveMember(context.Context, id.ChannelID, id.ParticipantID) error {
	return errBoom
}

func TestStoreErrorsPropagate(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, brokenGroups{}); !errors.Is(err, errBoom) {
		t.Fatalf("New: %v", err)
	}
	s := store.NewMemory()
	c, _ := newChannels(t, s)
	code, err := c.CreateGroup(ctx, a, TTLForever)
	must(t, err)
	c.repo = brokenGroups{GroupRepo: s.Groups()}
	if _, err := c.CreateGroup(ctx, a, TTLToday); !errors.Is(err, errBoom) {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := c.JoinGroup(ctx, b, code); !errors.Is(err, errBoom) {
		t.Fatalf("JoinGroup: %v", err)
	}
	info, _ := c.Current(ctx, a)
	if err := c.Leave(ctx, a, info.Channel.ID); !errors.Is(err, errBoom) {
		t.Fatalf("Leave: %v", err)
	}
	if err := c.Sweep(ctx); !errors.Is(err, errBoom) {
		t.Fatalf("Sweep: %v", err)
	}
}
