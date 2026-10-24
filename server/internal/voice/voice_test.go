package voice

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const (
	a id.DeviceID = "02:00:00:00:00:0a" // 发言人
	b id.DeviceID = "02:00:00:00:00:0b" // a 的好友
	c id.DeviceID = "02:00:00:00:00:0c" // 陌生人
	d id.DeviceID = "02:00:00:00:00:0d" // a 的另一个好友
)

const park id.ChannelID = "zone:park"

type fakeMembers struct {
	mu sync.Mutex
	fg map[id.ParticipantID]id.ChannelID
}

func (f *fakeMembers) ForegroundOf(p id.ParticipantID) (id.ChannelID, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch, ok := f.fg[p]
	return ch, ok
}

func (f *fakeMembers) Members(ch id.ChannelID) []id.ParticipantID {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []id.ParticipantID
	for _, x := range []id.DeviceID{a, b, c, d} {
		if f.fg[id.Device(x)] == ch {
			out = append(out, id.Device(x))
		}
	}
	return out
}

func (f *fakeMembers) join(x id.DeviceID) { f.mu.Lock(); f.fg[id.Device(x)] = park; f.mu.Unlock() }
func (f *fakeMembers) leave(x id.DeviceID) {
	f.mu.Lock()
	delete(f.fg, id.Device(x))
	f.mu.Unlock()
}

type fakeFriends struct {
	mu    sync.Mutex
	pairs map[[2]id.DeviceID]bool
}

func (f *fakeFriends) AreFriends(_ context.Context, x, y id.ParticipantID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dx, dy := id.OwnerDevice(x), id.OwnerDevice(y)
	return f.pairs[[2]id.DeviceID{dx, dy}] || f.pairs[[2]id.DeviceID{dy, dx}], nil
}

func (f *fakeFriends) unfriend(x, y id.DeviceID) {
	f.mu.Lock()
	delete(f.pairs, [2]id.DeviceID{x, y})
	delete(f.pairs, [2]id.DeviceID{y, x})
	f.mu.Unlock()
}

type fakeAI struct {
	mu        sync.Mutex
	available bool
	calls     []string
	forwarded int
}

func (f *fakeAI) Forward(id.DeviceID, []byte) { f.mu.Lock(); f.forwarded++; f.mu.Unlock() }
func (f *fakeAI) Interrupt(x id.DeviceID)     { f.log("interrupt " + string(x)) }
func (f *fakeAI) Available(id.DeviceID) bool  { f.mu.Lock(); defer f.mu.Unlock(); return f.available }
func (f *fakeAI) Listen(x id.DeviceID, on bool) {
	f.log(fmt.Sprintf("listen %s %v", x, on))
}
func (f *fakeAI) log(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fakeAI) fwd() int     { f.mu.Lock(); defer f.mu.Unlock(); return f.forwarded }
func (f *fakeAI) has(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == s {
			return true
		}
	}
	return false
}

// fakeSink 记录每个参与者收到的消息与帧；block 中的参与者发送会卡住，模拟慢听者。
type fakeSink struct {
	mu    sync.Mutex
	got   map[id.ParticipantID][]string
	block map[id.ParticipantID]chan struct{}
}

func (f *fakeSink) record(p id.ParticipantID, s string) {
	f.mu.Lock()
	ch := f.block[p]
	f.mu.Unlock()
	if ch != nil {
		<-ch
	}
	f.mu.Lock()
	f.got[p] = append(f.got[p], s)
	f.mu.Unlock()
}

func (f *fakeSink) Send(p id.ParticipantID, m protocol.Message) error {
	switch m := m.(type) {
	case *protocol.SpeakerState:
		f.record(p, "speaker "+m.State+" "+m.From)
	case *protocol.Sound:
		f.record(p, "sound "+m.Name)
	case *protocol.Haptic:
		f.record(p, "haptic "+m.Pattern)
	}
	return nil
}

func (f *fakeSink) SendAudio(p id.ParticipantID, frame []byte) error {
	f.record(p, fmt.Sprintf("frame %d", frame[0]))
	return nil
}

func (f *fakeSink) of(x id.DeviceID) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got[id.Device(x)]...)
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(dt time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(dt)
	c.mu.Unlock()
}

type rig struct {
	r       *Router
	members *fakeMembers
	friends *fakeFriends
	ai      *fakeAI
	sink    *fakeSink
	clk     *clock
}

