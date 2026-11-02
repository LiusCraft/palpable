package voice

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/kazzmir/opus-go/opus"

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
func (f *fakeMembers) joinTo(x id.DeviceID, ch id.ChannelID) {
	f.mu.Lock()
	f.fg[id.Device(x)] = ch
	f.mu.Unlock()
}
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
	case *protocol.Light:
		f.record(p, "light "+m.State)
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
	if got := g.sink.of(a); len(got) < 2 || got[len(got)-2] != "sound wake" || got[len(got)-1] != "light listening" {
		t.Fatalf("accepted wake must chime and light up, got %v", got)
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
	before := len(g.sink.of(a))
	g.r.OnWake(a)
	if len(g.sink.of(a)) != before {
		t.Fatal("wake without AI must not chime")
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
	if g.r.bursts[burstKey{park, id.Device(a)}] == nil {
		t.Fatal("kid should start speaking in the channel")
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

// 群靠邀请码手动加入，成员不是好友也互相听见；好友被解除不影响群内收听。
func TestGroupMembersHearWithoutFriendship(t *testing.T) {
	g := newRig(t)
	const grp id.ChannelID = "group:1"
	g.members.joinTo(a, grp)
	g.members.joinTo(c, grp) // a、c 不是好友
	g.r.OnMicState(a, true)
	if started := nextEvent(t, g.r); len(started.Burst.Listeners) != 1 {
		t.Fatalf("listeners = %+v", started.Burst.Listeners)
	}
	g.r.OnMicFrame(a, []byte{1})
	want := []string{"speaker start " + string(a), "frame 1"}
	eventually(t, func() bool { return equal(g.sink.of(c), want) }, "non-friend in group hears a")

	g.r.OnFriendRemoved(a, c)
	g.r.OnMicFrame(a, []byte{2})
	eventually(t, func() bool { return equal(g.sink.of(c), append(want, "frame 2")) }, "group listening unaffected")
}

// 3. 同时说话：不占线，听者收到混音，设备上只看到一次"有人在说话"。
func TestConcurrentSpeakersAreMixed(t *testing.T) {
	g := newRig(t)
	g.friends.pairs[[2]id.DeviceID{b, d}] = true
	for _, x := range []id.DeviceID{a, b, d} {
		g.members.join(x)
	}
	g.r.OnMicState(a, true)
	nextEvent(t, g.r)
	if g.r.mixers[park] != nil {
		t.Fatal("one speaker must stay pass-through")
	}
	g.r.OnMicFrame(a, []byte{1})
	eventually(t, func() bool { return equal(g.sink.of(d), []string{"speaker start " + string(a), "frame 1"}) }, "raw frame while alone")

	g.r.OnMicState(b, true)
	if e := nextEvent(t, g.r); e.Kind != BurstStarted || e.Burst.Speaker != id.Device(b) {
		t.Fatalf("b should start speaking at once: %+v", e)
	}
	if g.r.mixers[park] == nil {
		t.Fatal("two speakers must be mixed")
	}
	g.r.OnMicFrame(a, opusFrame(t))
	g.r.OnMicFrame(b, opusFrame(t))
	g.r.OnMicFrame(b, []byte{0xff}) // 坏包丢弃，不能让服务端 panic
	g.r.mixTick()
	eventually(t, func() bool { return len(g.sink.of(d)) == 3 }, "d hears the mix")
	eventually(t, func() bool { return len(g.sink.of(a)) == 2 }, "a hears b")
	for _, x := range []id.DeviceID{a, b} {
		for _, m := range g.sink.of(x) {
			if m == "sound busy" {
				t.Fatalf("%s told busy: %v", x, g.sink.of(x))
			}
		}
	}
	if starts := count(g.sink.of(d), "speaker start "+string(a)) + count(g.sink.of(d), "speaker start "+string(b)); starts != 1 {
		t.Fatalf("d saw %d starts: %v", starts, g.sink.of(d))
	}

	// a 说完：只剩 b，退回透传；d 还在听 b，不能收到 stop。
	g.r.OnMicState(a, false)
	g.clk.add(burstTail + tick)
	g.r.OnMicFrame(b, opusFrame(t)) // b 还在说，看门狗不能把它也结束
	g.r.expire()
	if g.r.mixers[park] != nil {
		t.Fatal("back to pass-through when one speaker is left")
	}
	g.r.OnMicFrame(b, []byte{2})
	eventually(t, func() bool { return count(g.sink.of(d), "frame 2") == 1 }, "d hears b raw again")
	if count(g.sink.of(d), "speaker stop "+string(a)) != 0 {
		t.Fatalf("d must not be told stop while b still talks: %v", g.sink.of(d))
	}
	g.r.OnMicState(b, false)
	g.clk.add(burstTail + tick)
	g.r.expire()
	eventually(t, func() bool { return count(g.sink.of(d), "speaker stop "+string(a)) == 1 }, "d told stop once everyone is done")
}

func count(got []string, want string) int {
	n := 0
	for _, s := range got {
		if s == want {
			n++
		}
	}
	return n
}

// opusFrame 是一帧真实的 16kHz 60ms Opus 包。
func opusFrame(t *testing.T) []byte {
	t.Helper()
	enc, err := opus.NewEncoder(16000, 1, opus.ApplicationVoIP)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = enc.Close() })
	pcm := make([]int16, 960)
	for i := range pcm {
		pcm[i] = int16(8000 * math.Sin(float64(i)/10))
	}
	buf := make([]byte, 400)
	n, err := enc.Encode(pcm, 960, buf)
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

// 句中停顿：拖尾内重新开口仍是同一段发言；发言人断线立即结束。
func TestBurstTailAndDisconnect(t *testing.T) {
	g := newRig(t)
	for _, x := range []id.DeviceID{a, b} {
		g.members.join(x)
	}
	g.r.OnMicState(a, true)
	g.r.OnMicState(a, false)
	g.clk.add(burstTail / 2)
	g.r.expire()
	g.r.OnMicState(a, true)
	g.clk.add(burstTail)
	g.r.OnMicFrame(a, []byte{1})
	g.r.expire()
	if g.r.bursts[burstKey{park, id.Device(a)}] == nil {
		t.Fatal("pause within tail must keep the burst")
	}
	g.r.OnDisconnect(a)
	if g.r.bursts[burstKey{park, id.Device(a)}] != nil {
		t.Fatal("speaker disconnect must end the burst")
	}
}

func TestWatchdogEndsSilentBurst(t *testing.T) {
	g := newRig(t)
	g.members.join(a)
	g.members.join(b)
	g.r.OnMicFrame(a, []byte{1}) // 没有 mic.state 也能开始
	g.clk.add(frameTimeout + tick)
	g.r.expire()
	if g.r.bursts[burstKey{park, id.Device(a)}] != nil {
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

// 小伙伴进频道：主人点名后，下一段 AI 语音作为 agt:<owner> 播给好友；陌生人、被屏蔽的听者听不到。
func TestAgentSpeaksToFriendsOnlyWhenArmed(t *testing.T) {
	g := newRig(t)
	for _, x := range []id.DeviceID{a, b, c, d} {
		g.members.join(x)
	}
	// 没点名：AI 说话只有主人自己听到。
	g.r.OnAIState(a, AISpeaking)
	g.r.OnAgentFrame(a, []byte{7})
	g.r.OnAIState(a, AIIdle)
	time.Sleep(20 * time.Millisecond)
	if len(g.sink.of(b)) != 0 {
		t.Fatalf("unarmed AI speech leaked to friend: %v", g.sink.of(b))
	}

	// 点名；d 的家长屏蔽了 a 的小伙伴。
	if err := g.r.ArmAgent(a, map[id.DeviceID]bool{d: true}); err != nil {
		t.Fatal(err)
	}
	g.r.OnAIState(a, AISpeaking)
	if e := nextEvent(t, g.r); e.Kind != BurstStarted || e.Burst.Speaker != id.Agent(a) || len(e.Burst.Listeners) != 1 || e.Burst.Listeners[0] != id.Device(b) {
		t.Fatalf("agent burst = %+v", e)
	}
	g.r.OnAgentFrame(a, []byte{8})
	g.r.OnAgentFrame(b, []byte{9}) // 别人的设备不算
	eventually(t, func() bool { return equal(g.sink.of(b), []string{"speaker start " + string(a), "frame 8"}) }, "friend hears agent")
	if len(g.sink.of(c)) != 0 || len(g.sink.of(d)) != 0 || len(g.sink.of(a)) != 0 {
		t.Fatalf("stranger %v, blocked %v, owner %v", g.sink.of(c), g.sink.of(d), g.sink.of(a))
	}
	// 小伙伴发言时朋友开口：两边同时说，小伙伴不被打断，也不占线。
	g.r.OnMicState(b, true)
	if e := nextEvent(t, g.r); e.Kind != BurstStarted || e.Burst.Speaker != id.Device(b) {
		t.Fatalf("friend burst = %+v", e)
	}
	if g.ai.has("interrupt " + string(a)) {
		t.Fatal("a friend talking must not interrupt the speaking agent")
	}
	for _, m := range g.sink.of(b) {
		if m == "sound busy" {
			t.Fatalf("friend told busy: %v", g.sink.of(b))
		}
	}
	g.r.OnMicState(b, false)
	g.clk.add(burstTail + tick)
	g.r.expire()
	if e := nextEvent(t, g.r); e.Kind != BurstEnded || e.Burst.Speaker != id.Device(b) {
		t.Fatalf("friend burst end = %+v", e)
	}

	g.r.OnAIState(a, AIIdle)
	if e := nextEvent(t, g.r); e.Kind != BurstEnded {
		t.Fatalf("want ended, got %+v", e)
	}
	eventually(t, func() bool {
		got := g.sink.of(b)
		return len(got) > 0 && got[len(got)-1] == "speaker stop "+string(a)
	}, "agent stop")
	// 一次点名只管一段话。
	g.r.OnAIState(a, AISpeaking)
	g.r.OnAgentFrame(a, []byte{10})
	time.Sleep(20 * time.Millisecond)
	for _, m := range g.sink.of(b) {
		if m == "frame 10" {
			t.Fatal("second utterance should be private")
		}
	}
}

func TestAgentArmRulesAndOwnerPreempts(t *testing.T) {
	g := newRig(t)
	if err := g.r.ArmAgent(a, nil); err != ErrNotInChannel {
		t.Fatalf("not in channel: %v", err)
	}
	g.members.join(a)
	g.members.join(b)
	// 点名后太久才开口：过期，不播。
	_ = g.r.ArmAgent(a, nil)
	g.clk.add(agentArmWindow)
	g.r.OnAIState(a, AISpeaking)
	g.r.OnAIState(a, AIIdle)
	select {
	case e := <-g.r.Subscribe():
		t.Fatalf("expired arm should not start a burst: %+v", e)
	default:
	}
	// 主人自己开口：小伙伴让路。
	_ = g.r.ArmAgent(a, nil)
	g.r.OnAIState(a, AISpeaking)
	nextEvent(t, g.r)
	g.r.OnMicState(a, true)
	if e := nextEvent(t, g.r); e.Kind != BurstEnded || e.Burst.Speaker != id.Agent(a) {
		t.Fatalf("owner should preempt agent: %+v", e)
	}
	if e := nextEvent(t, g.r); e.Kind != BurstStarted || e.Burst.Speaker != id.Device(a) {
		t.Fatalf("owner burst: %+v", e)
	}
	g.r.OnMicState(a, false)
	g.r.OnDisconnect(a)
	nextEvent(t, g.r)

	// 小伙伴发言断流太久：看门狗结束（比人说话的超时宽）。
	_ = g.r.ArmAgent(b, nil)
	g.r.OnAIState(b, AISpeaking)
	nextEvent(t, g.r)
	g.clk.add(frameTimeout + time.Millisecond)
	g.r.expire()
	select {
	case e := <-g.r.Subscribe():
		t.Fatalf("agent burst ended too early: %+v", e)
	default:
	}
	g.clk.add(agentFrameTimeout)
	g.r.expire()
	if e := nextEvent(t, g.r); e.Kind != BurstEnded {
		t.Fatalf("watchdog: %+v", e)
	}
}

// 孩子在 AI 说话中途开口：照常转给灵矽由它判断；再次唤醒则立即打断。
func TestBargeInDuringAISpeech(t *testing.T) {
	g := newRig(t)
	if g.r.RouteOf(a) != RouteAI {
		t.Fatal("no channel → AI")
	}
	g.r.OnMicState(a, true)
	g.r.OnMicState(a, false)
	if g.ai.has("interrupt " + string(a)) {
		t.Fatal("talking while AI is silent must not interrupt")
	}

	g.r.OnAIState(a, AISpeaking)
	g.r.OnMicState(a, true)
	if g.ai.has("interrupt " + string(a)) {
		t.Fatal("talking over AI must be left to linx to judge, not hard-interrupted")
	}
	if !g.ai.has("listen " + string(a) + " true") {
		t.Fatal("talking over AI must still be forwarded to linx")
	}
	g.r.OnMicState(a, false)
	g.r.OnAIState(a, AIIdle)

	g.ai.mu.Lock()
	g.ai.calls = nil
	g.ai.mu.Unlock()
	g.r.OnAIState(a, AISpeaking)
	g.r.OnWake(a)
	if !g.ai.has("interrupt " + string(a)) {
		t.Fatal("wake while AI speaks must interrupt it")
	}
}
