// Package linx 把设备的 AI 对话桥接到灵矽（xrobotd/linxd）。
//
// 每台已开通的设备在灵矽侧对应一条独立的 WebSocket 会话；凭证只存在服务端。
// 灵矽协议见 xrobotd 的 linxdv0/internal/protocol：hello 握手后发 listen 开始收音，
// 灵矽用 stt / llm / tts 文本消息和 Opus 二进制帧回应。
// 音频要不要给 AI 由 voice 决定，这里只管会话本身。
package linx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const (
	dialTimeout  = 8 * time.Second
	writeTimeout = 5 * time.Second
	readLimit    = 1 << 20
	outQueue     = 32 // 每条会话的上行队列；满了丢音频帧，不阻塞调用方

	// 连上后稳定这么久，下次断线从最短退避重新算。
	stableAfter = 30 * time.Second
)

var (
	ErrUnavailable = errors.New("linx: session unavailable")
	ErrAlreadyOpen = errors.New("linx: session already open")
)

// DeviceLink 是设备侧连接，由 gateway.Session 满足。
type DeviceLink interface {
	DeviceID() id.DeviceID
	Send(msg protocol.Message) error
	SendAudio(frame []byte) error
	Context() context.Context
}

// Config 来自 YAML 配置；智能体 ID 取自设备档案（见 Persona）。
type Config struct {
	URL   string // 例如 wss://xrobo-io.qiniuapi.com/v1/ws/
	Token string // Bearer 令牌
}

// Persona 是开一条会话所需的档案片段，由 app 从设备档案映射，bridge 因此不依赖 device。
type Persona struct {
	AgentID   string // 经 X-Agent-ID 指定；为空时用灵矽平台上该 MAC 已绑定的智能体
	DollName  string
	KidName   string
	VoiceStep string // 仅引导智能体：声音登记这一步的指令（由 device 给出），注入 {{voice_step}}
}

type AIState int

const (
	AIIdle AIState = iota
	AISpeaking
)

// Bridge 为每台设备维护一条灵矽会话，断线后指数退避重连，设备断开即停止。
type Bridge struct {
	cfg    Config
	dialer *websocket.Dialer
	links  sync.Map // id.DeviceID -> *link；Forward 是热路径，读不加锁

	onAIState atomic.Pointer[func(id.DeviceID, AIState)]
	onMCP     atomic.Pointer[func(id.DeviceID, json.RawMessage)]

	backoffMin, backoffMax time.Duration
}

func New(cfg Config) *Bridge {
	return &Bridge{
		cfg:        cfg,
		dialer:     &websocket.Dialer{HandshakeTimeout: dialTimeout},
		backoffMin: time.Second,
		backoffMax: 30 * time.Second,
	}
}

// OnAIState 注册 AI 说话状态的回调；回调在会话读循环里调用，不得阻塞。
func (b *Bridge) OnAIState(fn func(d id.DeviceID, s AIState)) { b.onAIState.Store(&fn) }

// OnMCP 注册灵矽发来的 mcp 消息的回调（交给 bridge/mcp 代理）；在会话读循环里调用，不得阻塞。
func (b *Bridge) OnMCP(fn func(d id.DeviceID, payload json.RawMessage)) { b.onMCP.Store(&fn) }

// SendMCP 把代理的 JSON-RPC 回复发给灵矽；会话未连上时返回 ErrUnavailable。
func (b *Bridge) SendMCP(d id.DeviceID, payload json.RawMessage) error {
	l := b.link(d)
	if l == nil {
		return fmt.Errorf("send mcp to %s: %w", d, ErrUnavailable)
	}
	c := l.cur.Load()
	if c == nil {
		return fmt.Errorf("send mcp to %s: %w", d, ErrUnavailable)
	}
	ctx, cancel := context.WithTimeout(l.ctx, writeTimeout)
	defer cancel()
	msg := map[string]any{"type": "mcp", "session_id": c.session, "payload": payload}
	if err := c.sendJSON(ctx, msg); err != nil {
		return fmt.Errorf("send mcp to %s: %w", d, err)
	}
	return nil
}

// Open 为设备开一条会话并在后台连接；会话随 dev.Context() 结束。
// 交棒换智能体时先 Close 再用新的 Persona Open。
func (b *Bridge) Open(_ context.Context, dev DeviceLink, p Persona) error {
	ctx, cancel := context.WithCancel(dev.Context())
	l := &link{b: b, dev: dev, persona: p, ctx: ctx, cancel: cancel, done: make(chan struct{}), ready: make(chan struct{})}
	if _, loaded := b.links.LoadOrStore(dev.DeviceID(), l); loaded {
		cancel()
		return fmt.Errorf("open %s: %w", dev.DeviceID(), ErrAlreadyOpen)
	}
	go l.run()
	return nil
}

