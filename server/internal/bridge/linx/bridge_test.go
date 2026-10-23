package linx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/gateway"
)

const helloJSON = `{"type":"hello","device_id":"plush-001","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":["mic","speaker"]}`

// fakeLinx 模拟灵矽：校验握手头，记录收到的帧，并允许测试主动下发消息。
type fakeLinx struct {
	t       *testing.T
	headers chan http.Header
	texts   chan string
	audio   chan []byte
	conn    chan *websocket.Conn
}

func newFakeLinx(t *testing.T) (*fakeLinx, string) {
	f := &fakeLinx{
		t:       t,
		headers: make(chan http.Header, 1),
		texts:   make(chan string, 16),
		audio:   make(chan []byte, 16),
		conn:    make(chan *websocket.Conn, 1),
	}
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		f.headers <- r.Header.Clone()
		// 灵矽先收 hello，再回 hello。
		if _, _, err := ws.ReadMessage(); err != nil {
			return
		}
		_ = ws.WriteJSON(map[string]any{"type": "hello", "session_id": "linx-s1"})
		f.conn <- ws
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if mt == websocket.BinaryMessage {
				f.audio <- data
			} else {
				f.texts <- string(data)
			}
		}
	}))
	t.Cleanup(srv.Close)
	return f, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dialDevice(t *testing.T, b *Bridge) *websocket.Conn {
	t.Helper()
	gw := gateway.New(b, func(*http.Request) bool { return true })
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := ws.WriteMessage(websocket.TextMessage, []byte(helloJSON)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.ReadMessage(); err != nil { // hello.ack
		t.Fatal(err)
	}
	return ws
}

func recvText(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for text")
		return ""
	}
}

func readDeviceText(t *testing.T, ws *websocket.Conn) string {
	t.Helper()
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

func TestDialSendsAuthAndDeviceID(t *testing.T) {
	f, url := newFakeLinx(t)
	dialDevice(t, New(Config{URL: url, Token: "secret-token"}))

	select {
	case h := <-f.headers:
		if h.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("Authorization = %q", h.Get("Authorization"))
		}
		if h.Get("Device-Id") != "plush-001" {
			t.Errorf("Device-Id = %q", h.Get("Device-Id"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("linx was never dialed")
	}
}

func TestMicStartForwardsListenAndAudio(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	<-f.conn

	_ = dev.WriteMessage(websocket.TextMessage, []byte(`{"type":"mic.state","state":"start"}`))
	if got := recvText(t, f.texts); !strings.Contains(got, `"state":"start"`) || !strings.Contains(got, `"mode":"auto"`) {
		t.Fatalf("expected listen start, got %s", got)
	}
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"listening"`) {
		t.Fatalf("expected listening light, got %s", got)
	}

	_ = dev.WriteMessage(websocket.BinaryMessage, []byte{9, 8, 7})
	select {
	case a := <-f.audio:
		if len(a) != 3 {
			t.Fatalf("audio len %d", len(a))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mic audio not forwarded")
	}

	_ = dev.WriteMessage(websocket.TextMessage, []byte(`{"type":"mic.state","state":"stop"}`))
	if got := recvText(t, f.texts); !strings.Contains(got, `"state":"stop"`) {
		t.Fatalf("expected listen stop, got %s", got)
	}
}

func TestAudioDroppedWhenMicOff(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	<-f.conn

	_ = dev.WriteMessage(websocket.BinaryMessage, []byte{1})
	select {
	case <-f.audio:
		t.Fatal("audio must not be forwarded while mic is off")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestTTSMapsToLightSpeakerAndAudio(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	linx := <-f.conn

	_ = linx.WriteJSON(map[string]any{"type": "stt", "text": "你好"})
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"thinking"`) {
		t.Fatalf("expected thinking, got %s", got)
	}

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "start"})
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"speaking"`) {
		t.Fatalf("expected speaking light, got %s", got)
	}
	if got := readDeviceText(t, dev); !strings.Contains(got, `"speaker.state"`) || !strings.Contains(got, `"source":"ai"`) {
		t.Fatalf("expected speaker.state start, got %s", got)
	}

	_ = linx.WriteMessage(websocket.BinaryMessage, []byte{5, 5})
	mt, data, err := dev.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || len(data) != 2 {
		t.Fatalf("expected tts audio frame, got mt=%d len=%d err=%v", mt, len(data), err)
	}

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "stop"})
	if got := readDeviceText(t, dev); !strings.Contains(got, `"speaker.state"`) || !strings.Contains(got, `"state":"stop"`) {
		t.Fatalf("expected speaker.state stop, got %s", got)
	}
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"idle"`) {
		t.Fatalf("expected idle light, got %s", got)
	}
}

func TestLinxUnavailableReportsErrorToDevice(t *testing.T) {
	dev := dialDevice(t, New(Config{URL: "ws://127.0.0.1:1"}))
	if got := readDeviceText(t, dev); !strings.Contains(got, `"type":"error"`) {
		t.Fatalf("expected error, got %s", got)
	}
}
