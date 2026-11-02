package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const helloJSON = `{"type":"hello","device_id":"02:00:00:00:00:01","skin":"dragon","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":["mic","speaker"]}`

func dial(t *testing.T, gw *Gateway) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	return ws
}

func newGateway(h Handler) *Gateway {
	return New(h, func(*http.Request) bool { return true })
}

func TestHandshakeReturnsAck(t *testing.T) {
	gw := newGateway(nil)
	ws := dial(t, gw)

	if err := ws.WriteMessage(websocket.TextMessage, []byte(helloJSON)); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"type":"hello.ack"`) || !strings.Contains(string(data), `"session_id"`) {
		t.Fatalf("unexpected ack: %s", data)
	}
	if _, ok := gw.Session("02:00:00:00:00:01"); !ok {
		t.Fatal("session not registered after hello")
	}
}

func TestFirstMessageMustBeHello(t *testing.T) {
	ws := dial(t, newGateway(nil))
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"wake","source":"touch"}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"type":"error"`) {
		t.Fatalf("expected error, got %s", data)
	}
}

type recorder struct {
	msgs   chan protocol.Message
	audio  chan []byte
	closed chan struct{}
}

func newRecorder() *recorder {
	return &recorder{msgs: make(chan protocol.Message, 4), audio: make(chan []byte, 4), closed: make(chan struct{})}
}
func (r *recorder) OnConnect(*Session)                       {}
func (r *recorder) OnMessage(_ *Session, m protocol.Message) { r.msgs <- m }
func (r *recorder) OnAudio(_ *Session, f []byte)             { r.audio <- f }
func (r *recorder) OnDisconnect(*Session)                    { close(r.closed) }

func TestRoutesMessagesAudioAndDisconnect(t *testing.T) {
	rec := newRecorder()
	gw := newGateway(rec)
	ws := dial(t, gw)

	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	if _, _, err := ws.ReadMessage(); err != nil {
		t.Fatal(err)
	}

	_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"wake","source":"touch"}`))
	_ = ws.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3})

	select {
	case m := <-rec.msgs:
		if m.MsgType() != protocol.TypeWake {
			t.Fatalf("got %s", m.MsgType())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message routed")
	}
	select {
	case f := <-rec.audio:
		if len(f) != 3 {
			t.Fatalf("audio len %d", len(f))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no audio routed")
	}

	ws.Close()
	select {
	case <-rec.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("OnDisconnect not called")
	}
	waitUntil(t, func() bool { _, ok := gw.Session("02:00:00:00:00:01"); return !ok })
}

func TestDuplicateDeviceRejected(t *testing.T) {
	gw := newGateway(nil)
	ws1 := dial(t, gw)
	_ = ws1.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	if _, _, err := ws1.ReadMessage(); err != nil {
		t.Fatal(err)
	}

	ws2 := dial(t, gw)
	_ = ws2.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	_, data, err := ws2.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"unauthorized"`) {
		t.Fatalf("expected unauthorized, got %s", data)
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met")
}

func TestInvalidTextMessageGetsProtocolError(t *testing.T) {
	gw := newGateway(nil)
	ws := dial(t, gw)
	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	if _, _, err := ws.ReadMessage(); err != nil { // hello.ack
		t.Fatal(err)
	}

	_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"nope"}`))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"code":"bad_message"`) {
		t.Fatalf("expected bad_message, got %s", data)
	}
}

func TestSecondHelloRejected(t *testing.T) {
	gw := newGateway(nil)
	ws := dial(t, gw)
	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	if _, _, err := ws.ReadMessage(); err != nil { // hello.ack
		t.Fatal(err)
	}

	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "handshake already completed") {
		t.Fatalf("expected duplicate hello error, got %s", data)
	}
}