func newRig(t *testing.T) *rig {
	g := &rig{
		members: &fakeMembers{fg: map[id.ParticipantID]id.ChannelID{}},
		friends: &fakeFriends{pairs: map[[2]id.DeviceID]bool{{a, b}: true, {a, d}: true}},
		ai:      &fakeAI{available: true},
		sink:    &fakeSink{got: map[id.ParticipantID][]string{}, block: map[id.ParticipantID]chan struct{}{}},
		clk:     &clock{t: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)},
	}
	g.r = New(g.members, g.friends, g.ai, g.sink, func(x id.DeviceID) string { return "name-" + string(x[len(x)-1]) })
	g.r.now = g.clk.now
	t.Cleanup(func() {
		for _, x := range []id.DeviceID{a, b, c, d} {
			g.r.OnDisconnect(x)
		}
	})
	return g
}

func eventually(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func equal(x, y []string) bool { return fmt.Sprint(x) == fmt.Sprint(y) }

func nextEvent(t *testing.T, r *Router) Event {
	t.Helper()
	select {
	case e := <-r.Subscribe():
		return e
	case <-time.After(time.Second):
		t.Fatal("no voice event")
		return Event{}
	}
}

// 1. 路由
func TestRouting(t *testing.T) {
	g := newRig(t)
	if g.r.RouteOf(a) != RouteAI {
		t.Fatal("not in channel → AI")
	}
	g.r.OnMicState(a, true)
	if !g.ai.has("listen " + string(a) + " true") {
		t.Fatal("mic on with AI route must start AI listening")
	}
	g.r.OnMicFrame(a, []byte{1})
	if g.ai.fwd() != 1 {
		t.Fatal("frame not forwarded to AI")
	}
	g.r.OnMicState(a, false)

	g.members.join(a)
	if g.r.RouteOf(a) != RouteChannel {
		t.Fatal("in channel → channel")
	}

	g.r.OnWake(a)
	if g.r.RouteOf(a) != RouteAI {
		t.Fatal("wake → AI for one turn")
	}
	g.r.OnAIState(a, AISpeaking)
	g.r.OnAIState(a, AIIdle)
	g.clk.add(time.Second)
	if g.r.RouteOf(a) != RouteAI {
		t.Fatal("still AI within 1.5s after tts stop")
	}
	g.clk.add(time.Second)
	if g.r.RouteOf(a) != RouteChannel {
		t.Fatal("back to channel after 1.5s of silence")
	}

	g.members.leave(a)
	g.ai.available = false
	if g.r.RouteOf(a) != RouteNone {
		t.Fatal("no channel and no AI → drop")
	}
}

// 加入频道后小伙伴还在播报时孩子开口：打断播报，话进频道而不是进 AI。
func TestSpeakingInterruptsOwnAIAnnouncement(t *testing.T) {
	g := newRig(t)
	g.members.join(a)
	g.r.OnAIState(a, AISpeaking)

	g.r.OnMicState(a, true)
	if !g.ai.has("interrupt " + string(a)) {
		t.Fatal("kid speaking must interrupt own AI announcement")
	}
	g.r.OnMicFrame(a, []byte{1})
	if g.ai.fwd() != 0 {
		t.Fatal("speech after joining must go to the channel, not the AI")
	}
	if bu := g.r.bursts[park]; bu == nil || bu.Speaker != id.Device(a) {
		t.Fatal("kid should hold the floor")
	}
}

// 2. 屏蔽 + 6. 延迟时间戳
func TestOnlyFriendsHearAndRemovalCutsOff(t *testing.T) {
	g := newRig(t)
	for _, x := range []id.DeviceID{a, b, c, d} {
		g.members.join(x)
	}
	g.r.OnMicState(a, true)
	started := nextEvent(t, g.r)
	if started.Kind != BurstStarted || len(started.Burst.Listeners) != 2 {
		t.Fatalf("listeners = %+v", started.Burst.Listeners)
	}
	g.r.OnMicFrame(a, []byte{1})

	want := []string{"speaker start " + string(a), "frame 1"}
	eventually(t, func() bool { return equal(g.sink.of(b), want) }, "friend b hears a")
	eventually(t, func() bool { return equal(g.sink.of(d), want) }, "friend d hears a")
	if got := g.sink.of(c); len(got) != 0 {
		t.Fatalf("stranger must hear nothing: %v", got)
	}

	g.friends.unfriend(a, b)
	g.r.OnFriendRemoved(a, b)
	g.r.OnMicFrame(a, []byte{2})
	eventually(t, func() bool {
		return equal(g.sink.of(b), append(want, "speaker stop "+string(a)))
	}, "b is cut off immediately")
	eventually(t, func() bool { return equal(g.sink.of(d), append(want, "frame 2")) }, "d keeps hearing")

	g.r.OnMicState(a, false)
	g.clk.add(burstTail + tick)
	g.r.expire()
	ended := nextEvent(t, g.r)
	if ended.Kind != BurstEnded || ended.Burst.FirstInAt.IsZero() || ended.Burst.FirstOutAt.IsZero() {
		t.Fatalf("ended = %+v", ended)
	}
}

// 3. 发言权
func TestFloorBusyAndRelease(t *testing.T) {
	g := newRig(t)
	g.friends.pairs[[2]id.DeviceID{b, d}] = true
	for _, x := range []id.DeviceID{a, b, d} {
		g.members.join(x)
	}
	g.r.OnMicState(a, true)
	g.r.OnMicState(b, true)
	g.r.OnMicFrame(b, []byte{9})
	g.r.OnMicFrame(b, []byte{9})
	eventually(t, func() bool { return len(g.sink.of(b)) == 3 }, "b hears a and is told busy")
	counts := map[string]int{}
	for _, got := range g.sink.of(b) {
		counts[got]++
	}
	if counts["sound busy"] != 1 || counts["haptic tap"] != 1 || counts["speaker start "+string(a)] != 1 {
		t.Fatalf("b got %v; busy must be told once per sentence", g.sink.of(b))
	}
	for _, got := range g.sink.of(d) {
		if got == "frame 9" {
			t.Fatal("b's frames must be dropped while a holds the floor")
		}
	}

	// 句中停顿：拖尾内重新开口，发言权仍是 a 的。
	g.r.OnMicState(a, false)
	g.clk.add(burstTail / 2)
	g.r.expire()
	g.r.OnMicState(a, true)
	g.clk.add(burstTail)
	g.r.OnMicFrame(a, []byte{1})
	g.r.expire()
	if fg := g.r.bursts[park]; fg == nil || fg.Speaker != id.Device(a) {
		t.Fatal("pause within tail must keep the floor")
	}

	// 发言人断线立即释放。
	g.r.OnDisconnect(a)
	if g.r.bursts[park] != nil {
		t.Fatal("speaker disconnect must end the burst")
	}
	g.r.OnMicState(b, true)
	if bu := g.r.bursts[park]; bu == nil || bu.Speaker != id.Device(b) {
		t.Fatal("b should get the floor after a left")
	}
}

func TestWatchdogEndsSilentBurst(t *testing.T) {
	g := newRig(t)
	g.members.join(a)
	g.members.join(b)
	g.r.OnMicFrame(a, []byte{1}) // 没有 mic.state 也能开始
	g.clk.add(frameTimeout + tick)
	g.r.expire()
	if g.r.bursts[park] != nil {
		t.Fatal("silent burst must end")
	}
}

// 4. 优先级
func TestFriendInterruptsAI(t *testing.T) {
	g := newRig(t)
	g.members.join(a)
	g.members.join(b)
	g.r.OnAIState(b, AISpeaking)
	g.r.OnMicState(a, true)
	if !g.ai.has("interrupt " + string(b)) {
		t.Fatal("friend talking must interrupt AI on the listener")
	}

	// 好友说话期间 AI 又开口：AI 让路。
	g.ai.calls = nil
	g.r.OnAIState(b, AISpeaking)
	if !g.ai.has("interrupt " + string(b)) {
		t.Fatal("AI starting during peer audio must be interrupted")
	}
}

// 5. 背压
func TestSlowListenerDoesNotBlockOthers(t *testing.T) {
	g := newRig(t)
	for _, x := range []id.DeviceID{a, b, d} {
		g.members.join(x)
	}
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release) // 失败时也放开，避免清理时卡住
	g.sink.block[id.Device(b)] = gate

	g.r.OnMicState(a, true)
	for i := range 20 {
		g.r.OnMicFrame(a, []byte{byte(i)})
	}
	eventually(t, func() bool {
		got := g.sink.of(d)
		return len(got) > 0 && got[len(got)-1] == "frame 19"
	}, "fast listener is not held up by the slow one")

	g.sink.mu.Lock()
	delete(g.sink.block, id.Device(b))
	g.sink.mu.Unlock()
	release()
	eventually(t, func() bool {
		got := g.sink.of(b)
		return len(got) > 0 && got[len(got)-1] == "frame 19"
	}, "slow listener catches up with the newest frame")
	if n := len(g.sink.of(b)); n > 1+1+queueFrames {
		t.Fatalf("slow listener got %d items; queue must drop oldest frames", n)
	}
}
