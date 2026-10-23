package linx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

const mac id.DeviceID = "02:00:00:00:00:01"

// fakeLinx 模拟灵矽：校验握手头，记录收到的帧，并允许测试主动下发消息或断开。
type fakeLinx struct {
	headers chan http.Header
	hellos  chan map[string]any
	texts   chan string
	audio   chan []byte
	conns   chan *websocket.Conn
	url     string
}

func newFakeLinx(t *testing.T) *fakeLinx {
	f := &fakeLinx{
		headers: make(chan http.Header, 8),
		hellos:  make(chan map[string]any, 8),
		texts:   make(chan string, 32),
		audio:   make(chan []byte, 32),
		conns:   make(chan *websocket.Conn, 8),
	}
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		f.headers <- r.Header.Clone()
		// 灵矽先收 hello，再回 hello。
		var hello map[string]any
		if err := ws.ReadJSON(&hello); err != nil {
			return
		}
		f.hellos <- hello
		_ = ws.WriteJSON(map[string]any{
			"type": "hello", "transport": "websocket", "session_id": "linx-s1",
			"audio_params": map[string]any{"format": "opus", "sample_rate": 24000, "channels": 1, "frame_duration": 60},
		})
		f.conns <- ws
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
	f.url = "ws" + strings.TrimPrefix(srv.URL, "http")
	return f
}

// fakeDev 是测试用的 DeviceLink。
type fakeDev struct {
	ctx    context.Context
	cancel context.CancelFunc
	msgs   chan protocol.Message
	audio  chan []byte
}

func newFakeDev(t *testing.T) *fakeDev {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &fakeDev{ctx: ctx, cancel: cancel, msgs: make(chan protocol.Message, 32), audio: make(chan []byte, 32)}
}

func (d *fakeDev) DeviceID() id.DeviceID              { return mac }
func (d *fakeDev) Context() context.Context           { return d.ctx }
func (d *fakeDev) Send(m protocol.Message) error      { d.msgs <- m; return nil }
func (d *fakeDev) SendAudio(frame []byte) error       { d.audio <- frame; return nil }
func (d *fakeDev) next(t *testing.T) protocol.Message { t.Helper(); return recv(t, d.msgs) }

func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
		panic("unreachable")
	}
}

func noRecv[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("unexpected %s: %v", what, v)
	case <-time.After(200 * time.Millisecond):
	}
}

func wantLight(t *testing.T, m protocol.Message, state string) {
	t.Helper()
	if l, ok := m.(*protocol.Light); !ok || l.State != state {
		t.Fatalf("want light %s, got %#v", state, m)
	}
}

func wantSpeaker(t *testing.T, m protocol.Message, state string) {
	t.Helper()
	if s, ok := m.(*protocol.SpeakerState); !ok || s.State != state || s.Source != "ai" {
		t.Fatalf("want speaker.state %s/ai, got %#v", state, m)
	}
}

func fastBridge(url string) *Bridge {
	b := New(Config{URL: url, Token: "secret-token"})
	b.backoffMin, b.backoffMax = 10*time.Millisecond, 50*time.Millisecond
	return b
}

// open 开会话并等到连上，跳过 audio.downlink。
func open(t *testing.T, f *fakeLinx, p Persona) (*Bridge, *fakeDev, *websocket.Conn) {
	t.Helper()
	b := fastBridge(f.url)
	dev := newFakeDev(t)
	if err := b.Open(context.Background(), dev, p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close(mac) })
	ws := recv(t, f.conns)
	if _, ok := dev.next(t).(*protocol.AudioDownlink); !ok {
		t.Fatal("first message to device must be audio.downlink")
	}
	return b, dev, ws
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestHandshakeUsesDeviceMACAndPersona(t *testing.T) {
	f := newFakeLinx(t)
	open(t, f, Persona{AgentID: "agent-7", DollName: "小龙", KidName: "朵朵", VoiceStep: true})

	h := recv(t, f.headers)
	if h.Get("Authorization") != "Bearer secret-token" {
		t.Errorf("Authorization = %q", h.Get("Authorization"))
	}
	if h.Get("Device-Id") != string(mac) {
		t.Errorf("Device-Id = %q, want device MAC", h.Get("Device-Id"))
	}
	if h.Get("Protocol-Version") != "1" {
		t.Errorf("Protocol-Version = %q", h.Get("Protocol-Version"))
	}
	if !uuidRe.MatchString(h.Get("Client-Id")) {
		t.Errorf("Client-Id = %q, want UUID v4", h.Get("Client-Id"))
	}
	if h.Get("X-Agent-ID") != "agent-7" {
		t.Errorf("X-Agent-ID = %q", h.Get("X-Agent-ID"))
	}

	hello := recv(t, f.hellos)
	vars := hello["agent_params"].(map[string]any)["custom_replace_prompt"].(map[string]any)
	if vars["doll_name"] != "小龙" || vars["kid_name"] != "朵朵" || vars["voice_step"] != "true" {
		t.Errorf("custom_replace_prompt = %v", vars)
	}
}