// Close 结束设备的会话，返回时所有相关 goroutine 都已退出。
func (b *Bridge) Close(d id.DeviceID) {
	v, ok := b.links.LoadAndDelete(d)
	if !ok {
		return
	}
	l := v.(*link)
	l.cancel()
	<-l.done
}

func (b *Bridge) Available(d id.DeviceID) bool {
	l := b.link(d)
	return l != nil && l.cur.Load() != nil
}

// Forward 把一帧麦克风 Opus 交给 AI；未连接、未在收音或队列满时丢弃。
func (b *Bridge) Forward(d id.DeviceID, frame []byte) {
	l := b.link(d)
	if l == nil || !l.listening.Load() {
		return
	}
	if c := l.cur.Load(); c != nil {
		c.trySend(outFrame{binary: true, data: frame})
	}
}

// Listen 开关灵矽收音，并相应切换灯光。
func (b *Bridge) Listen(d id.DeviceID, on bool) {
	if l := b.link(d); l != nil {
		l.setListening(on)
	}
}

// Interrupt 中止 AI 播报：通知灵矽，并丢弃本轮剩余的 TTS 音频。
func (b *Bridge) Interrupt(d id.DeviceID) {
	if l := b.link(d); l != nil {
		l.interrupt()
	}
}

// Say 让小伙伴直接说出 text，不经 LLM。会话尚未连上时等到连上或 ctx 结束。
func (b *Bridge) Say(ctx context.Context, d id.DeviceID, text string) error {
	l := b.link(d)
	if l == nil {
		return fmt.Errorf("say to %s: %w", d, ErrUnavailable)
	}
	c, err := l.waitReady(ctx)
	if err != nil {
		return fmt.Errorf("say to %s: %w", d, err)
	}
	// 灵矽文档里 listen detect 的 text_response 是"需要播放的文字"。
	msg := map[string]any{"type": "listen", "session_id": c.session, "state": "detect", "mode": "realtime", "text_response": text}
	if err := c.sendJSON(ctx, msg); err != nil {
		return fmt.Errorf("say to %s: %w", d, err)
	}
	return nil
}

func (b *Bridge) link(d id.DeviceID) *link {
	v, ok := b.links.Load(d)
	if !ok {
		return nil
	}
	return v.(*link)
}

func (b *Bridge) emitAIState(d id.DeviceID, s AIState) {
	if fn := b.onAIState.Load(); fn != nil {
		(*fn)(d, s)
	}
}

// link 是一台设备在 bridge 里的全部状态，跨越多次重连。
type link struct {
	b       *Bridge
	dev     DeviceLink
	persona Persona
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}

	cur       atomic.Pointer[conn] // 当前连接，未连上为 nil
	listening atomic.Bool
	speaking  atomic.Bool
	muted     atomic.Bool // Interrupt 之后到下一次 tts start 之间丢弃 TTS 音频
	// platformErr 表示本次连接收到平台错误，之后的 TTS 都不播；重连后清除。
	platformErr atomic.Bool

	mu    sync.Mutex
	ready chan struct{} // 连上时关闭；断线后换新的
}

// conn 是一次灵矽连接；所有写都经 out 交给唯一的写 goroutine。
type conn struct {
	ws      *websocket.Conn
	session string
	out     chan outFrame
	closed  chan struct{}
}

type outFrame struct {
	binary bool
	data   []byte
}

func (c *conn) trySend(f outFrame) bool {
	select {
	case c.out <- f:
		return true
	default:
		return false
	}
}

func (c *conn) sendJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	select {
	case c.out <- outFrame{data: data}:
		return nil
	case <-c.closed:
		return ErrUnavailable
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *link) run() {
	defer close(l.done)
	defer l.b.links.CompareAndDelete(l.dev.DeviceID(), l)
	defer l.cancel()

	delay := l.b.backoffMin
	reported := false
	for {
		started := time.Now()
		connected, err := l.session()
		if l.ctx.Err() != nil {
			return
		}
		if connected {
			reported = false
		} else if !reported {
			// 只报告一次，避免重试期间刷屏。
			_ = l.dev.Send(&protocol.Error{Type: protocol.TypeError, Code: protocol.CodeInternal, Message: "ai service unavailable"})
			reported = true
		}
		slog.Warn("linx session ended, will retry", "device_id", l.dev.DeviceID(), "connected", connected, "retry_in", delay, "error", err)
		if time.Since(started) >= stableAfter {
			delay = l.b.backoffMin
		}
		select {
		case <-l.ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, l.b.backoffMax)
	}
}

