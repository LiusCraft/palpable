package channel

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
)

var park = ZoneID("park")

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newChannels(t *testing.T, s store.Store) (*Channels, *clock) {
	t.Helper()
	c, err := New(context.Background(), s.Groups())
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)}
	c.now = clk.now
	c.DefineZone(park, "公园")
	return c, clk
}

func drain(c *Channels) []Event {
	var out []Event
	for {
		select {
		case e := <-c.Subscribe():
			out = append(out, e)
		default:
			return out
		}
	}
}

func kinds(evs []Event) []EventKind {
	var out []EventKind
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestZoneJoinLeave(t *testing.T) {
	c, _ := newChannels(t, store.NewMemory())
	ctx := context.Background()
	if err := c.JoinZone(ctx, a, "zone:nope"); !errors.Is(err, ErrNoSuchChannel) {
		t.Fatalf("unknown zone: %v", err)
	}
	must(t, c.JoinZone(ctx, a, park))
	must(t, c.JoinZone(ctx, b, park))
	evs := drain(c)
	if len(evs) != 2 || evs[0].Kind != Joined || evs[0].Channel.Name != "公园" {
		t.Fatalf("events = %+v", evs)
	}
	if fg, ok := c.ForegroundOf(id.Device(a)); !ok || fg != park {
		t.Fatalf("foreground = %v %v", fg, ok)
	}
	if m := c.Members(park); len(m) != 2 {
		t.Fatalf("members = %v", m)
	}
	must(t, c.JoinZone(ctx, a, park)) // 重复加入不发事件
	if evs := drain(c); len(evs) != 0 {
		t.Fatalf("events = %+v", evs)
	}

	must(t, c.LeaveZone(ctx, a))
	if k := kinds(drain(c)); len(k) != 1 || k[0] != Left {
		t.Fatalf("kinds = %v", k)
	}
	if _, ok := c.ForegroundOf(id.Device(a)); ok {
		t.Fatal("still has foreground after leaving")
	}
	if _, err := c.Current(ctx, a); !errors.Is(err, ErrNotInChannel) {
		t.Fatalf("Current: %v", err)
	}
}

func TestGroupCodeJoinSwitchAndPersistence(t *testing.T) {
	s := store.NewMemory()
	c, _ := newChannels(t, s)
	ctx := context.Background()

	must(t, c.JoinZone(ctx, a, park))
	code, err := c.CreateGroup(ctx, a, TTLToday)
	must(t, err)
	if len(code) != 4 {
		t.Fatalf("code = %q", code)
	}
	info, err := c.Current(ctx, a)
	must(t, err)
	if info.Channel.Kind != KindGroup || info.Channel.Code != code || !info.Foreground {
		t.Fatalf("current = %+v", info)
	}

	must(t, c.JoinGroup(ctx, b, code))
	if m := c.Members(info.Channel.ID); len(m) != 2 || m[0] != id.Device(a) {
		t.Fatalf("members = %v", m)
	}

	drain(c)
	must(t, c.Switch(ctx, a, "next"))
	if e := drain(c); len(e) != 1 || e[0].Kind != Switched || e[0].Channel.ID != park {
		t.Fatalf("switch next = %+v", e)
	}
	must(t, c.Switch(ctx, a, info.Channel.Name))
	if fg, _ := c.ForegroundOf(id.Device(a)); fg != info.Channel.ID {
		t.Fatalf("switch by name: fg = %v", fg)
	}
	if err := c.Switch(ctx, a, "不存在的群"); !errors.Is(err, ErrNoSuchChannel) {
		t.Fatalf("switch unknown: %v", err)
	}
	if l := c.List(ctx, a); len(l) != 2 || !l[0].Foreground {
		t.Fatalf("list = %+v", l)
	}

	// 重启后群与成员仍在。
	c2, _ := newChannels(t, s)
	if m := c2.Members(info.Channel.ID); len(m) != 2 {
		t.Fatalf("members after restart = %v", m)
	}

	// 离开前台群，前台回到公园。
	drain(c)
	must(t, c.Leave(ctx, a, info.Channel.ID))
	if k := kinds(drain(c)); len(k) != 2 || k[0] != Left || k[1] != Switched {
		t.Fatalf("leave kinds = %v", k)
	}
	if ms, _ := s.Groups().Members(ctx, info.Channel.ID); len(ms) != 1 {
		t.Fatalf("stored members = %+v", ms)
	}
}

func TestGroupExpiry(t *testing.T) {
	c, clk := newChannels(t, store.NewMemory())
	ctx := context.Background()
	code, err := c.CreateGroup(ctx, a, TTLTrip)
	must(t, err)
	clk.t = clk.t.Add(7 * time.Hour)
	if err := c.JoinGroup(ctx, b, code); !errors.Is(err, ErrBadCode) {
		t.Fatalf("expired code: %v", err)
	}
	drain(c)
	must(t, c.Sweep(ctx))
	if k := kinds(drain(c)); len(k) != 1 || k[0] != Left {
		t.Fatalf("sweep kinds = %v", k)
	}
	if _, ok := c.ForegroundOf(id.Device(a)); ok {
		t.Fatal("foreground left after expiry")
	}

	code, err = c.CreateGroup(ctx, a, TTLForever)
	must(t, err)
	clk.t = clk.t.Add(1000 * time.Hour)
	must(t, c.JoinGroup(ctx, b, code))
}

func TestWrongCodeRateLimit(t *testing.T) {
	c, clk := newChannels(t, store.NewMemory())
	ctx := context.Background()
	code, err := c.CreateGroup(ctx, a, TTLForever)
	must(t, err)
	wrong := "0000"
	if wrong == code {
		wrong = "0001"
	}
	for range badCodeLimit {
		if err := c.JoinGroup(ctx, b, wrong); !errors.Is(err, ErrBadCode) {
			t.Fatalf("wrong code: %v", err)
		}
	}
	if err := c.JoinGroup(ctx, b, code); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("limited: %v", err)
	}
	clk.t = clk.t.Add(61 * time.Second)
	must(t, c.JoinGroup(ctx, b, code))
}