func TestAgentIDHeaderOmittedWhenUnset(t *testing.T) {
	f := newFakeLinx(t)
	open(t, f, Persona{})
	if h := recv(t, f.headers); h.Get("X-Agent-ID") != "" {
		t.Errorf("X-Agent-ID must not be sent when unset, got %q", h.Get("X-Agent-ID"))
	}
}

func TestDownlinkAudioParamsComeFromLinx(t *testing.T) {
	f := newFakeLinx(t)
	b := fastBridge(f.url)
	dev := newFakeDev(t)
	_ = b.Open(context.Background(), dev, Persona{})
	defer b.Close(mac)
	m, ok := dev.next(t).(*protocol.AudioDownlink)
	if !ok || m.SampleRate != 24000 || m.FrameDuration != 60 {
		t.Fatalf("expected audio.downlink 24000/60, got %#v", m)
	}
}

func TestListenForwardsAudio(t *testing.T) {
	f := newFakeLinx(t)
	b, dev, _ := open(t, f, Persona{})

	b.Forward(mac, []byte{1})
	noRecv(t, f.audio, "audio while not listening")

	b.Listen(mac, true)
	if got := recv(t, f.texts); !strings.Contains(got, `"state":"start"`) || !strings.Contains(got, `"mode":"realtime"`) {
		t.Fatalf("expected listen start, got %s", got)
	}
	wantLight(t, dev.next(t), "listening")

	b.Forward(mac, []byte{9, 8, 7})
	if a := recv(t, f.audio); len(a) != 3 {
		t.Fatalf("audio len %d", len(a))
	}

	b.Listen(mac, false)
	if got := recv(t, f.texts); !strings.Contains(got, `"state":"stop"`) {
		t.Fatalf("expected listen stop, got %s", got)
	}
	wantLight(t, dev.next(t), "idle")
}

func TestTTSMapsToLightSpeakerAudioAndAIState(t *testing.T) {
	f := newFakeLinx(t)
	b, dev, linx := open(t, f, Persona{})
	states := make(chan AIState, 4)
	b.OnAIState(func(d id.DeviceID, s AIState) {
		if d == mac {
			states <- s
		}
	})

	_ = linx.WriteJSON(map[string]any{"type": "stt", "text": "你好"})
	wantLight(t, dev.next(t), "thinking")

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "start"})
	wantLight(t, dev.next(t), "speaking")
	wantSpeaker(t, dev.next(t), "start")
	if s := recv(t, states); s != AISpeaking {
		t.Fatalf("ai state = %v", s)
	}
	if !b.Available(mac) {
		t.Fatal("Available must be true while connected")
	}

	_ = linx.WriteMessage(websocket.BinaryMessage, []byte{5, 5})
	if a := recv(t, dev.audio); len(a) != 2 {
		t.Fatalf("tts frame len %d", len(a))
	}

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "stop"})
	wantSpeaker(t, dev.next(t), "stop")
	wantLight(t, dev.next(t), "idle")
	if s := recv(t, states); s != AIIdle {
		t.Fatalf("ai state = %v", s)
	}
}

func TestSayWaitsForSessionAndSendsTextResponse(t *testing.T) {
	f := newFakeLinx(t)
	b := fastBridge(f.url)
	dev := newFakeDev(t)
	_ = b.Open(context.Background(), dev, Persona{})
	defer b.Close(mac)

	// 刚 Open 还没连上，Say 应等待而不是失败。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.Say(ctx, mac, "你好呀，我是小可"); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(recv(t, f.texts)), &m)
	if m["type"] != "listen" || m["state"] != "detect" || m["text_response"] != "你好呀，我是小可" || m["session_id"] != "linx-s1" {
		t.Fatalf("unexpected say message %v", m)
	}
	if _, has := m["text"]; has {
		t.Fatal("say must not send text, which would be fed to the LLM as a wake word")
	}

	if err := b.Say(ctx, "02:00:00:00:00:99", "x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("say to unknown device: %v", err)
	}
}