// session 连接一次并服务到断线。connected 表示握手是否成功。
func (l *link) session() (connected bool, err error) {
	ws, w, err := l.dial()
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithCancel(l.ctx)
	c := &conn{ws: ws, session: w.SessionID, out: make(chan outFrame, outQueue), closed: make(chan struct{})}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		l.writeLoop(ctx, c)
	}()
	defer func() {
		cancel()
		_ = ws.Close()
		wg.Wait()
		l.disconnected()
	}()

	l.connected(c)
	slog.Info("linx connected", "device_id", l.dev.DeviceID(), "agent_id", l.persona.AgentID,
		"linx_session", w.SessionID, "downlink_rate", w.AudioParams.SampleRate)

	// 灵矽的下行采样率可能是 24kHz，必须在任何音频帧之前告知设备。
	_ = l.dev.Send(&protocol.AudioDownlink{
		Type:          protocol.TypeAudioDownlink,
		SampleRate:    w.AudioParams.SampleRate,
		FrameDuration: w.AudioParams.FrameDuration,
	})
	// 设备在连上之前就开了麦克风（或是重连）时，补发 listen。
	if l.listening.Load() {
		l.sendListen(c, "start")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		_ = ws.Close() // 解除读阻塞
	}()
	return true, l.readLoop(c)
}

func (l *link) dial() (*websocket.Conn, welcome, error) {
	hdr := http.Header{}
	hdr.Set("Device-Id", string(l.dev.DeviceID())) // 灵矽要求 MAC，且已预注册到智能体
	hdr.Set("Client-Id", newUUID())
	hdr.Set("Protocol-Version", "1")
	if l.b.cfg.Token != "" {
		hdr.Set("Authorization", "Bearer "+l.b.cfg.Token)
	}
	if l.persona.AgentID != "" {
		hdr.Set("X-Agent-ID", l.persona.AgentID)
	}

	ctx, cancel := context.WithTimeout(l.ctx, dialTimeout)
	defer cancel()
	ws, _, err := l.b.dialer.DialContext(ctx, l.b.cfg.URL, hdr)
	if err != nil {
		return nil, welcome{}, fmt.Errorf("dial: %w", err)
	}
	ws.SetReadLimit(readLimit)

	_ = ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := ws.WriteJSON(l.hello()); err != nil {
		ws.Close()
		return nil, welcome{}, fmt.Errorf("send hello: %w", err)
	}

	_ = ws.SetReadDeadline(time.Now().Add(dialTimeout))
	var w welcome
	if err := ws.ReadJSON(&w); err != nil {
		ws.Close()
		return nil, welcome{}, fmt.Errorf("read welcome: %w", err)
	}
	_ = ws.SetReadDeadline(time.Time{})
	if w.Type != "hello" {
		ws.Close()
		return nil, welcome{}, errors.New("unexpected first message from linx")
	}
	w.normalize()
	return ws, w, nil
}

func (l *link) hello() map[string]any {
	return map[string]any{
		"type":      "hello",
		"version":   1,
		"transport": "websocket",
		"audio_params": map[string]any{
			"format": "opus", "sample_rate": 16000, "channels": 1, "frame_duration": 60,
		},
		// 总是声明 mcp：设备自身不支持时，代理仍向灵矽提供业务工具。
		"features": map[string]any{"mcp": true},
		// 名字每次会话注入，起名后不必改智能体配置；空值也要发，否则提示词里会留下 {{占位符}}。
		"agent_params": map[string]any{
			"custom_replace_prompt": map[string]string{
				"doll_name":  l.persona.DollName,
				"kid_name":   l.persona.KidName,
				"voice_step": l.persona.VoiceStep,
			},
		},
	}
}

func (l *link) writeLoop(ctx context.Context, c *conn) {
	defer close(c.closed)
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-c.out:
			mt := websocket.TextMessage
			if f.binary {
				mt = websocket.BinaryMessage
			}
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.ws.WriteMessage(mt, f.data); err != nil {
				_ = c.ws.Close()
				return
			}
		}
	}
}

// readLoop 把灵矽的下行翻译成设备协议。
func (l *link) readLoop(c *conn) error {
	for {
		mt, data, err := c.ws.ReadMessage()
		if err != nil {
			return err
		}
		switch mt {
		case websocket.BinaryMessage:
			if l.muted.Load() {
				continue
			}
			if err := l.dev.SendAudio(data); err != nil {
				return fmt.Errorf("send audio to device: %w", err)
			}
		case websocket.TextMessage:
			l.onLinxText(data)
		}
	}
}

