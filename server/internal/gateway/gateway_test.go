package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const helloJSON = `{"type":"hello","device_id":"plush-001","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":["mic","speaker"]}`

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
	if _, ok := gw.Session("plush-001"); !ok {
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
	waitUntil(t, func() bool { _, ok := gw.Session("plush-001"); return !ok })
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
	if !strings.Contains(string(data), "hello already received") {
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

	s, ok := gw.Session("plush-001")
	if !ok {
		t.Fatal("session not found")
	}
	if s.ID() == "" || s.DeviceID() != "plush-001" || len(s.Capabilities()) != 2 || s.Context() == nil {
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
			if _, err := readHello(server); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readHello error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