func TestInterruptAbortsAndDropsRemainingTTS(t *testing.T) {
	f := newFakeLinx(t)
	b, dev, linx := open(t, f, Persona{})

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "start"})
	wantLight(t, dev.next(t), "speaking")
	wantSpeaker(t, dev.next(t), "start")

	b.Interrupt(mac)
	if got := recv(t, f.texts); !strings.Contains(got, `"type":"abort"`) {
		t.Fatalf("expected abort, got %s", got)
	}
	wantSpeaker(t, dev.next(t), "stop")
	wantLight(t, dev.next(t), "idle")

	_ = linx.WriteMessage(websocket.BinaryMessage, []byte{1})
	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "stop"})
	noRecv(t, dev.audio, "tts audio after interrupt")
	noRecv(t, dev.msgs, "message after interrupted tts stop")

	// 下一轮正常播放。
	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "start"})
	wantLight(t, dev.next(t), "speaking")
	wantSpeaker(t, dev.next(t), "start")
	_ = linx.WriteMessage(websocket.BinaryMessage, []byte{2})
	recv(t, dev.audio)
}

func TestReconnectsAfterLinxDrops(t *testing.T) {
	f := newFakeLinx(t)
	b, dev, linx := open(t, f, Persona{})
	b.Listen(mac, true)
	recv(t, f.texts) // listen start
	wantLight(t, dev.next(t), "listening")

	_ = linx.WriteJSON(map[string]any{"type": "tts", "state": "start"})
	wantLight(t, dev.next(t), "speaking")
	wantSpeaker(t, dev.next(t), "start")

	_ = linx.Close()
	// 断线时正在说话：设备收到 stop，回到 listening。
	wantSpeaker(t, dev.next(t), "stop")
	wantLight(t, dev.next(t), "listening")

	recv(t, f.conns)
	if _, ok := dev.next(t).(*protocol.AudioDownlink); !ok {
		t.Fatal("reconnect must resend audio.downlink")
	}
	// 重连后恢复收音。
	if got := recv(t, f.texts); !strings.Contains(got, `"state":"start"`) {
		t.Fatalf("expected listen start after reconnect, got %s", got)
	}
}

func TestUnreachableLinxReportsErrorOnceAndKeepsRetrying(t *testing.T) {
	b := fastBridge("ws://127.0.0.1:1")
	dev := newFakeDev(t)
	_ = b.Open(context.Background(), dev, Persona{})
	defer b.Close(mac)

	if e, ok := dev.next(t).(*protocol.Error); !ok || e.Code != protocol.CodeInternal {
		t.Fatalf("expected error, got %#v", e)
	}
	noRecv(t, dev.msgs, "repeated error")
	if b.Available(mac) {
		t.Fatal("Available must be false while disconnected")
	}
}

func TestOpenTwiceFailsAndCloseThenOpenSwitchesAgent(t *testing.T) {
	f := newFakeLinx(t)
	b, dev, _ := open(t, f, Persona{AgentID: "welcome"})
	recv(t, f.headers)

	if err := b.Open(context.Background(), dev, Persona{AgentID: "dragon"}); !errors.Is(err, ErrAlreadyOpen) {
		t.Fatalf("second Open: %v", err)
	}

	b.Close(mac)
	if b.Available(mac) {
		t.Fatal("Available after Close")
	}
	if err := b.Open(context.Background(), dev, Persona{AgentID: "dragon"}); err != nil {
		t.Fatal(err)
	}
	if h := recv(t, f.headers); h.Get("X-Agent-ID") != "dragon" {
		t.Fatalf("X-Agent-ID after handoff = %q", h.Get("X-Agent-ID"))
	}
}

func TestDeviceDisconnectReleasesEverything(t *testing.T) {
	f := newFakeLinx(t)
	before := runtime.NumGoroutine()

	b := fastBridge(f.url)
	dev := newFakeDev(t)
	_ = b.Open(context.Background(), dev, Persona{})
	recv(t, f.conns)

	dev.cancel()
	deadline := time.Now().Add(2 * time.Second)
	for b.link(mac) != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if b.link(mac) != nil {
		t.Fatal("link not removed after device disconnect")
	}
	b.Close(mac) // 已自行退出时也安全

	// 灵矽一侧的服务 goroutine 也会随连接关闭退出。
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines leaked: before=%d after=%d\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}
