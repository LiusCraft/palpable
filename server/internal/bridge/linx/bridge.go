// Package linx 把设备的 AI 对话桥接到灵矽（xrobotd/linxd）。
//
// 每台设备在灵矽侧对应一条独立的 WebSocket 会话；凭证只存在服务端。
// 灵矽协议见 xrobotd 的 linxdv0/internal/protocol：hello 握手后发 listen 开始收音，
// 灵矽用 stt / llm / tts 文本消息和 Opus 二进制帧回应。
package linx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/gateway"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const (
	dialTimeout  = 8 * time.Second
	writeTimeout = 5 * time.Second
	readLimit    = 1 << 20
)

// Config 来自环境变量，见 ConfigFromEnv。
type Config struct {
	URL     string // 例如 wss://xrobo-io.qiniuapi.com/v1/ws/
	Token   string // Bearer 令牌
	AgentID string // 可选，经 X-Agent-ID 指定智能体；为空时用设备在平台上绑定的智能体
}

// Bridge 实现 gateway.Handler：为每台设备维护一条灵矽会话。
type Bridge struct {
	cfg    Config
	dialer *websocket.Dialer

	mu    sync.Mutex
	links map[string]*link // session id -> link
}

func New(cfg Config) *Bridge {
	return &Bridge{
		cfg:    cfg,
		dialer: &websocket.Dialer{HandshakeTimeout: dialTimeout},
		links:  make(map[string]*link),
	}
}

var _ gateway.Handler = (*Bridge)(nil)

func (b *Bridge) OnConnect(s *gateway.Session) {
	ctx, cancel := context.WithCancel(s.Context())
	l := &link{b: b, dev: s, ctx: ctx, cancel: cancel}

	b.mu.Lock()
	b.links[s.ID()] = l
	b.mu.Unlock()

	// 拨号放到后台，不阻塞设备读循环；就绪前的音频帧直接丢弃。
	go l.run()
}

func (b *Bridge) OnDisconnect(s *gateway.Session) {
	b.mu.Lock()
	l := b.links[s.ID()]
	delete(b.links, s.ID())
	b.mu.Unlock()
	if l != nil {
		l.cancel()
	}
}

func (b *Bridge) OnMessage(s *gateway.Session, msg protocol.Message) {
	if l := b.link(s); l != nil {
		l.onDeviceMessage(msg)
	}
}

func (b *Bridge) OnAudio(s *gateway.Session, frame []byte) {
	if l := b.link(s); l != nil {
		l.onMic(frame)
	}
}

func (b *Bridge) link(s *gateway.Session) *link {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.links[s.ID()]
}

// link 是一台设备与灵矽之间的会话。
type link struct {
	b      *Bridge
	dev    *gateway.Session
	ctx    context.Context
	cancel context.CancelFunc

	wmu sync.Mutex // 保护对灵矽的写
	ws  *websocket.Conn

	mu        sync.Mutex
	ready     bool
	listening bool // 设备麦克风已开启
	speaking  bool // 灵矽正在下发 TTS
	linxSess  string
}

func (l *link) run() {
	defer l.cancel()

	mac := PseudoMAC(l.dev.DeviceID())
	ws, welcome, err := l.dial(mac)
	if err != nil {
		slog.Warn("linx dial failed", "device_id", l.dev.DeviceID(), "linx_mac", mac, "error", err)
		_ = l.dev.Send(&protocol.Error{Type: protocol.TypeError, Code: protocol.CodeInternal, Message: "ai service unavailable"})
		return
	}
	defer ws.Close()

	l.mu.Lock()
	l.ws = ws
	l.ready = true
	l.linxSess = welcome.SessionID
	wantListen := l.listening
	l.mu.Unlock()
	slog.Info("linx connected", "device_id", l.dev.DeviceID(), "linx_mac", mac,
		"linx_session", welcome.SessionID, "downlink_rate", welcome.AudioParams.SampleRate)

	// 灵矽的下行采样率可能是 24kHz，必须在任何音频帧之前告知设备。
	_ = l.dev.Send(&protocol.AudioDownlink{
		Type:          protocol.TypeAudioDownlink,
		SampleRate:    welcome.AudioParams.SampleRate,
		FrameDuration: welcome.AudioParams.FrameDuration,
	})

	// 设备在握手完成前就开了麦克风时，补发 listen。
	if wantListen {
		l.sendListen("start")
	}

	// 设备断开时关闭灵矽连接以解除读阻塞。
	go func() {
		<-l.ctx.Done()
		_ = ws.Close()
	}()
	l.readLoop(ws)
}

