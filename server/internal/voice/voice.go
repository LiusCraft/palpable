// Package voice 是多人频道语音的核心，回答三件事：设备的话说给谁听、
// 频道里同时有人说话怎么办、怎么把声音送到好友设备。
//
// 它是唯一决定声音去向的地方：不在频道时说给 AI；在频道时说给同频道的好友，
// 陌生人的音频不转发。只透传 Opus，不解码、不混音。
package voice

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const (
	queueFrames  = 8                      // 每个听者 ≈ 480ms
	burstTail    = 600 * time.Millisecond // 说完后保留发言权，避免句中停顿被抢
	frameTimeout = 600 * time.Millisecond // 看门狗：这么久没有新帧视为说完
	aiTurnGrace  = 1500 * time.Millisecond
	aiTurnMax    = 30 * time.Second // AI 一直不回应时，回合最长保持这么久
	tick         = 100 * time.Millisecond
	eventBuffer  = 256
)

type Route int

const (
	RouteNone Route = iota
	RouteAI
	RouteChannel
)

type Source int

const (
	SourceNone Source = iota
	SourceAI
	SourcePeer
)

type AIState int

const (
	AIIdle AIState = iota
	AISpeaking
)

// 端口：由 app 注入。

type Membership interface {
	ForegroundOf(p id.ParticipantID) (id.ChannelID, bool)
	Members(ch id.ChannelID) []id.ParticipantID
}

type Friends interface {
	AreFriends(ctx context.Context, a, b id.ParticipantID) (bool, error)
}

type AI interface {
	Forward(d id.DeviceID, frame []byte)
	Interrupt(d id.DeviceID)
	Available(d id.DeviceID) bool
	Listen(d id.DeviceID, on bool)
}

type Sink interface {
	Send(p id.ParticipantID, m protocol.Message) error
	SendAudio(p id.ParticipantID, f []byte) error
}

// Names 给 speaker.state 带上发言者小伙伴的名字；可为 nil。
type Names func(d id.DeviceID) string

type Burst struct {
	Channel     id.ChannelID
	Speaker     id.ParticipantID
	Listeners   []id.ParticipantID
	StartedAt   time.Time
	LastFrameAt time.Time
	FirstInAt   time.Time // 首帧收到
	FirstOutAt  time.Time // 首帧交给听者队列
	stopAt      time.Time // mic.state stop 之后的拖尾截止；零值表示还在说
}

type EventKind int

const (
	BurstStarted EventKind = iota
	BurstEnded
)

type Event struct {
	Kind  EventKind
	Burst Burst
}

type deviceVoice struct {
	micOn       bool
	aiListening bool
	aiTurn      bool      // wake 之后的 AI 回合
	aiTurnUntil time.Time // AI 说完后的回合截止；零值表示等待 AI 回应
	aiTurnStart time.Time
	aiSpeaking  bool
	source      Source
	busyNotice  bool // 本句已提示过占线
	out         *outbox
}

type Router struct {
	members Membership
	friends Friends
	ai      AI
	sink    Sink
	names   Names
	now     func() time.Time
	events  chan Event

	// 一把锁保护全部状态；临界区里不做 I/O，下行经各听者的 outbox 发出。
	mu      sync.Mutex
	devices map[id.DeviceID]*deviceVoice
	bursts  map[id.ChannelID]*Burst
}

func New(members Membership, friends Friends, ai AI, sink Sink, names Names) *Router {
	if names == nil {
		names = func(id.DeviceID) string { return "" }
	}
	return &Router{
		members: members, friends: friends, ai: ai, sink: sink, names: names,
		now:     time.Now,
		events:  make(chan Event, eventBuffer),
		devices: map[id.DeviceID]*deviceVoice{},
		bursts:  map[id.ChannelID]*Burst{},
	}
}

func (r *Router) Subscribe() <-chan Event { return r.events }

// Run 运行看门狗：结束拖尾已过或断流的 Burst，直到 ctx 结束。
func (r *Router) Run(ctx context.Context) {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.expire()
		}
	}
}

