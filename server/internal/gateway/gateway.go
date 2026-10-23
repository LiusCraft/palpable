// Package gateway 接受设备 WebSocket 连接，完成握手并路由消息。
// 它不含任何业务规则：好友、频道、屏蔽由其他模块决定。
package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const (
	helloTimeout = 10 * time.Second
	writeTimeout = 5 * time.Second
	maxTextBytes = 4 << 10
	maxFrameSize = 4 << 10
)

// Handler 由 app 实现，接收设备事件。OnAudio 是热路径。
type Handler interface {
	OnConnect(s *Session)
	OnMessage(s *Session, msg protocol.Message)
	OnAudio(s *Session, frame []byte)
	OnDisconnect(s *Session)
}

// Gateway 管理所有设备会话。
type Gateway struct {
	handler  Handler
	upgrader websocket.Upgrader

	mu       sync.RWMutex
	sessions map[id.DeviceID]*Session
}

// New 创建网关。handler 为 nil 时只做握手；checkOrigin 为 nil 时只允许同源。
func New(handler Handler, checkOrigin func(r *http.Request) bool) *Gateway {
	if handler == nil {
		handler = noopHandler{}
	}
	return &Gateway{
		handler:  handler,
		upgrader: websocket.Upgrader{CheckOrigin: checkOrigin, ReadBufferSize: 4096, WriteBufferSize: 4096},
		sessions: make(map[id.DeviceID]*Session),
	}
}

// Session 返回在线设备的会话。
func (g *Gateway) Session(deviceID id.DeviceID) (*Session, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	s, ok := g.sessions[deviceID]
	return s, ok
}

// ServeHTTP 升级为 WebSocket 并运行设备会话，直到连接关闭。
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.WarnContext(r.Context(), "gateway upgrade failed", "error", err)
		return
	}
	ws.SetReadLimit(maxFrameSize)
	g.run(r.Context(), ws)
}

func (g *Gateway) run(ctx context.Context, ws *websocket.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer ws.Close()

	s := &Session{ws: ws, id: newSessionID(), ctx: ctx}

	hello, err := readHello(ws)
	if err != nil {
		_ = s.Send(errorMsg(protocol.CodeBadMessage, "first message must be a valid hello"))
		slog.Warn("gateway handshake failed", "error", err)
		return
	}
	s.deviceID = hello.DeviceID
	s.skin = hello.Skin
	s.firmware = hello.Firmware
	s.capabilities = hello.Capabilities

	if !g.register(s) {
		_ = s.Send(errorMsg(protocol.CodeUnauthorized, "device already connected"))
		return
	}
	defer g.unregister(s)

	ack := &protocol.HelloAck{Type: protocol.TypeHelloAck, SessionID: s.id, AudioParams: protocol.DefaultAudioParams()}
	if err := s.Send(ack); err != nil {
		return
	}
	slog.Info("device connected", "device_id", s.deviceID, "session_id", s.id)

	g.handler.OnConnect(s)
	defer g.handler.OnDisconnect(s)

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			g.handler.OnAudio(s, data)
		case websocket.TextMessage:
			if len(data) > maxTextBytes {
				_ = s.Send(errorMsg(protocol.CodeBadMessage, "message too large"))
				continue
			}
			msg, err := protocol.DecodeUpstream(data)
			if err != nil {
				_ = s.Send(errorMsg(protocol.CodeBadMessage, err.Error()))
				continue
			}
			if _, isHello := msg.(*protocol.Hello); isHello {
				_ = s.Send(errorMsg(protocol.CodeBadMessage, "hello already received"))
				continue
			}
			g.handler.OnMessage(s, msg)
		}
	}
}

func readHello(ws *websocket.Conn) (*protocol.Hello, error) {
	_ = ws.SetReadDeadline(time.Now().Add(helloTimeout))
	defer ws.SetReadDeadline(time.Time{})

	mt, data, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}
	if mt != websocket.TextMessage {
		return nil, errors.New("hello must be a text frame")
	}
	msg, err := protocol.DecodeUpstream(data)
	if err != nil {
		return nil, err
	}
	hello, ok := msg.(*protocol.Hello)
	if !ok {
		return nil, errors.New("first message is not hello")
	}
	return hello, nil
}

func (g *Gateway) register(s *Session) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.sessions[s.deviceID]; exists {
		return false
	}
	g.sessions[s.deviceID] = s
	return true
}

func (g *Gateway) unregister(s *Session) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sessions[s.deviceID] == s {
		delete(g.sessions, s.deviceID)
	}
}

// Session 代表一台在线设备。写操作串行化，可从多个 goroutine 调用。
type Session struct {
	id           string
	deviceID     id.DeviceID
	skin         string
	firmware     string
	capabilities []string
	ctx          context.Context

	wmu sync.Mutex
	ws  *websocket.Conn
}

func (s *Session) ID() string               { return s.id }
func (s *Session) DeviceID() id.DeviceID    { return s.deviceID }
func (s *Session) Skin() string             { return s.skin }
func (s *Session) Firmware() string         { return s.firmware }
func (s *Session) Capabilities() []string   { return s.capabilities }
func (s *Session) Context() context.Context { return s.ctx }

// Send 发送一条下行文本消息。
func (s *Session) Send(msg protocol.Message) error {
	data, err := protocol.Encode(msg)
	if err != nil {
		return err
	}
	return s.write(websocket.TextMessage, data)
}

// SendAudio 发送一帧下行 Opus 音频。
func (s *Session) SendAudio(frame []byte) error {
	return s.write(websocket.BinaryMessage, frame)
}

func (s *Session) write(mt int, data []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_ = s.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	return s.ws.WriteMessage(mt, data)
}

func errorMsg(code, message string) *protocol.Error {
	return &protocol.Error{Type: protocol.TypeError, Code: code, Message: message}
}

func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type noopHandler struct{}

func (noopHandler) OnConnect(*Session)                   {}
func (noopHandler) OnMessage(*Session, protocol.Message) {}
func (noopHandler) OnAudio(*Session, []byte)             {}
func (noopHandler) OnDisconnect(*Session)                {}
