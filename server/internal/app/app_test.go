package app

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/bridge/linx"
	"github.com/LiusCraft/palpable/server/internal/id"
)

const mac id.DeviceID = "02:00:00:00:00:01"

const helloJSON = `{"type":"hello","device_id":"02:00:00:00:00:01","skin":"dragon","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":["mic","speaker"]}`

// fakeAI 记录 app 对 AI 的调用，验证分发表。
type fakeAI struct{ calls chan string }

func (f *fakeAI) Open(_ context.Context, dev linx.DeviceLink, p linx.Persona) error {
	f.calls <- fmt.Sprintf("open %s agent=%q", dev.DeviceID(), p.AgentID)
	return nil
}
func (f *fakeAI) Close(d id.DeviceID) { f.calls <- "close " + string(d) }
func (f *fakeAI) Forward(d id.DeviceID, frame []byte) {
	f.calls <- fmt.Sprintf("forward %s %d", d, len(frame))
}
func (f *fakeAI) Listen(d id.DeviceID, on bool) { f.calls <- fmt.Sprintf("listen %s %v", d, on) }

func (f *fakeAI) want(t *testing.T, call string) {
	t.Helper()
	select {
	case got := <-f.calls:
		if got != call {
			t.Fatalf("call = %q, want %q", got, call)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for %q", call)
	}
}

func TestDispatch(t *testing.T) {
	f := &fakeAI{calls: make(chan string, 16)}
	srv := httptest.NewServer(newApp(f).Handler())
	defer srv.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	send := func(mt int, data string) {
		if err := ws.WriteMessage(mt, []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	send(websocket.TextMessage, helloJSON)
	f.want(t, `open 02:00:00:00:00:01 agent=""`)

	send(websocket.TextMessage, `{"type":"mic.state","state":"start"}`)
	f.want(t, "listen 02:00:00:00:00:01 true")
	send(websocket.BinaryMessage, "abc")
	f.want(t, "forward 02:00:00:00:00:01 3")
	send(websocket.TextMessage, `{"type":"mic.state","state":"stop"}`)
	f.want(t, "listen 02:00:00:00:00:01 false")
	send(websocket.TextMessage, `{"type":"wake","source":"touch"}`)
	f.want(t, "listen 02:00:00:00:00:01 true")

	// 尚无对应模块的消息不触发 AI。
	send(websocket.TextMessage, `{"type":"location","lat":31.2,"lng":121.5}`)

	_ = ws.Close()
	f.want(t, "close "+string(mac))
}

func TestWithoutLinxDevicesStillConnect(t *testing.T) {
	srv := httptest.NewServer(newApp(nil).Handler())
	defer srv.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	_ = ws.WriteMessage(websocket.TextMessage, []byte(helloJSON))
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil || !strings.Contains(string(data), "hello.ack") {
		t.Fatalf("ack: %s %v", data, err)
	}
	_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"mic.state","state":"start"}`))
	_ = ws.WriteMessage(websocket.BinaryMessage, []byte{1})
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newApp(nil).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("healthz = %d %q", rec.Code, rec.Body.String())
	}
}
