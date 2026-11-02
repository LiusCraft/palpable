// Package gateway 接受设备 WebSocket 连接，完成握手并路由消息。
// 它不含任何业务规则：好友、频道、屏蔽由其他模块决定。
package gateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	// 顶号时等旧连接清理完的上限；超时则拒绝新连接，避免两条会话同时操作同一设备。
	takeoverWait = 5 * time.Second
)

// Handler 由 app 实现，接收设备事件。OnAudio 是热路径。
type Handler interface {
	OnConnect(s *Session)
	OnMessage(s *Session, msg protocol.Message)
	OnAudio(s *Session, frame []byte)
	OnDisconnect(s *Session)
}

// Authenticator 校验设备持有私钥；由 app 注入 device.Authenticator。
type Authenticator interface {
	Required() bool
	Verify(ctx context.Context, d id.DeviceID, pubkey []byte, nonce string, sig []byte) error
}

// Gateway 管理所有设备会话。
type Gateway struct {
	handler  Handler
	upgrader websocket.Upgrader
	auth     Authenticator // nil 表示不认证

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

// SetAuth 须在开始接受连接前调用。
func (g *Gateway) SetAuth(a Authenticator) { g.auth = a }

func (g *Gateway) authRequired() bool { return g.auth != nil && g.auth.Required() }

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

	s := &Session{ws: ws, id: newSessionID(), ctx: ctx, done: make(chan struct{})}
	// 最后执行：OnDisconnect 与 unregister 都完成后，顶号的新连接才继续。
	defer close(s.done)

	deadline := time.Now().Add(helloTimeout)
	hello, err := readHello(ws, deadline)
	if err != nil {
		_ = s.Send(errorMsg(protocol.CodeBadMessage, "first message must be a valid hello"))
		slog.Warn("gateway handshake failed", "error", err)
		return
	}
	s.deviceID = hello.DeviceID
	s.skin = hello.Skin
	s.firmware = hello.Firmware
	s.capabilities = hello.Capabilities

	if g.authRequired() {
		if err := g.authenticate(ctx, s, hello, deadline); err != nil {
			_ = s.Send(errorMsg(protocol.CodeUnauthorized, "authentication failed"))
			// 公钥不是秘密；记下来便于运维把未登记的设备（如 Web 模拟器）登记进出厂清单。
			slog.Warn("device authentication failed", "device_id", s.deviceID, "error", err, "pubkey", hello.PubKey)
			return
		}
	}

	if !g.register(s, g.authRequired()) {
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
			switch msg.(type) {
			case *protocol.Hello, *protocol.HelloProof:
				_ = s.Send(errorMsg(protocol.CodeBadMessage, "handshake already completed"))
				continue
			}
			g.handler.OnMessage(s, msg)
		}
	}
}

// authenticate 发挑战并校验签名；随机数每条连接一个，不依赖设备时钟。
func (g *Gateway) authenticate(ctx context.Context, s *Session, hello *protocol.Hello, deadline time.Time) error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	nonce := base64.RawURLEncoding.EncodeToString(b)
	if err := s.Send(&protocol.HelloChallenge{Type: protocol.TypeHelloChallenge, Nonce: nonce}); err != nil {
		return err
	}
	msg, err := readText(s.ws, deadline)
	if err != nil {
		return err
	}
	proof, ok := msg.(*protocol.HelloProof)
	if !ok {
		return errors.New("expected hello.proof")
	}
	return g.auth.Verify(ctx, s.deviceID, hello.PubKeyBytes(), nonce, proof.SignatureBytes())
}

func readHello(ws *websocket.Conn, deadline time.Time) (*protocol.Hello, error) {
	msg, err := readText(ws, deadline)
	if err != nil {
		return nil, err
	}
	hello, ok := msg.(*protocol.Hello)
	if !ok {
		return nil, errors.New("first message is not hello")
	}
	return hello, nil
}

// readText 读一条握手阶段的文本消息。
func readText(ws *websocket.Conn, deadline time.Time) (protocol.Message, error) {
	_ = ws.SetReadDeadline(deadline)
	defer ws.SetReadDeadline(time.Time{})

	mt, data, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}
	if mt != websocket.TextMessage {
		return nil, errors.New("handshake message must be a text frame")
	}
	return protocol.DecodeUpstream(data)
}

// register 登记会话。takeover 时（仅在已认证的前提下）顶掉旧连接：等它的 OnDisconnect 与注销
// 都做完再登记，避免旧会话的清理误伤新会话。
func (g *Gateway) register(s *Session, takeover bool) bool {
	for range 2 {
		g.mu.Lock()
		old, exists := g.sessions[s.deviceID]
		if !exists {
			g.sessions[s.deviceID] = s
			g.mu.Unlock()
			return true
		}
		g.mu.Unlock()
		if !takeover {
			return false
		}
		slog.Info("device reconnected, replacing old session", "device_id", s.deviceID, "old_session", old.id)
		old.kick()
		select {
		case <-old.done:
		case <-time.After(takeoverWait):
			return false
		case <-s.ctx.Done():
			return false
		}
	}
	return false
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
	done         chan struct{} // 会话彻底清理完时关闭

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

// Close 断开设备；读循环随之退出并走正常的清理。设备会重连，用于解绑后让设备回到出厂流程。
func (s *Session) Close() { _ = s.ws.Close() }

// kick 通知设备被新连接顶替并断开；读循环随之退出并走正常的清理。
func (s *Session) kick() {
	_ = s.Send(errorMsg(protocol.CodeUnauthorized, "replaced by a new connection"))
	_ = s.ws.Close()
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
