package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/bridge/linx"
	"github.com/LiusCraft/palpable/server/internal/config"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/zone"
)

const mac id.DeviceID = "02:00:00:00:00:01"

const helloJSON = `{"type":"hello","device_id":"02:00:00:00:00:01","skin":"dragon","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":["mic","speaker"]}`

// fakeAI 记录 app 对 AI 的调用，验证分发表。
type fakeAI struct {
	calls   chan string
	aiState func(id.DeviceID, linx.AIState)
	onMCP   func(id.DeviceID, json.RawMessage)
}

func (f *fakeAI) Open(_ context.Context, dev linx.DeviceLink, p linx.Persona) error {
	f.calls <- fmt.Sprintf("open %s agent=%q", dev.DeviceID(), p.AgentID)
	return nil
}
func (f *fakeAI) Close(d id.DeviceID) { f.calls <- "close " + string(d) }
func (f *fakeAI) Forward(d id.DeviceID, frame []byte) {
	f.calls <- fmt.Sprintf("forward %s %d", d, len(frame))
}
func (f *fakeAI) Listen(d id.DeviceID, on bool)                { f.calls <- fmt.Sprintf("listen %s %v", d, on) }
func (f *fakeAI) Interrupt(d id.DeviceID)                      { f.calls <- "interrupt " + string(d) }
func (f *fakeAI) Available(id.DeviceID) bool                   { return true }
func (f *fakeAI) OnAIState(fn func(id.DeviceID, linx.AIState)) { f.aiState = fn }
func (f *fakeAI) SendMCP(d id.DeviceID, p json.RawMessage) error {
	f.calls <- "mcp " + string(d) + " " + string(p)
	return nil
}
func (f *fakeAI) OnMCP(fn func(id.DeviceID, json.RawMessage)) { f.onMCP = fn }
func (f *fakeAI) Say(_ context.Context, d id.DeviceID, text string) error {
	f.calls <- fmt.Sprintf("say %s %s", d, text)
	return nil
}