func (r *Router) dev(d id.DeviceID) *deviceVoice {
	v := r.devices[d]
	if v == nil {
		v = &deviceVoice{out: newOutbox(id.Device(d), r.sink)}
		r.devices[d] = v
	}
	return v
}

// RouteOf 返回设备此刻麦克风音频的去向。
func (r *Router) RouteOf(d id.DeviceID) Route {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.routeLocked(d, r.now())
}

func (r *Router) routeLocked(d id.DeviceID, now time.Time) Route {
	v := r.devices[d]
	if v != nil && v.aiTurn {
		expired := (!v.aiTurnUntil.IsZero() && now.After(v.aiTurnUntil)) || now.Sub(v.aiTurnStart) > aiTurnMax
		if !expired {
			return RouteAI
		}
		v.aiTurn = false
	}
	if _, ok := r.members.ForegroundOf(id.Device(d)); ok {
		return RouteChannel
	}
	if r.ai.Available(d) {
		return RouteAI
	}
	return RouteNone
}

func (r *Router) OnMicState(d id.DeviceID, on bool) {
	now := r.now()
	var fx effects
	r.mu.Lock()
	v := r.dev(d)
	v.micOn = on
	route := r.routeLocked(d, now)
	if on {
		v.busyNotice = false
		if v.aiTurn {
			v.aiTurnUntil = time.Time{} // 又开口了：继续等 AI 回应
		}
		if route == RouteChannel {
			r.takeFloorLocked(d, now, &fx)
		}
	} else if ch, ok := r.members.ForegroundOf(id.Device(d)); ok {
		if b := r.bursts[ch]; b != nil && b.Speaker == id.Device(d) && b.stopAt.IsZero() {
			b.stopAt = now.Add(burstTail)
		}
	}
	r.syncListenLocked(d, route, &fx)
	r.mu.Unlock()
	fx.apply(r)
}

// OnWake 开始一个 AI 回合：在频道里叫小伙伴的名字，接下来这句话说给 AI。
func (r *Router) OnWake(d id.DeviceID) {
	now := r.now()
	var fx effects
	r.mu.Lock()
	v := r.dev(d)
	if r.ai.Available(d) {
		v.aiTurn, v.aiTurnUntil, v.aiTurnStart = true, time.Time{}, now
		// 正在频道里说话的话，先放下发言权。
		if ch, ok := r.members.ForegroundOf(id.Device(d)); ok {
			if b := r.bursts[ch]; b != nil && b.Speaker == id.Device(d) {
				r.endBurstLocked(ch, &fx)
			}
		}
	}
	r.syncListenLocked(d, r.routeLocked(d, now), &fx)
	r.mu.Unlock()
	fx.apply(r)
}

// OnMicFrame 热路径：按路由把一帧送给 AI、送给频道里的好友，或丢弃。
func (r *Router) OnMicFrame(d id.DeviceID, frame []byte) {
	now := r.now()
	var fx effects
	r.mu.Lock()
	route := r.routeLocked(d, now)
	switch route {
	case RouteAI:
		r.mu.Unlock()
		r.ai.Forward(d, frame)
		return
	case RouteChannel:
		ch, _ := r.members.ForegroundOf(id.Device(d))
		b := r.bursts[ch]
		if b == nil {
			b = r.takeFloorLocked(d, now, &fx)
		}
		if b == nil || b.Speaker != id.Device(d) {
			r.mu.Unlock()
			fx.apply(r)
			return
		}
		b.LastFrameAt = now
		if b.FirstInAt.IsZero() {
			b.FirstInAt = now
		}
		if !b.stopAt.IsZero() && r.dev(d).micOn {
			b.stopAt = time.Time{}
		}
		for _, l := range b.Listeners {
			r.dev(id.OwnerDevice(l)).out.pushFrame(frame)
		}
		if b.FirstOutAt.IsZero() && len(b.Listeners) > 0 {
			b.FirstOutAt = r.now()
		}
	}
	r.mu.Unlock()
	fx.apply(r)
}