func (l *link) dial(mac string) (*websocket.Conn, welcome, error) {
	hdr := http.Header{}
	hdr.Set("Device-Id", mac) // 灵矽要求 MAC 形式，且该 MAC 需已预注册到智能体
	hdr.Set("Client-Id", newUUID())
	hdr.Set("Protocol-Version", "1")
	if l.b.cfg.Token != "" {
		hdr.Set("Authorization", "Bearer "+l.b.cfg.Token)
	}
	if l.b.cfg.AgentID != "" {
		hdr.Set("X-Agent-ID", l.b.cfg.AgentID)
	}

	ctx, cancel := context.WithTimeout(l.ctx, dialTimeout)
	defer cancel()
	ws, _, err := l.b.dialer.DialContext(ctx, l.b.cfg.URL, hdr)
	if err != nil {
		return nil, welcome{}, fmt.Errorf("dial: %w", err)
	}
	ws.SetReadLimit(readLimit)

	hello := map[string]any{
		"type":      "hello",
		"version":   1,
		"transport": "websocket",
		"audio_params": map[string]any{
			"format": "opus", "sample_rate": 16000, "channels": 1, "frame_duration": 60,
		},
		"features": map[string]any{"mcp": false},
	}
	_ = ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := ws.WriteJSON(hello); err != nil {
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

// readLoop 把灵矽的下行翻译成设备协议。
func (l *link) readLoop(ws *websocket.Conn) {
	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			if err := l.dev.SendAudio(data); err != nil {
				return
			}
		case websocket.TextMessage:
			l.onLinxText(data)
		}
	}
}

func (l *link) writeLinx(v any) error {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	l.mu.Lock()
	ws := l.ws
	l.mu.Unlock()
	if ws == nil {
		return errors.New("linx not connected")
	}
	_ = ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	if raw, ok := v.([]byte); ok {
		return ws.WriteMessage(websocket.BinaryMessage, raw)
	}
	return ws.WriteJSON(v)
}

func (l *link) sendListen(state string) {
	// realtime 为全双工：说话可打断 AI，是免按键对话推荐的模式。
	msg := map[string]any{"type": "listen", "state": state, "mode": "realtime"}
	if err := l.writeLinx(msg); err != nil {
		slog.Debug("linx listen send failed", "state", state, "error", err)
	}
}

// onMic 转发设备麦克风的一帧 Opus 到灵矽；未就绪或麦克风关闭时丢弃。
func (l *link) onMic(frame []byte) {
	l.mu.Lock()
	ok := l.ready && l.listening
	l.mu.Unlock()
	if !ok {
		return
	}
	_ = l.writeLinx(frame)
}

func (l *link) onDeviceMessage(msg protocol.Message) {
	switch m := msg.(type) {
	case *protocol.MicState:
		l.setListening(m.State == "start")
	case *protocol.Wake:
		// 唤醒视为开始收音；已在说话时不重复。
		l.setListening(true)
	}
}

func (l *link) setListening(on bool) {
	l.mu.Lock()
	changed := l.listening != on
	l.listening = on
	ready := l.ready
	speaking := l.speaking
	l.mu.Unlock()

	if !changed {
		return
	}
	if on {
		if ready {
			l.sendListen("start")
		}
		if !speaking {
			l.light("listening")
		}
		return
	}
	if ready {
		l.sendListen("stop")
	}
	if !speaking {
		l.light("idle")
	}
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
		// 一句话识别完成，AI 开始思考。文本本身不转发给设备（设备没有屏幕）。
		if m.Text != "" {
			l.light("thinking")
		}
	case "tts":
		switch m.State {
		case "start":
			l.mu.Lock()
			l.speaking = true
			l.mu.Unlock()
			l.light("speaking")
			_ = l.dev.Send(&protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "start", Source: "ai"})
		case "stop":
			l.mu.Lock()
			l.speaking = false
			listening := l.listening
			l.mu.Unlock()
			_ = l.dev.Send(&protocol.SpeakerState{Type: protocol.TypeSpeakerState, State: "stop", Source: "ai"})
			if listening {
				l.light("listening")
			} else {
				l.light("idle")
			}
		}
	}
}

func (l *link) light(state string) {
	_ = l.dev.Send(&protocol.Light{Type: protocol.TypeLight, State: state})
}