func (f *fakeAI) wantPrefix(t *testing.T, prefix string) {
	t.Helper()
	select {
	case got := <-f.calls:
		if !strings.HasPrefix(got, prefix) {
			t.Fatalf("call = %q, want prefix %q", got, prefix)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for %q", prefix)
	}
}

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
	if err := a.catalog.EnsureAll(context.Background()); err != nil {
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
	// 新设备：开通后以引导智能体开会话，小可先打招呼。
	f.wantPrefix(t, `open 02:00:00:00:00:01 agent="agent-`)
	f.wantPrefix(t, "say 02:00:00:00:00:01 你好呀！我是小可")

	send(websocket.TextMessage, `{"type":"mic.state","state":"start"}`)
	f.want(t, "listen 02:00:00:00:00:01 true")
	send(websocket.BinaryMessage, "abc")
	f.want(t, "forward 02:00:00:00:00:01 3")
	send(websocket.TextMessage, `{"type":"mic.state","state":"stop"}`)
	f.want(t, "listen 02:00:00:00:00:01 false")
	send(websocket.TextMessage, `{"type":"wake","source":"touch"}`)
	send(websocket.TextMessage, `{"type":"mic.state","state":"start"}`)
	f.want(t, "listen 02:00:00:00:00:01 true")

	// 位置在公园外：不触发 AI。
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

func TestParentAPIMountedOnlyWithToken(t *testing.T) {
	rec := httptest.NewRecorder()
	mustApp(t, nil).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/kids", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("without parent token /api must not exist: %d", rec.Code)
	}
	a, err := newApp(context.Background(), deps{store: store.NewMemory(), demo: config.Demo{Guardian: "g1", ParentToken: "tok"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/kids", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("/api/kids = %d %s", rec.Code, rec.Body.String())
	}
}

func TestWorldOnlyInDemoMode(t *testing.T) {
	rec := httptest.NewRecorder()
	mustApp(t, nil).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/world", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/world must be 404 by default: %d", rec.Code)
	}
	a, err := newApp(context.Background(), deps{store: store.NewMemory(), demo: config.Demo{World: true}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/world", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if got := readText(t, ws); !strings.Contains(got, `"kind":"snapshot"`) || !strings.Contains(got, `"公园"`) {
		t.Fatalf("snapshot = %s", got)
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	mustApp(t, nil).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("healthz = %d %q", rec.Code, rec.Body.String())
	}
}

func TestChannelJoinNotifiesOnlyFriends(t *testing.T) {
	const friend, stranger id.DeviceID = "02:00:00:00:00:02", "02:00:00:00:00:03"
	s := store.NewMemory()
	ctx := context.Background()
	if err := s.Friendships().Create(ctx, store.Friendship{ID: "f1", A: mac, B: friend, Status: int(relation.StatusConfirmed), CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	a, err := newApp(ctx, deps{store: s})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.catalog.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	a.chans.DefineZone("zone:park", "公园")
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	wsMe, wsFriend, wsStranger := connect(t, srv, mac), connect(t, srv, friend), connect(t, srv, stranger)

	for _, d := range []id.DeviceID{friend, stranger} {
		if err := a.chans.JoinZone(ctx, d, "zone:park"); err != nil {
			t.Fatal(err)
		}
		a.onChannel(ctx, <-a.chans.Subscribe())
	}
	readText(t, wsFriend) // channel.state
	readText(t, wsFriend) // sound
	readText(t, wsStranger)
	readText(t, wsStranger)

	if err := a.chans.JoinZone(ctx, mac, "zone:park"); err != nil {
		t.Fatal(err)
	}
	a.onChannel(ctx, <-a.chans.Subscribe())

	if got := readText(t, wsMe); !strings.Contains(got, `"type":"channel.state"`) || !strings.Contains(got, `"name":"公园"`) {
		t.Fatalf("channel.state = %s", got)
	}
	readText(t, wsMe) // sound joined
	if got := readText(t, wsMe); !strings.Contains(got, `"type":"presence"`) || !strings.Contains(got, string(friend)) {
		t.Fatalf("me should learn friend is here: %s", got)
	}
	if got := readText(t, wsFriend); !strings.Contains(got, `"type":"presence"`) || !strings.Contains(got, string(mac)) {
		t.Fatalf("friend presence = %s", got)
	}
	_ = wsStranger.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, data, err := wsStranger.ReadMessage(); err == nil {
		t.Fatalf("stranger must not be told: %s", data)
	}

	// 今日回顾记下了遇到的好朋友，陌生人不算。
	dg, err := a.history.Today(ctx, mac)
	if err != nil || len(dg.Met) != 1 || dg.Met[0].Friend != friend {
		t.Fatalf("today met = %+v %v", dg.Met, err)
	}
}

func TestEnteringParkAsksToJoin(t *testing.T) {
	f := &fakeAI{calls: make(chan string, 16)}
	a := mustApp(t, f)
	ctx := context.Background()
	// 引导期的设备不打扰。
	a.onZone(ctx, zone.Event{Kind: zone.Entered, Device: mac, Zone: zone.Park})
	select {
	case got := <-f.calls:
		t.Fatalf("fresh device asked: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
	if err := a.store.Devices().Create(ctx, store.Device{ID: mac, Stage: int(device.StageReady), CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	a.onZone(context.Background(), zone.Event{Kind: zone.Entered, Device: mac, Zone: zone.Park})
	select {
	case got := <-f.calls:
		if !strings.HasPrefix(got, "say "+string(mac)) || !strings.Contains(got, "公园") {
			t.Fatalf("call = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no Say")
	}
}

// 起名 → 小可说完 → 交棒到风格智能体 → 新小伙伴用名字打招呼。
func TestNamingHandsOffToStyleAgent(t *testing.T) {
	f := &fakeAI{calls: make(chan string, 32)}
	a := mustApp(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Start(ctx)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	connect(t, srv, mac)
	f.wantPrefix(t, "open ")
	f.wantPrefix(t, "say ")

	// 灵矽调用业务工具 set_names。
	f.onMCP(mac, json.RawMessage(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"self.profile.set_names","arguments":{"doll_name":"小龙","kid_name":"朵朵"}}}`))
	f.wantPrefix(t, `mcp 02:00:00:00:00:01 {"jsonrpc":"2.0","id":7,"result":{"content":[{"text":"记住啦`)

	// 小可说告别语。
	f.aiState(mac, linx.AISpeaking)
	time.Sleep(200 * time.Millisecond)
	f.aiState(mac, linx.AIIdle)

	f.want(t, "close "+string(mac))
	f.wantPrefix(t, `open 02:00:00:00:00:01 agent="`+a.catalog.ForSkin("dragon").AgentID)
	f.wantPrefix(t, "say 02:00:00:00:00:01 你好呀朵朵！我是小龙")
	p, _ := a.devices.Profile(ctx, mac)
	if p.Stage != device.StageReady || p.StyleID != "dragon" {
		t.Fatalf("profile after handoff = %+v", p)
	}
}
