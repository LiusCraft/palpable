package linx

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
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
	return newFakeLinxWithGate(t, nil)
}

// newFakeLinxWithGate 在 gate 非 nil 时先等它关闭再回 welcome，
// 用来模拟设备在灵矽握手完成前就打开了麦克风。
func newFakeLinxWithGate(t *testing.T, gate <-chan struct{}) (*fakeLinx, string) {
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
		if gate != nil {
			select {
			case <-gate:
			case <-time.After(2 * time.Second):
				return
			}
		}
		_ = ws.WriteJSON(map[string]any{
			"type": "hello", "transport": "websocket", "session_id": "linx-s1",
			"audio_params": map[string]any{"format": "opus", "sample_rate": 24000, "channels": 1, "frame_duration": 60},
		})
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

var (
	macRe  = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

func TestDialSendsLinxHandshakeHeaders(t *testing.T) {
	f, url := newFakeLinx(t)
	dialDevice(t, New(Config{URL: url, Token: "secret-token", AgentID: "agent-7"}))

	select {
	case h := <-f.headers:
		if h.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("Authorization = %q", h.Get("Authorization"))
		}
		if got := h.Get("Device-Id"); !macRe.MatchString(got) || got != PseudoMAC("plush-001") {
			t.Errorf("Device-Id = %q, want pseudo MAC of plush-001", got)
		}
		if h.Get("Protocol-Version") != "1" {
			t.Errorf("Protocol-Version = %q", h.Get("Protocol-Version"))
		}
		if got := h.Get("Client-Id"); !uuidRe.MatchString(got) {
			t.Errorf("Client-Id = %q, want UUID v4", got)
		}
		if h.Get("X-Agent-ID") != "agent-7" {
			t.Errorf("X-Agent-ID = %q", h.Get("X-Agent-ID"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("linx was never dialed")
	}
}

func TestAgentIDHeaderOmittedWhenUnset(t *testing.T) {
	f, url := newFakeLinx(t)
	dialDevice(t, New(Config{URL: url}))
	h := <-f.headers
	if _, ok := h["X-Agent-Id"]; ok {
		t.Errorf("X-Agent-ID must not be sent when unset, got %q", h.Get("X-Agent-ID"))
	}
}

func TestPseudoMAC(t *testing.T) {
	a, b := PseudoMAC("plush-001"), PseudoMAC("plush-002")
	if a == b {
		t.Fatal("different devices must get different MACs")
	}
	if a != PseudoMAC("plush-001") {
		t.Fatal("PseudoMAC must be stable")
	}
	if !macRe.MatchString(a) {
		t.Fatalf("bad MAC format %q", a)
	}
	first, _ := strconv.ParseUint(a[:2], 16, 8)
	if first&0x02 == 0 || first&0x01 != 0 {
		t.Fatalf("first octet %02x must be locally administered unicast", first)
	}
}

func TestDeviceGetsDownlinkAudioParamsFromLinx(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	<-f.conn
	got := readDeviceText(t, dev)
	if !strings.Contains(got, `"type":"audio.downlink"`) || !strings.Contains(got, `"sample_rate":24000`) || !strings.Contains(got, `"frame_duration":60`) {
		t.Fatalf("expected audio.downlink 24000/60, got %s", got)
	}
}

func TestMicStartForwardsListenAndAudio(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	<-f.conn
	readDeviceText(t, dev) // audio.downlink

	_ = dev.WriteMessage(websocket.TextMessage, []byte(`{"type":"mic.state","state":"start"}`))
	if got := recvText(t, f.texts); !strings.Contains(got, `"state":"start"`) || !strings.Contains(got, `"mode":"realtime"`) {
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

	// 重复的 start 不产生新的 listen，随后停止时的输出顺序可证明它已被处理。
	_ = dev.WriteMessage(websocket.TextMessage, []byte(`{"type":"mic.state","state":"start"}`))

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
	readDeviceText(t, dev) // audio.downlink

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

func TestWakeStartsListening(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	<-f.conn
	readDeviceText(t, dev) // audio.downlink

	_ = dev.WriteMessage(websocket.TextMessage, []byte(`{"type":"wake","source":"voice"}`))
	if got := recvText(t, f.texts); !strings.Contains(got, `"type":"listen"`) || !strings.Contains(got, `"state":"start"`) {
		t.Fatalf("expected listen start after wake, got %s", got)
	}
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"listening"`) {
		t.Fatalf("expected listening light, got %s", got)
	}
}

// 麦克风在灵矽握手完成前就打开时，握手后要补发 listen start。
func TestMicStartBeforeLinxReadyIsReplayed(t *testing.T) {
	gate := make(chan struct{})
	f, url := newFakeLinxWithGate(t, gate)
	dev := dialDevice(t, New(Config{URL: url}))

	_ = dev.WriteMessage(websocket.TextMessage, []byte(`{"type":"mic.state","state":"start"}`))
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"listening"`) {
		t.Fatalf("expected listening light, got %s", got)
	}

	close(gate)
	if got := readDeviceText(t, dev); !strings.Contains(got, `"type":"audio.downlink"`) {
		t.Fatalf("expected audio.downlink, got %s", got)
	}
	if got := recvText(t, f.texts); !strings.Contains(got, `"type":"listen"`) || !strings.Contains(got, `"state":"start"`) {
		t.Fatalf("expected replayed listen start, got %s", got)
	}
}

func TestTTSStopKeepsListeningLightWhenMicOn(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	linx := <-f.conn
	readDeviceText(t, dev) // audio.downlink

	_ = dev.WriteMessage(websocket.TextMessage, []byte(`{"type":"mic.state","state":"start"}`))
	recvText(t, f.texts) // listen start
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"listening"`) {
		t.Fatalf("expected listening light, got %s", got)
	}

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "start"})
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"speaking"`) {
		t.Fatalf("expected speaking light, got %s", got)
	}
	readDeviceText(t, dev) // speaker.state start

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "stop"})
	readDeviceText(t, dev) // speaker.state stop
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"listening"`) {
		t.Fatalf("expected listening light after tts stop, got %s", got)
	}
}

func TestBadLinxMessageIsIgnored(t *testing.T) {
	f, url := newFakeLinx(t)
	dev := dialDevice(t, New(Config{URL: url}))
	linx := <-f.conn
	readDeviceText(t, dev) // audio.downlink

	_ = linx.WriteMessage(websocket.TextMessage, []byte("{"))
	// 坏消息之后连接仍可用：随后的 stt 应触发 thinking。
	_ = linx.WriteJSON(map[string]any{"type": "stt", "text": "你好"})
	if got := readDeviceText(t, dev); !strings.Contains(got, `"state":"thinking"`) {
		t.Fatalf("expected thinking light, got %s", got)
	}
}

func TestLinxWelcomeReadFailureReportsError(t *testing.T) {
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_, _, _ = ws.ReadMessage() // hello
		ws.Close()
	}))
	t.Cleanup(srv.Close)

	dev := dialDevice(t, New(Config{URL: "ws" + strings.TrimPrefix(srv.URL, "http")}))
	if got := readDeviceText(t, dev); !strings.Contains(got, `"type":"error"`) {
		t.Fatalf("expected error, got %s", got)
	}
}

func TestUnexpectedLinxWelcomeReportsError(t *testing.T) {
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_, _, _ = ws.ReadMessage() // hello
		_ = ws.WriteJSON(map[string]any{"type": "nope"})
	}))
	t.Cleanup(srv.Close)

	dev := dialDevice(t, New(Config{URL: "ws" + strings.TrimPrefix(srv.URL, "http")}))
	if got := readDeviceText(t, dev); !strings.Contains(got, `"type":"error"`) {
		t.Fatalf("expected error, got %s", got)
	}
}

func TestWelcomeNormalize(t *testing.T) {
	var empty welcome
	empty.normalize()
	if empty.AudioParams.SampleRate != 16000 || empty.AudioParams.FrameDuration != 60 {
		t.Fatalf("defaults not applied: %+v", empty.AudioParams)
	}

	full := welcome{}
	full.AudioParams.SampleRate = 24000
	full.AudioParams.FrameDuration = 40
	full.normalize()
	if full.AudioParams.SampleRate != 24000 || full.AudioParams.FrameDuration != 40 {
		t.Fatalf("explicit params overwritten: %+v", full.AudioParams)
	}
}

func TestParseLinxRejectsMalformedJSON(t *testing.T) {
	if _, err := parseLinx([]byte("{")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestWriteLinxBeforeConnectFails(t *testing.T) {
	l := &link{}
	if err := l.writeLinx(map[string]any{"type": "listen"}); err == nil {
		t.Fatal("expected error before linx is connected")
	}
}

func TestSendListenBeforeConnectIsDropped(t *testing.T) {
	l := &link{}
	l.sendListen("start") // 只记录 debug 日志，不应 panic
}
