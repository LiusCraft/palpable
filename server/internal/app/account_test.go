package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/protocol"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/wechat"
)

// wxApp 是只开微信登录、不配演示令牌的产品形态。
func wxApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	return wxAppWith(t, linxapi.NewFake())
}

func wxAppWith(t *testing.T, fake *linxapi.Fake) (*App, *httptest.Server) {
	t.Helper()
	a, err := newApp(context.Background(), deps{store: store.NewMemory(), requireBind: true, wx: wechat.Fake{}, linx: fake})
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

func wxCall(t *testing.T, srv *httptest.Server, method, path, tok, payload string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(payload))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

func signIn(t *testing.T, srv *httptest.Server, code string) string {
	t.Helper()
	status, body := wxCall(t, srv, "POST", "/api/session", "", `{"code":"`+code+`"}`)
	var out struct{ Token string }
	_ = json.Unmarshal([]byte(body), &out)
	if status != http.StatusCreated || out.Token == "" {
		t.Fatalf("sign in: %d %s", status, body)
	}
	return out.Token
}

func TestWeChatGuardianBindsAndDeletesAccount(t *testing.T) {
	a, srv := wxApp(t)
	ctx := context.Background()
	mom, dad := signIn(t, srv, "mom"), signIn(t, srv, "dad")
	if status, _ := wxCall(t, srv, "GET", "/api/kids", "tok", ""); status != http.StatusUnauthorized {
		t.Fatalf("demo token accepted without demo config: %d", status)
	}

	ws := connect(t, srv, mac)
	var bc protocol.BindCode
	_ = json.Unmarshal([]byte(readUntil(t, ws, "bind.code")), &bc)
	if status, body := wxCall(t, srv, "POST", "/api/bindings", mom, `{"code":"`+bc.Code+`","ai_companion":true}`); status != http.StatusCreated {
		t.Fatalf("bind: %d %s", status, body)
	}
	readUntil(t, ws, "bind.state")
	waitFor(t, func() bool { return a.serving(mac) })

	const other id.DeviceID = "02:00:00:00:00:02"
	if _, err := a.devices.Ensure(ctx, device.DeviceInfo{MAC: string(other)}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Friendships().Create(ctx, store.Friendship{ID: "f1", A: mac, B: other, Status: 1}); err != nil {
		t.Fatal(err)
	}
	if _, body := wxCall(t, srv, "GET", "/api/kids", dad, ""); body != "[]\n" {
		t.Fatalf("dad sees mom's kid: %s", body)
	}

	if status, _ := wxCall(t, srv, "DELETE", "/api/me", mom, ""); status != http.StatusNoContent {
		t.Fatalf("delete account: %d", status)
	}
	if m := readUntil(t, ws, "bind.state"); !strings.Contains(m, `"unbound"`) {
		t.Fatalf("device not reset: %s", m)
	}
	if _, err := a.devices.Profile(ctx, mac); err == nil {
		t.Fatal("device profile survives account deletion")
	}
	if fs, _ := a.rel.Friends(ctx, other); len(fs) != 0 {
		t.Fatalf("friendships survive account deletion: %v", fs)
	}
	if status, _ := wxCall(t, srv, "GET", "/api/kids", mom, ""); status != http.StatusUnauthorized {
		t.Fatalf("token survives account deletion: %d", status)
	}
}

func TestDeleteAccountDeletesVoiceprint(t *testing.T) {
	fake := linxapi.NewFake()
	a, srv := wxAppWith(t, fake)
	ctx := context.Background()
	mom := signIn(t, srv, "mom")
	ws := connect(t, srv, mac)
	var bc protocol.BindCode
	_ = json.Unmarshal([]byte(readUntil(t, ws, "bind.code")), &bc)
	if status, _ := wxCall(t, srv, "POST", "/api/bindings", mom, `{"code":"`+bc.Code+`","ai_companion":true,"voice_consent":true}`); status != http.StatusCreated {
		t.Fatalf("bind: %d", status)
	}
	readUntil(t, ws, "bind.state")
	// 引导期登记过声纹，并且（将来）绑到了智能体上。
	d, _ := a.store.Devices().Get(ctx, mac)
	d.SpeakerID = "spk-kid"
	if err := a.store.Devices().Update(ctx, d); err != nil {
		t.Fatal(err)
	}
	fake.Speakers["spk-kid"] = "mine.wav"
	fake.Bound[d.AgentID] = []string{"spk-kid"}

	if status, _ := wxCall(t, srv, "DELETE", "/api/me", mom, ""); status != http.StatusNoContent {
		t.Fatalf("delete account: %d", status)
	}
	if _, ok := fake.Speakers["spk-kid"]; ok || len(fake.DeletedSpeakers) != 1 {
		t.Fatalf("voiceprint survives account deletion: %v %v", fake.Speakers, fake.DeletedSpeakers)
	}
	if len(fake.Bound[d.AgentID]) != 0 {
		t.Fatalf("speaker still bound: %v", fake.Bound[d.AgentID])
	}
}