// OnAIState 由 bridge/linx 回报：AI 开始或结束说话。
func (r *Router) OnAIState(d id.DeviceID, s AIState) {
	now := r.now()
	var fx effects
	r.mu.Lock()
	v := r.dev(d)
	switch s {
	case AISpeaking:
		v.aiSpeaking = true
		if v.source == SourcePeer {
			// 好友正在说话时优先好友，AI 让路。
			fx.interrupt = append(fx.interrupt, d)
		} else {
			v.source = SourceAI
		}
	case AIIdle:
		v.aiSpeaking = false
		if v.source == SourceAI {
			v.source = SourceNone
		}
		if v.aiTurn && v.aiTurnUntil.IsZero() {
			v.aiTurnUntil = now.Add(aiTurnGrace)
		}
	}
	r.mu.Unlock()
	fx.apply(r)
}

// OnFriendRemoved 立即把对方移出正在进行的转发。
func (r *Router) OnFriendRemoved(a, b id.DeviceID) {
	var fx effects
	r.mu.Lock()
	for _, bu := range r.bursts {
		sp := id.OwnerDevice(bu.Speaker)
		var other id.DeviceID
		switch sp {
		case a:
			other = b
		case b:
			other = a
		default:
			continue
		}
		r.dropListenerLocked(bu, other)
	}
	r.mu.Unlock()
	fx.apply(r)
}

// OnLeave 设备离开前台频道或切换前台：结束它的发言，并不再收听原频道。
func (r *Router) OnLeave(d id.DeviceID) {
	var fx effects
	r.mu.Lock()
	for ch, bu := range r.bursts {
		if bu.Speaker == id.Device(d) {
			r.endBurstLocked(ch, &fx)
			continue
		}
		if fg, ok := r.members.ForegroundOf(id.Device(d)); !ok || fg != ch {
			r.dropListenerLocked(bu, d)
		}
	}
	r.mu.Unlock()
	fx.apply(r)
}

// OnDisconnect 结束设备的一切语音状态并释放其下行 goroutine。
func (r *Router) OnDisconnect(d id.DeviceID) {
	var fx effects
	r.mu.Lock()
	for ch, bu := range r.bursts {
		if bu.Speaker == id.Device(d) {
			r.endBurstLocked(ch, &fx)
		} else {
			r.dropListenerLocked(bu, d)
		}
	}
	v := r.devices[d]
	delete(r.devices, d)
	r.mu.Unlock()
	fx.apply(r)
	if v != nil {
		v.out.close()
	}
}

// takeFloorLocked 频道空闲时让 d 拿到发言权；被占时提示占线并返回当前 Burst。
func (r *Router) takeFloorLocked(d id.DeviceID, now time.Time, fx *effects) *Burst {
	ch, ok := r.members.ForegroundOf(id.Device(d))
	if !ok {
		return nil
	}
	speaker := id.Device(d)
	if b := r.bursts[ch]; b != nil {
		if b.Speaker == speaker {
			b.stopAt = time.Time{}
			return b
		}
		v := r.dev(d)
		if !v.busyNotice {
			v.busyNotice = true
			fx.busy = append(fx.busy, d)
		}
		return b
	}

	b := &Burst{Channel: ch, Speaker: speaker, StartedAt: now, LastFrameAt: now}
	for _, m := range r.members.Members(ch) {
		if m == speaker {
			continue
		}
		if ok, _ := r.friends.AreFriends(context.Background(), speaker, m); ok {
			b.Listeners = append(b.Listeners, m)
		}
	}
	r.bursts[ch] = b
	start := &protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "start", Source: "peer", From: string(d), Name: r.names(d)}
	for _, l := range b.Listeners {
		ld := id.OwnerDevice(l)
		lv := r.dev(ld)
		if lv.aiSpeaking || lv.source == SourceAI {
			fx.interrupt = append(fx.interrupt, ld)
		}
		lv.source = SourcePeer
		fx.queued = append(fx.queued, queued{lv.out, start})
	}
	fx.events = append(fx.events, Event{Kind: BurstStarted, Burst: *b})
	return b
}