func TestServeHTTPRejectsNonWebSocketRequest(t *testing.T) {
	srv := httptest.NewServer(newGateway(nil))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSessionAccessorsAndSendAudio(t *testing.T) {
	gw := newGateway(nil)
	ws := dial(t, gw)
	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	if _, _, err := ws.ReadMessage(); err != nil { // hello.ack
		t.Fatal(err)
	}

	s, ok := gw.Session("02:00:00:00:00:01")
	if !ok {
		t.Fatal("session not found")
	}
	if s.ID() == "" || s.DeviceID() != "02:00:00:00:00:01" || s.Skin() != "dragon" || s.Firmware() != "" || len(s.Capabilities()) != 2 || s.Context() == nil {
		t.Fatalf("unexpected session: id=%q device=%q caps=%v ctx=%v", s.ID(), s.DeviceID(), s.Capabilities(), s.Context())
	}

	if err := s.SendAudio([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	mt, data, err := ws.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || len(data) != 3 {
		t.Fatalf("expected audio frame, got mt=%d len=%d err=%v", mt, len(data), err)
	}
}

// connPair 返回一条已升级的客户端连接和对应的服务端连接，用于直接测试 readHello。
func connPair(t *testing.T) (client, server *websocket.Conn) {
	t.Helper()
	conns := make(chan *websocket.Conn, 1)
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conns <- ws
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client, <-conns
}

func TestReadHelloErrors(t *testing.T) {
	tests := []struct {
		name string
		send func(*websocket.Conn)
		want string
	}{
		{"closed before hello", func(ws *websocket.Conn) { ws.Close() }, "close"},
		{"binary first frame", func(ws *websocket.Conn) { _ = ws.WriteMessage(websocket.BinaryMessage, []byte{1}) }, "text frame"},
		{"invalid message", func(ws *websocket.Conn) {
			_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"nope"}`))
		}, "bad message"},
		{"not hello", func(ws *websocket.Conn) {
			_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"wake","source":"touch"}`))
		}, "not hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := connPair(t)
			defer server.Close()
			tt.send(client)
			if _, err := readHello(server, time.Now().Add(time.Second)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readHello error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// fakeAuth 认为第一个字节为 1 的签名有效。
type fakeAuth struct{ required bool }

func (f fakeAuth) Required() bool { return f.required }
func (f fakeAuth) Verify(_ context.Context, _ id.DeviceID, _ []byte, nonce string, sig []byte) error {
	if len(nonce) != 43 || sig[0] != 1 {
		return errors.New("bad proof")
	}
	return nil
}

func proofJSON(ok bool) string {
	sig := make([]byte, 64)
	if ok {
		sig[0] = 1
	}
	return `{"type":"hello.proof","signature":"` + base64.RawURLEncoding.EncodeToString(sig) + `"}`
}

func readType(t *testing.T, ws *websocket.Conn) string {
	t.Helper()
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func authedHandshake(t *testing.T, srvURL string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srvURL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	if m := readType(t, ws); !strings.Contains(m, `"hello.challenge"`) {
		t.Fatalf("want challenge, got %s", m)
	}
	_ = ws.WriteMessage(websocket.TextMessage, []byte(proofJSON(true)))
	if m := readType(t, ws); !strings.Contains(m, `"hello.ack"`) {
		t.Fatalf("want ack, got %s", m)
	}
	return ws
}

func TestAuthHandshake(t *testing.T) {
	gw := newGateway(nil)
	gw.SetAuth(fakeAuth{required: true})
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)
	authedHandshake(t, srv.URL)
	if _, ok := gw.Session("02:00:00:00:00:01"); !ok {
		t.Fatal("session not registered")
	}
}

func TestAuthRejectsBadProof(t *testing.T) {
	for name, second := range map[string]string{
		"bad signature": proofJSON(false),
		"not a proof":   `{"type":"wake","source":"touch"}`,
		"garbage":       `{`,
	} {
		t.Run(name, func(t *testing.T) {
			gw := newGateway(nil)
			gw.SetAuth(fakeAuth{required: true})
			ws := dial(t, gw)
			_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
			readType(t, ws) // challenge
			_ = ws.WriteMessage(websocket.TextMessage, []byte(second))
			if m := readType(t, ws); !strings.Contains(m, `"unauthorized"`) {
				t.Fatalf("want unauthorized, got %s", m)
			}
			if _, ok := gw.Session("02:00:00:00:00:01"); ok {
				t.Fatal("unauthenticated session registered")
			}
		})
	}
}

func TestAuthOffSkipsChallenge(t *testing.T) {
	gw := newGateway(nil)
	gw.SetAuth(fakeAuth{required: false})
	ws := dial(t, gw)
	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	if m := readType(t, ws); !strings.Contains(m, `"hello.ack"`) {
		t.Fatalf("want ack, got %s", m)
	}
}

// 认证通过的新连接顶掉旧连接：旧连接收到 replaced 并断开，旧会话清理完后新会话才登记。
func TestAuthenticatedReconnectReplacesOldSession(t *testing.T) {
	rec := newRecorder()
	gw := newGateway(rec)
	gw.SetAuth(fakeAuth{required: true})
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)

	old := authedHandshake(t, srv.URL)
	first, _ := gw.Session("02:00:00:00:00:01")
	authedHandshake(t, srv.URL)

	if m := readType(t, old); !strings.Contains(m, "replaced") {
		t.Fatalf("old connection should be told it was replaced, got %s", m)
	}
	select {
	case <-rec.closed:
	case <-time.After(time.Second):
		t.Fatal("old session not cleaned up")
	}
	now, ok := gw.Session("02:00:00:00:00:01")
	if !ok || now == first {
		t.Fatal("new session should replace the old one")
	}
	if _, _, err := old.ReadMessage(); err == nil {
		t.Fatal("old connection should be closed")
	}
}

func TestHandshakeMessagesRejectedAfterHandshake(t *testing.T) {
	rec := newRecorder()
	gw := newGateway(rec)
	conn := dial(t, gw)
	_ = conn.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	readType(t, conn)
	_ = conn.WriteMessage(websocket.TextMessage, []byte(proofJSON(true)))
	if m := readType(t, conn); !strings.Contains(m, "handshake already completed") {
		t.Fatalf("got %s", m)
	}
}

type admitFunc func(ctx context.Context, d id.DeviceID) error

func (f admitFunc) Admit(ctx context.Context, d id.DeviceID) error { return f(ctx, d) }

func TestAdmissionRefusesBlockedDevice(t *testing.T) {
	gw := newGateway(nil)
	gw.SetAdmission(admitFunc(func(_ context.Context, d id.DeviceID) error {
		if d == "02:00:00:00:00:01" {
			return errors.New("locked")
		}
		return nil
	}))
	ws := dial(t, gw)
	if err := ws.WriteMessage(websocket.TextMessage, []byte(helloJSON)); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"code":"unauthorized"`) || strings.Contains(string(data), "hello.ack") {
		t.Fatalf("blocked device got %s", data)
	}
	if _, ok := gw.Session("02:00:00:00:00:01"); ok {
		t.Fatal("blocked device registered a session")
	}

	// 其他设备不受影响。
	ws2 := dial(t, gw)
	other := strings.Replace(helloJSON, "02:00:00:00:00:01", "02:00:00:00:00:02", 1)
	if err := ws2.WriteMessage(websocket.TextMessage, []byte(other)); err != nil {
		t.Fatal(err)
	}
	if _, data, err = ws2.ReadMessage(); err != nil || !strings.Contains(string(data), "hello.ack") {
		t.Fatalf("other device: %v %s", err, data)
	}
}
