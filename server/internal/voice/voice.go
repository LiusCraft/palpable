// Package voice 是多人频道语音的核心，回答三件事：设备的话说给谁听、
// 频道里同时有人说话怎么办、怎么把声音送到好友设备。
//
// 它是唯一决定声音去向的地方：不在频道时说给 AI；在频道时说给同频道的听者（区域频道是好友，
// 群是全体成员），陌生人的音频不转发。频道里只有一个人说话时原样透传 Opus；
// 多人同时说话时由 mixer 解码、混成一路再编码下发。
package voice

import (
	"context"
	"errors"
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

	// 小伙伴被点名后，这么久内开口的那一段话才播给频道。
	agentArmWindow = 20 * time.Second
	// TTS 句间停顿比人说话长；小伙伴的发言以 AI 说完为准，断流看门狗放宽。
	agentFrameTimeout = 3 * time.Second
)

var ErrNotInChannel = errors.New("voice: not in any channel")

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
	// hearing 是此刻正在给它送声音的发言数；0→1 时发 speaker.state start，回到 0 时发 stop，
	// 设备上看到的仍是"有人在说话"。heardSource/heardFrom 记下 start 时的内容，stop 要与之一致。
	hearing     int
	heardSource string
	heardFrom   id.DeviceID
	out         *outbox

	// 主人点名让小伙伴对频道说话：下一段 AI 语音作为 agt:<mac> 的发言。
	agentArmedUntil time.Time
	agentBlocked    map[id.DeviceID]bool // 这些听者的家长屏蔽了它
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
	bursts  map[burstKey]*Burst
	mixers  map[id.ChannelID]*chanMix // 频道里同时有 ≥2 人说话时才有
}

// burstKey：一个频道里每个发言者各有一段发言，可以同时进行。
type burstKey struct {
	ch      id.ChannelID
	speaker id.ParticipantID
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
		bursts:  map[burstKey]*Burst{},
		mixers:  map[id.ChannelID]*chanMix{},
	}
}

func (r *Router) Subscribe() <-chan Event { return r.events }