func (l *link) connected(c *conn) {
	l.cur.Store(c)
	l.mu.Lock()
	close(l.ready)
	l.mu.Unlock()
}

func (l *link) disconnected() {
	l.cur.Store(nil)
	l.mu.Lock()
	l.ready = make(chan struct{})
	l.mu.Unlock()
	l.muted.Store(false)
	l.platformErr.Store(false)
	if l.speaking.Swap(false) && l.dev.Context().Err() == nil {
		l.stopSpeaking()
	}
}

func (l *link) waitReady(ctx context.Context) (*conn, error) {
	for {
		if c := l.cur.Load(); c != nil {
			return c, nil
		}
		l.mu.Lock()
		ready := l.ready
		l.mu.Unlock()
		if c := l.cur.Load(); c != nil {
			return c, nil
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-l.ctx.Done():
			return nil, ErrUnavailable
		}
	}
}

func (l *link) sendListen(c *conn, state string) {
	// realtime 为全双工：说话可打断 AI，是免按键对话推荐的模式。
	msg := map[string]any{"type": "listen", "session_id": c.session, "state": state, "mode": "realtime"}
	ctx, cancel := context.WithTimeout(l.ctx, writeTimeout)
	defer cancel()
	if err := c.sendJSON(ctx, msg); err != nil {
		slog.Debug("linx listen send failed", "state", state, "error", err)
	}
}

func (l *link) setListening(on bool) {
	if l.listening.Swap(on) == on {
		return
	}
	if c := l.cur.Load(); c != nil {
		l.sendListen(c, map[bool]string{true: "start", false: "stop"}[on])
	}
	if !l.speaking.Load() {
		l.light(map[bool]string{true: "listening", false: "idle"}[on])
	}
}

func (l *link) interrupt() {
	if !l.speaking.Swap(false) {
		return
	}
	l.muted.Store(true)
	if c := l.cur.Load(); c != nil {
		ctx, cancel := context.WithTimeout(l.ctx, writeTimeout)
		_ = c.sendJSON(ctx, map[string]any{"type": "abort", "session_id": c.session, "reason": "interrupted"})
		cancel()
	}
	l.stopSpeaking()
}

// stopSpeaking 告诉设备 AI 说完了；调用方已把 speaking 置为 false。
func (l *link) stopSpeaking() {
	_ = l.dev.Send(&protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "stop", Source: "ai"})
	if l.listening.Load() {
		l.light("listening")
	} else {
		l.light("idle")
	}
	l.b.emitAIState(l.dev.DeviceID(), AIIdle)
}

// onLinxText 处理灵矽的 stt / tts / llm 文本消息。
func (l *link) onLinxText(data []byte) {
	m, err := parseLinx(data)
	if err != nil {
		slog.Debug("linx bad message", "error", err)
		return
	}
	switch m.Type {
	case "stt":
		if m.ErrorCode != 0 {
			// 平台错误（如设备未绑定智能体）会以 stt 文本加一段 TTS 播报：那是给大人看的提示，
			// 不播给孩子，改为向设备报错，并丢弃本会话后续的 TTS。
			l.platformErr.Store(true)
			l.muted.Store(true)
			slog.Warn("linx session error", "device_id", l.dev.DeviceID(), "error_code", m.ErrorCode)
			_ = l.dev.Send(&protocol.Error{Type: protocol.TypeError, Code: protocol.CodeInternal, Message: "ai service unavailable"})
			return
		}
		// 一句话识别完成，AI 开始思考。文本本身不转发给设备（设备没有屏幕）。
		if m.Text != "" {
			l.light("thinking")
		}
	case "mcp":
		if fn := l.b.onMCP.Load(); fn != nil && len(m.Payload) > 0 {
			(*fn)(l.dev.DeviceID(), m.Payload)
		}
	case "tts":
		switch m.State {
		case "start":
			if l.platformErr.Load() {
				return
			}
			l.muted.Store(false)
			if l.speaking.Swap(true) {
				return
			}
			l.light("speaking")
			_ = l.dev.Send(&protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "start", Source: "ai"})
			l.b.emitAIState(l.dev.DeviceID(), AISpeaking)
		case "stop":
			if l.speaking.Swap(false) {
				l.stopSpeaking()
			}
		}
	}
}

func (l *link) light(state string) {
	_ = l.dev.Send(&protocol.Light{Type: protocol.TypeLight, State: state})
}