func (r *Router) endBurstLocked(ch id.ChannelID, fx *effects) {
	b := r.bursts[ch]
	if b == nil {
		return
	}
	delete(r.bursts, ch)
	for _, l := range b.Listeners {
		r.stopListenerLocked(b, id.OwnerDevice(l), fx)
	}
	fx.events = append(fx.events, Event{Kind: BurstEnded, Burst: *b})
	if !b.FirstInAt.IsZero() && !b.FirstOutAt.IsZero() {
		slog.Debug("burst ended", "channel", ch, "speaker", b.Speaker, "listeners", len(b.Listeners),
			"internal_latency_ms", b.FirstOutAt.Sub(b.FirstInAt).Milliseconds())
	}
}

func (r *Router) dropListenerLocked(b *Burst, d id.DeviceID) {
	p := id.Device(d)
	for i, l := range b.Listeners {
		if l == p {
			b.Listeners = append(b.Listeners[:i:i], b.Listeners[i+1:]...)
			var fx effects
			r.stopListenerLocked(b, d, &fx)
			// 只入队，不做 I/O，可在锁内完成。
			for _, q := range fx.queued {
				q.out.pushMsg(q.msg)
			}
			return
		}
	}
}

func (r *Router) stopListenerLocked(b *Burst, d id.DeviceID, fx *effects) {
	v := r.devices[d]
	if v == nil {
		return
	}
	if v.source == SourcePeer {
		v.source = SourceNone
	}
	stop := &protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "stop", Source: "peer", From: string(id.OwnerDevice(b.Speaker))}
	fx.queued = append(fx.queued, queued{v.out, stop})
}

// syncListenLocked 让灵矽只在"路由到 AI 且麦克风开着"时收音。
func (r *Router) syncListenLocked(d id.DeviceID, route Route, fx *effects) {
	v := r.dev(d)
	want := v.micOn && route == RouteAI
	if want != v.aiListening {
		v.aiListening = want
		fx.listen = append(fx.listen, listen{d, want})
	}
}

func (r *Router) expire() {
	now := r.now()
	var fx effects
	r.mu.Lock()
	for ch, b := range r.bursts {
		tailOver := !b.stopAt.IsZero() && now.After(b.stopAt)
		silent := now.Sub(b.LastFrameAt) > frameTimeout
		if tailOver || silent {
			r.endBurstLocked(ch, &fx)
		}
	}
	for d, v := range r.devices {
		if v.aiTurn {
			r.syncListenLocked(d, r.routeLocked(d, now), &fx)
		}
	}
	r.mu.Unlock()
	fx.apply(r)
}

// effects 收集锁内决定、锁外执行的动作。
type effects struct {
	interrupt []id.DeviceID
	busy      []id.DeviceID
	listen    []listen
	queued    []queued
	events    []Event
}

type listen struct {
	d  id.DeviceID
	on bool
}

type queued struct {
	out *outbox
	msg protocol.Message
}

func (fx *effects) apply(r *Router) {
	// 先打断 AI，再开始好友音频：同一设备同一时刻只有一种来源。
	for _, d := range fx.interrupt {
		r.ai.Interrupt(d)
	}
	for _, q := range fx.queued {
		q.out.pushMsg(q.msg)
	}
	for _, l := range fx.listen {
		r.ai.Listen(l.d, l.on)
	}
	for _, d := range fx.busy {
		p := id.Device(d)
		_ = r.sink.Send(p, &protocol.Sound{Type: protocol.TypeSound, Name: "busy"})
		_ = r.sink.Send(p, &protocol.Haptic{Type: protocol.TypeHaptic, Pattern: "tap"})
	}
	for _, e := range fx.events {
		select {
		case r.events <- e:
		default:
			slog.Error("voice event dropped: subscriber too slow", "kind", e.Kind)
		}
	}
}