// Run 运行看门狗：结束拖尾已过或断流的 Burst，直到 ctx 结束。
func (r *Router) Run(ctx context.Context) {
	t := time.NewTicker(tick)
	defer t.Stop()
	m := time.NewTicker(mixFrame * time.Second / mixRate)
	defer m.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.expire()
		case <-m.C:
			r.mixTick()
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
		if v.aiTurn {
			v.aiTurnUntil = time.Time{} // 又开口了：继续等 AI 回应
		}
		if route == RouteChannel {
			r.endAgentLocked(d, &fx) // 主人开口：自己的小伙伴让路
			r.takeFloorLocked(d, now, &fx)
		}
		// AI 说话时孩子开口不在这里硬打断：音频照常转给灵矽，由它结合识别结果判断是否打断
		// （interrupt_probe），回声残留与杂音不会让小伙伴自己停下。
	} else if ch, ok := r.members.ForegroundOf(id.Device(d)); ok {
		if b := r.bursts[burstKey{ch, id.Device(d)}]; b != nil && b.stopAt.IsZero() {
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
		// 小伙伴正在说话时再叫它：先停下来听孩子说。
		if v.aiSpeaking || v.source == SourceAI {
			fx.interrupt = append(fx.interrupt, d)
		}
		fx.woke = append(fx.woke, d)
		// 正在频道里说话的话，先放下发言权。
		if ch, ok := r.members.ForegroundOf(id.Device(d)); ok {
			r.endBurstLocked(burstKey{ch, id.Device(d)}, &fx)
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
		b := r.bursts[burstKey{ch, id.Device(d)}]
		if b == nil {
			b = r.takeFloorLocked(d, now, &fx)
		}
		if b == nil {
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
		mx := r.mixers[ch]
		if mx == nil {
			for _, l := range b.Listeners {
				r.dev(id.OwnerDevice(l)).out.pushFrame(frame)
			}
		}
		if b.FirstOutAt.IsZero() && len(b.Listeners) > 0 {
			b.FirstOutAt = r.now()
		}
		r.mu.Unlock()
		fx.apply(r)
		if mx != nil {
			mx.push(b.Speaker, frame)
		}
		return
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
		if !v.agentArmedUntil.IsZero() && now.Before(v.agentArmedUntil) {
			v.agentArmedUntil = time.Time{}
			r.startAgentLocked(d, now, &fx)
		}
		if v.source == SourcePeer && !r.agentActiveLocked(d) {
			// 好友正在说话时优先好友，AI 让路；小伙伴正在对频道说话时不让，只有主人开口才打断它。
			fx.interrupt = append(fx.interrupt, d)
		} else {
			v.source = SourceAI
		}
	case AIIdle:
		v.aiSpeaking = false
		r.endAgentLocked(d, &fx)
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

// ArmAgent 主人点名让小伙伴对频道说话（self.channel.speak）：接下来这一段 AI 语音会作为
// agt:<owner> 播给前台频道里的好友。blocked 是家长屏蔽了它的听者，由调用方在锁外算好。
func (r *Router) ArmAgent(owner id.DeviceID, blocked map[id.DeviceID]bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members.ForegroundOf(id.Device(owner)); !ok {
		return ErrNotInChannel
	}
	v := r.dev(owner)
	v.agentArmedUntil, v.agentBlocked = r.now().Add(agentArmWindow), blocked
	return nil
}

// OnAgentFrame 热路径：主人设备收到的 AI 下行帧；小伙伴正在频道发言时同样转给听者。
func (r *Router) OnAgentFrame(owner id.DeviceID, frame []byte) {
	r.mu.Lock()
	ch, ok := r.members.ForegroundOf(id.Device(owner))
	if !ok {
		r.mu.Unlock()
		return
	}
	b := r.bursts[burstKey{ch, id.Agent(owner)}]
	if b == nil {
		r.mu.Unlock()
		return
	}
	now := r.now()
	b.LastFrameAt = now
	if b.FirstInAt.IsZero() {
		b.FirstInAt = now
	}
	mx := r.mixers[ch]
	if mx == nil {
		for _, l := range b.Listeners {
			r.dev(id.OwnerDevice(l)).out.pushFrame(frame)
		}
	}
	if b.FirstOutAt.IsZero() && len(b.Listeners) > 0 {
		b.FirstOutAt = now
	}
	r.mu.Unlock()
	if mx != nil {
		mx.push(b.Speaker, frame)
	}
}

// startAgentLocked 让小伙伴对频道说话，可以和别人同时说。主人自己直接听小伙伴的 TTS，
// 所以把主人从频道里其他人的听者中移出，免得两路声音在他的设备上串在一起。
func (r *Router) startAgentLocked(owner id.DeviceID, now time.Time, fx *effects) {
	ch, ok := r.members.ForegroundOf(id.Device(owner))
	if !ok || r.bursts[burstKey{ch, id.Agent(owner)}] != nil {
		return
	}
	blocked := r.dev(owner).agentBlocked
	r.startBurstLocked(ch, id.Agent(owner), now, fx, func(l id.DeviceID) bool { return !blocked[l] })
	for k, b := range r.bursts {
		if k.ch == ch && k.speaker != id.Agent(owner) {
			r.dropListenerLocked(b, owner)
		}
	}
}

func (r *Router) agentActiveLocked(owner id.DeviceID) bool {
	ch, ok := r.members.ForegroundOf(id.Device(owner))
	if !ok {
		return false
	}
	_, active := r.bursts[burstKey{ch, id.Agent(owner)}]
	return active
}

func (r *Router) endAgentLocked(owner id.DeviceID, fx *effects) {
	r.dev(owner).agentArmedUntil = time.Time{}
	if ch, ok := r.members.ForegroundOf(id.Device(owner)); ok {
		r.endBurstLocked(burstKey{ch, id.Agent(owner)}, fx)
	}
}

// OnFriendRemoved 立即把对方移出正在进行的转发；群里不靠好友关系收听，不受影响。
func (r *Router) OnFriendRemoved(a, b id.DeviceID) {
	var fx effects
	r.mu.Lock()
	for k, bu := range r.bursts {
		if k.ch.IsGroup() {
			continue
		}
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
	for k, bu := range r.bursts {
		if id.OwnerDevice(bu.Speaker) == d {
			r.endBurstLocked(k, &fx)
			continue
		}
		if fg, ok := r.members.ForegroundOf(id.Device(d)); !ok || fg != k.ch {
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
	for k, bu := range r.bursts {
		if id.OwnerDevice(bu.Speaker) == d {
			r.endBurstLocked(k, &fx)
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

// takeFloorLocked 让 d 开始一段发言。频道里可以多人同时说话，不再有占线。
func (r *Router) takeFloorLocked(d id.DeviceID, now time.Time, fx *effects) *Burst {
	ch, ok := r.members.ForegroundOf(id.Device(d))
	if !ok {
		return nil
	}
	speaker := id.Device(d)
	if b := r.bursts[burstKey{ch, speaker}]; b != nil {
		b.stopAt = time.Time{}
		return b
	}

	// 加入频道后小伙伴的确认播报还没说完时孩子开口了：孩子优先，直接打断播报。
	if sv := r.dev(d); sv.aiSpeaking || sv.source == SourceAI {
		fx.interrupt = append(fx.interrupt, d)
	}

	return r.startBurstLocked(ch, speaker, now, fx, nil)
}

// startBurstLocked 开始一段发言：听者是同频道里的设备。区域频道只给与发言者是好友的设备（小伙伴按主人判断）；
// 群是凭邀请码手动加入的，成员不要求是好友。allow 非 nil 时再按家长控制过滤。
// 正在对频道说话的小伙伴的主人直接听自己的小伙伴，不是听者。
func (r *Router) startBurstLocked(ch id.ChannelID, speaker id.ParticipantID, now time.Time, fx *effects, allow func(id.DeviceID) bool) *Burst {
	owner := id.OwnerDevice(speaker)
	b := &Burst{Channel: ch, Speaker: speaker, StartedAt: now, LastFrameAt: now}
	for _, m := range r.members.Members(ch) {
		md := id.OwnerDevice(m)
		if md == owner || (allow != nil && !allow(md)) {
			continue
		}
		if _, speaking := r.bursts[burstKey{ch, id.Agent(md)}]; speaking {
			continue
		}
		if ch.IsGroup() {
			b.Listeners = append(b.Listeners, m)
		} else if ok, _ := r.friends.AreFriends(context.Background(), speaker, m); ok {
			b.Listeners = append(b.Listeners, m)
		}
	}
	r.bursts[burstKey{ch, speaker}] = b
	for _, l := range b.Listeners {
		ld := id.OwnerDevice(l)
		lv := r.dev(ld)
		lv.hearing++
		if lv.hearing > 1 {
			continue // 已经在听别人说话，设备上本来就是"有人在说话"
		}
		if lv.aiSpeaking || lv.source == SourceAI {
			fx.interrupt = append(fx.interrupt, ld)
		}
		lv.source = SourcePeer
		lv.heardSource, lv.heardFrom = sourceOf(speaker), owner
		fx.queued = append(fx.queued, queued{lv.out, &protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "start", Source: lv.heardSource, From: string(owner), Name: r.names(owner)}})
	}
	r.syncMixLocked(ch)
	fx.events = append(fx.events, Event{Kind: BurstStarted, Burst: *b})
	return b
}

func (r *Router) endBurstLocked(k burstKey, fx *effects) {
	b := r.bursts[k]
	if b == nil {
		return
	}
	delete(r.bursts, k)
	for _, l := range b.Listeners {
		r.stopListenerLocked(id.OwnerDevice(l), fx)
	}
	if mx := r.syncMixLocked(k.ch); mx != nil {
		fx.closeMix = append(fx.closeMix, mx)
	}
	fx.events = append(fx.events, Event{Kind: BurstEnded, Burst: *b})
	if !b.FirstInAt.IsZero() && !b.FirstOutAt.IsZero() {
		slog.Debug("burst ended", "channel", k.ch, "speaker", b.Speaker, "listeners", len(b.Listeners),
			"internal_latency_ms", b.FirstOutAt.Sub(b.FirstInAt).Milliseconds())
	}
}

// syncMixLocked 让混音状态跟上频道里的发言人数：≥2 人建，<2 人撤。撤掉的交给调用方在锁外关闭。
func (r *Router) syncMixLocked(ch id.ChannelID) (closed *chanMix) {
	n := 0
	for k := range r.bursts {
		if k.ch == ch {
			n++
		}
	}
	switch mx := r.mixers[ch]; {
	case n >= 2 && mx == nil:
		r.mixers[ch] = newChanMix()
	case n < 2 && mx != nil:
		delete(r.mixers, ch)
		return mx
	}
	return nil
}

func (r *Router) dropListenerLocked(b *Burst, d id.DeviceID) {
	p := id.Device(d)
	for i, l := range b.Listeners {
		if l == p {
			b.Listeners = append(b.Listeners[:i:i], b.Listeners[i+1:]...)
			var fx effects
			r.stopListenerLocked(d, &fx)
			// 只入队，不做 I/O，可在锁内完成。
			for _, q := range fx.queued {
				q.out.pushMsg(q.msg)
			}
			return
		}
	}
}

// stopListenerLocked 少一路发言在给 d 送声音；全部结束时才告诉设备"说完了"。
func (r *Router) stopListenerLocked(d id.DeviceID, fx *effects) {
	v := r.devices[d]
	if v == nil || v.hearing == 0 {
		return
	}
	if v.hearing--; v.hearing > 0 {
		return
	}
	if v.source == SourcePeer {
		v.source = SourceNone
	}
	fx.queued = append(fx.queued, queued{v.out, &protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "stop", Source: v.heardSource, From: string(v.heardFrom)}})
}

func sourceOf(p id.ParticipantID) string {
	if p.Kind() == id.KindAgent {
		return "agent"
	}
	return "peer"
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
	for k, b := range r.bursts {
		tailOver := !b.stopAt.IsZero() && now.After(b.stopAt)
		timeout := frameTimeout
		if b.Speaker.Kind() == id.KindAgent {
			timeout = agentFrameTimeout
		}
		silent := now.Sub(b.LastFrameAt) > timeout
		if tailOver || silent {
			r.endBurstLocked(k, &fx)
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
	closeMix  []*chanMix
	woke      []id.DeviceID
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
	for _, d := range fx.woke {
		// 唤醒被接受：提示音加聆听灯，让孩子知道小伙伴在听。
		p := id.Device(d)
		_ = r.sink.Send(p, &protocol.Sound{Type: protocol.TypeSound, Name: "wake"})
		_ = r.sink.Send(p, &protocol.Light{Type: protocol.TypeLight, State: "listening"})
	}
	for _, m := range fx.closeMix {
		m.close()
	}
	for _, e := range fx.events {
		select {
		case r.events <- e:
		default:
			slog.Error("voice event dropped: subscriber too slow", "kind", e.Kind)
		}
	}
}

// mixTick 每 60ms 给有多人同时说话的频道各混一帧。听者名单在锁内取快照，解码与编码在锁外做。
func (r *Router) mixTick() {
	type job struct {
		mx      *chanMix
		sources []id.ParticipantID
		targets []mixTarget
	}
	var jobs []job
	r.mu.Lock()
	for ch, mx := range r.mixers {
		j := job{mx: mx}
		idx := map[id.DeviceID]int{}
		for k, b := range r.bursts {
			if k.ch != ch {
				continue
			}
			j.sources = append(j.sources, b.Speaker)
			for _, l := range b.Listeners {
				ld := id.OwnerDevice(l)
				i, ok := idx[ld]
				if !ok {
					i = len(j.targets)
					idx[ld] = i
					j.targets = append(j.targets, mixTarget{listener: ld, out: r.dev(ld).out})
				}
				j.targets[i].from = append(j.targets[i].from, b.Speaker)
			}
		}
		jobs = append(jobs, j)
	}
	r.mu.Unlock()
	for _, j := range jobs {
		j.mx.tick(j.sources, j.targets)
	}
}
