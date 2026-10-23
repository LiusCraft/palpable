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
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/store"
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

func mustApp(t *testing.T, f ai) *App {
	t.Helper()
	d := deps{store: store.NewMemory()}
	if f != nil {
		d.ai = f
	}
	a, err := newApp(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// connect 以 mac 接入并读掉 hello.ack。
func connect(t *testing.T, srv *httptest.Server, mac id.DeviceID) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	hello := strings.Replace(helloJSON, "02:00:00:00:00:01", string(mac), 1)
	if err := ws.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := ws.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	return ws
}

func readText(t *testing.T, ws *websocket.Conn) string {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if mt == websocket.TextMessage {
			return string(data)
		}
	}
}

func TestRelationEventsReachBothDevices(t *testing.T) {
	a := mustApp(t, nil)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	const other id.DeviceID = "02:00:00:00:00:02"
	wsA, wsB := connect(t, srv, mac), connect(t, srv, other)

	a.onRelation(context.Background(), relation.Event{Kind: relation.Proposed, Friendship: relation.Friendship{ID: "f1", A: mac, B: other}})
	for _, c := range []struct {
		ws   *websocket.Conn
		peer id.DeviceID
	}{{wsA, other}, {wsB, mac}} {
		if got := readText(t, c.ws); !strings.Contains(got, `"type":"friend.state"`) || !strings.Contains(got, `"event":"pending"`) || !strings.Contains(got, string(c.peer)) {
			t.Fatalf("friend.state = %s", got)
		}
		if got := readText(t, c.ws); !strings.Contains(got, `"type":"haptic"`) {
			t.Fatalf("haptic = %s", got)
		}
		if got := readText(t, c.ws); !strings.Contains(got, `"name":"friend"`) {
			t.Fatalf("sound = %s", got)
		}
	}
}

func TestDispatch(t *testing.T) {
	f := &fakeAI{calls: make(chan string, 16)}
	srv := httptest.NewServer(mustApp(t, f).Handler())
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
	srv := httptest.NewServer(mustApp(t, nil).Handler())
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
	mustApp(t, nil).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("healthz = %d %q", rec.Code, rec.Body.String())
	}
}
