package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/config"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
	"github.com/LiusCraft/palpable/server/internal/store"
)

func bindApp(t *testing.T, ai ai) (*App, *httptest.Server) {
	t.Helper()
	d := deps{store: store.NewMemory(), requireBind: true,
		demo: config.Demo{Guardian: "g1", ParentToken: "tok", ManualBind: true}}
	if ai != nil {
		d.ai = ai
	}
	a, err := newApp(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.catalog.EnsureAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a.Start(ctx)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return a, srv
}

func api(t *testing.T, srv *httptest.Server, method, path, payload string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

// readUntil 读到指定类型的文本消息为止。
func readUntil(t *testing.T, ws *websocket.Conn, typ string) string {
	t.Helper()
	for {
		m := readText(t, ws)
		if strings.Contains(m, `"type":"`+typ+`"`) {
			return m
		}
	}
}

func TestUnboundDeviceOnlyGetsBindCode(t *testing.T) {
	f := &fakeAI{calls: make(chan string, 64)}
	a, srv := bindApp(t, f)
	ws := connect(t, srv, mac)

	var bc protocol.BindCode
	if err := json.Unmarshal([]byte(readUntil(t, ws, "bind.code")), &bc); err != nil || len(bc.Code) != 6 || bc.ExpiresIn != 600 {
		t.Fatalf("bind code %+v %v", bc, err)
	}
	// 绑定前：不服务。
	if a.serving(mac) {
		t.Fatal("unbound device must not be served")
	}
	_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"location","lat":31.2,"lng":121.5}`))
	_ = ws.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3})

	// 家长端：没勾 AI 陪伴同意、错码，都不行。
	if code, _ := api(t, srv, "POST", "/api/bindings", `{"code":"`+bc.Code+`"}`); code != http.StatusBadRequest {
		t.Fatalf("without consent: %d", code)
	}
	wrong := "000000"
	if wrong == bc.Code {
		wrong = "000001"
	}
	if code, _ := api(t, srv, "POST", "/api/bindings", `{"code":"`+wrong+`","ai_companion":true}`); code != http.StatusBadRequest {
		t.Fatalf("wrong code: %d", code)
	}
	if code, _ := api(t, srv, "POST", "/api/bindings", `{"code":"12"}`); code != http.StatusBadRequest {
		t.Fatalf("short code: %d", code)
	}
	code, body := api(t, srv, "POST", "/api/bindings", `{"code":"`+bc.Code+`","ai_companion":true,"voice_consent":false}`)
	if code != http.StatusCreated || !strings.Contains(body, string(mac)) {
		t.Fatalf("bind: %d %s", code, body)
	}
	if m := readUntil(t, ws, "bind.state"); !strings.Contains(m, `"bound"`) {
		t.Fatalf("got %s", m)
	}
	waitFor(t, func() bool { return a.serving(mac) })
	f.wantPrefix(t, "open ")
	p, _ := a.devices.Profile(context.Background(), mac)
	if p.Guardian != "g1" || p.VoiceConsent {
		t.Fatalf("profile %+v", p)
	}
	// 同一个码不能再用。
	if code, _ := api(t, srv, "POST", "/api/bindings", `{"code":"`+bc.Code+`","ai_companion":true}`); code != http.StatusBadRequest {
		t.Fatalf("reuse: %d", code)
	}
}

func TestUnbindResetsDevice(t *testing.T) {
	a, srv := bindApp(t, nil)
	ctx := context.Background()
	const other id.DeviceID = "02:00:00:00:00:02"
	ws := connect(t, srv, mac)
	var bc protocol.BindCode
	_ = json.Unmarshal([]byte(readUntil(t, ws, "bind.code")), &bc)
	if code, _ := api(t, srv, "POST", "/api/bindings", `{"code":"`+bc.Code+`","ai_companion":true}`); code != http.StatusCreated {
		t.Fatalf("bind: %d", code)
	}
	readUntil(t, ws, "bind.state") // bound
	waitFor(t, func() bool { return a.serving(mac) })
	if _, err := a.devices.Ensure(ctx, device.DeviceInfo{MAC: string(other)}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Friendships().Create(ctx, store.Friendship{ID: "f1", A: mac, B: other, Status: 1}); err != nil {
		t.Fatal(err)
	}

	// 别人的设备：404。
	if code, _ := api(t, srv, "DELETE", "/api/kids/"+string(other), ""); code != http.StatusNotFound {
		t.Fatalf("not mine: %d", code)
	}
	if code, _ := api(t, srv, "DELETE", "/api/kids/"+string(mac), ""); code != http.StatusNoContent {
		t.Fatalf("unbind: %d", code)
	}
	if m := readUntil(t, ws, "bind.state"); !strings.Contains(m, `"unbound"`) {
		t.Fatalf("got %s", m)
	}
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("device should be disconnected after unbind")
	}
	if _, err := a.devices.Profile(ctx, mac); err == nil {
		t.Fatal("profile should be deleted")
	}
	if fs, _ := a.rel.Friends(ctx, other); len(fs) != 0 {
		t.Fatalf("friendships should be gone: %v", fs)
	}
	// 重连后是一台新设备，重新播报绑定码。
	ws2 := connect(t, srv, mac)
	readUntil(t, ws2, "bind.code")
}
