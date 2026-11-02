package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/adminhook"
	"github.com/LiusCraft/palpable/server/internal/config"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/palpclient"
	"github.com/LiusCraft/palpable/server/internal/protocol"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	opsToken = "ops-token-0123456789"
	hookTok  = "hook-token-0123456789"
	hmacKey  = "owner-hmac-key-0123456789"
)

type adminSink struct {
	mu     sync.Mutex
	events []adminhook.Event
	auth   string
}

func (s *adminSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var e adminhook.Event
	_ = json.NewDecoder(r.Body).Decode(&e)
	s.mu.Lock()
	s.events = append(s.events, e)
	s.auth = r.Header.Get("Authorization")
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *adminSink) got() []adminhook.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]adminhook.Event(nil), s.events...)
}

// opsApp 起一个开放了 /internal 并向（假）厂家后台推送事件的 palpabled。
func opsApp(t *testing.T, withToken bool) (*App, *httptest.Server, *adminSink) {
	t.Helper()
	sink := &adminSink{}
	admin := httptest.NewServer(sink)
	t.Cleanup(admin.Close)
	o := config.Ops{AdminURL: admin.URL, AdminToken: hookTok, OwnerHMACKey: hmacKey}
	if withToken {
		o.Token = opsToken
	}
	d := deps{store: store.NewMemory(), requireBind: true, ops: o,
		demo: config.Demo{Guardian: "g1", ParentToken: "tok", ManualBind: true}}
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
	return a, srv, sink
}

func opsCall(t *testing.T, srv *httptest.Server, method, path, body string) int {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+"/internal/devices/"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+opsToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode
}

func spkiB64(t *testing.T) string {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	return base64.StdEncoding.EncodeToString(der)
}

// dialRaw 发 hello 并返回第一条回复；用来看被拒绝的设备收到什么。
func dialRaw(t *testing.T, srv *httptest.Server, d id.DeviceID) (*websocket.Conn, string) {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	hello := strings.Replace(helloJSON, "02:00:00:00:00:01", string(d), 1)
	_ = ws.WriteMessage(websocket.TextMessage, []byte(hello))
	return ws, readText(t, ws)
}

func TestOpsRoutesNeedToken(t *testing.T) {
	_, srv, _ := opsApp(t, false)
	if code := opsCall(t, srv, "POST", string(mac)+"/wipe", ""); code != http.StatusNotFound {
		t.Fatalf("/internal must not exist without ops.token, got %d", code)
	}
	_, srv2, _ := opsApp(t, true)
	req, _ := http.NewRequest("POST", srv2.URL+"/internal/devices/"+string(mac)+"/wipe", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token = %d", res.StatusCode)
	}
}

func TestOpsWipeFactoryResetsAnyDevice(t *testing.T) {
	a, srv, _ := opsApp(t, true)
	ctx := context.Background()
	const other id.DeviceID = "02:00:00:00:00:02"
	for _, d := range []id.DeviceID{mac, other} {
		if _, err := a.devices.Ensure(ctx, device.DeviceInfo{MAC: string(d)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.devices.Bind(ctx, mac, "g1"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Friendships().Create(ctx, store.Friendship{ID: "f1", A: mac, B: other, Status: 1}); err != nil {
		t.Fatal(err)
	}

	if code := opsCall(t, srv, "POST", string(mac)+"/wipe", ""); code != http.StatusNoContent {
		t.Fatalf("wipe = %d", code)
	}
	if _, err := a.devices.Profile(ctx, mac); err == nil {
		t.Fatal("profile survived the wipe")
	}
	if fs, _ := a.rel.Friends(ctx, other); len(fs) != 0 {
		t.Fatalf("friend of the wiped device remains: %v", fs)
	}
	if _, err := a.devices.Profile(ctx, other); err != nil {
		t.Fatalf("wipe touched another device: %v", err)
	}
	// 幂等，且设备不存在也成功。
	if code := opsCall(t, srv, "POST", string(mac)+"/wipe", ""); code != http.StatusNoContent {
		t.Fatalf("second wipe = %d", code)
	}
	if code := opsCall(t, srv, "POST", "02:00:00:00:00:99/wipe", ""); code != http.StatusNoContent {
		t.Fatalf("wipe unknown = %d", code)
	}
	// 未绑定的新设备同样能擦。
	if code := opsCall(t, srv, "POST", string(other)+"/wipe", ""); code != http.StatusNoContent {
		t.Fatalf("wipe unbound = %d", code)
	}
}

func TestOpsLockKicksAndRefuses(t *testing.T) {
	a, srv, _ := opsApp(t, true)
	ws := connect(t, srv, mac)
	readUntil(t, ws, "bind.code")

	if code := opsCall(t, srv, "PUT", string(mac)+"/lock", `{"locked":true}`); code != http.StatusNoContent {
		t.Fatalf("lock = %d", code)
	}
	if m := readUntil(t, ws, "error"); !strings.Contains(m, "locked") {
		t.Fatalf("kicked with %s", m)
	}
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("connection should be closed after lock")
	}
	if _, first := dialRaw(t, srv, mac); !strings.Contains(first, `"unauthorized"`) {
		t.Fatalf("locked device reconnect got %s", first)
	}
	if _, ok := a.gw.Session(mac); ok {
		t.Fatal("locked device has a session")
	}
	if code := opsCall(t, srv, "PUT", string(mac)+"/lock", `{"locked":false}`); code != http.StatusNoContent {
		t.Fatalf("unlock = %d", code)
	}
	connect(t, srv, mac) // 解锁后能连上
}

func TestOpsEnrollAndRevoke(t *testing.T) {
	a, srv, _ := opsApp(t, true)
	ctx := context.Background()
	key := spkiB64(t)
	body := `{"public_key":"` + key + `"}`
	if code := opsCall(t, srv, "PUT", string(mac)+"/key", body); code != http.StatusNoContent {
		t.Fatalf("enroll = %d", code)
	}
	if code := opsCall(t, srv, "PUT", string(mac)+"/key", body); code != http.StatusNoContent {
		t.Fatalf("re-enroll = %d", code)
	}
	if code := opsCall(t, srv, "PUT", string(mac)+"/key", `{"public_key":"`+spkiB64(t)+`"}`); code != http.StatusConflict {
		t.Fatalf("different key = %d", code)
	}
	if k, err := a.store.DeviceKeys().Get(ctx, mac); err != nil || base64.StdEncoding.EncodeToString(k.PublicKey) != key {
		t.Fatalf("stored key: %v", err)
	}

	ws := connect(t, srv, mac) // enrollment 为空：不做签名认证，这里只验证吊销踢线
	if code := opsCall(t, srv, "DELETE", string(mac)+"/key", ""); code != http.StatusNoContent {
		t.Fatalf("revoke = %d", code)
	}
	// 读到连接断开为止（中间可能还有 bind.code 之类的消息）。
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break
		}
	}
	if _, first := dialRaw(t, srv, mac); !strings.Contains(first, `"unauthorized"`) {
		t.Fatalf("revoked device reconnect got %s", first)
	}
	if code := opsCall(t, srv, "PUT", string(mac)+"/key", body); code != http.StatusConflict {
		t.Fatalf("enroll after revoke = %d", code)
	}
	if code := opsCall(t, srv, "PUT", string(mac)+"/lock", `{"locked":false}`); code != http.StatusConflict {
		t.Fatalf("unlock of revoked = %d", code)
	}
}

func TestBindAndUnbindAreReportedToAdmin(t *testing.T) {
	a, srv, sink := opsApp(t, true)
	ws := connect(t, srv, mac)
	var bc protocol.BindCode
	_ = json.Unmarshal([]byte(readUntil(t, ws, "bind.code")), &bc)
	if code, _ := api(t, srv, "POST", "/api/bindings", `{"code":"`+bc.Code+`","ai_companion":true}`); code != http.StatusCreated {
		t.Fatalf("bind: %d", code)
	}
	waitFor(t, func() bool { return len(sink.got()) == 1 })
	e := sink.got()[0]
	if e.Event != "activated" || e.MAC != string(mac) || e.OwnerRef != a.ownerRef("g1") {
		t.Fatalf("activated event: %+v", e)
	}
	if strings.Contains(e.OwnerRef, "g1") || len(e.OwnerRef) != 64 {
		t.Fatalf("owner_ref must be an opaque HMAC, got %q", e.OwnerRef)
	}
	if a.ownerRef("g2") == e.OwnerRef {
		t.Fatal("different guardians must give different owner refs")
	}
	if sink.auth != "Bearer "+hookTok {
		t.Fatalf("hook auth %q", sink.auth)
	}

	if code, _ := api(t, srv, "DELETE", "/api/kids/"+string(mac), ""); code != http.StatusNoContent {
		t.Fatalf("unbind: %d", code)
	}
	waitFor(t, func() bool { return len(sink.got()) == 2 })
	if e := sink.got()[1]; e.Event != "unbound" || e.MAC != string(mac) || e.OwnerRef != "" {
		t.Fatalf("unbound event: %+v", e)
	}
}

func TestAdminWipeDoesNotReportBack(t *testing.T) {
	a, srv, sink := opsApp(t, true)
	ctx := context.Background()
	_, _ = a.devices.Ensure(ctx, device.DeviceInfo{MAC: string(mac)})
	_ = a.devices.Bind(ctx, mac, "g1")
	if code := opsCall(t, srv, "POST", string(mac)+"/wipe", ""); code != http.StatusNoContent {
		t.Fatalf("wipe = %d", code)
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(sink.got()); n != 0 {
		t.Fatalf("a wipe ordered by the admin must not echo an event back, got %d", n)
	}
}

func TestDemoBindIsReportedToAdmin(t *testing.T) {
	sink := &adminSink{}
	admin := httptest.NewServer(sink)
	defer admin.Close()
	d := deps{store: store.NewMemory(), demo: config.Demo{Guardian: "demo-parent"},
		ops: config.Ops{AdminURL: admin.URL, AdminToken: hookTok, OwnerHMACKey: hmacKey}}
	a, err := newApp(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Start(ctx)
	if err := a.catalog.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := a.devices.Ensure(ctx, device.DeviceInfo{MAC: string(mac)})
	if err != nil {
		t.Fatal(err)
	}
	a.applyDemo(ctx, p)
	waitFor(t, func() bool { return len(sink.got()) == 1 })
	if sink.got()[0].Event != "activated" {
		t.Fatalf("%+v", sink.got())
	}
}

func TestNewRejectsWeakOpsConfig(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "p.db")}
	cfg.Ops.Token = "short"
	if _, err := New(context.Background(), cfg); err == nil {
		t.Fatal("a weak ops token must prevent startup")
	}
}

// 后台用的 palpclient 与这里的 /internal 路由必须说同一套话：契约测试，防止两边各改各的。
func TestPalpclientSpeaksToInternalAPI(t *testing.T) {
	a, srv, _ := opsApp(t, true)
	ctx := context.Background()
	c := palpclient.New(srv.URL, opsToken, nil)
	der, _ := base64.StdEncoding.DecodeString(spkiB64(t))
	const d = "aa:bb:cc:00:00:01"

	if err := c.Enroll(ctx, d, der); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if err := c.Enroll(ctx, d, der); err != nil {
		t.Fatalf("enroll twice: %v", err)
	}
	if err := c.SetLock(ctx, d, true); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := a.registry.Admit(ctx, d); err == nil {
		t.Fatal("device should be locked")
	}
	if err := c.SetLock(ctx, d, false); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := c.Wipe(ctx, d); err != nil {
		t.Fatalf("wipe: %v", err)
	}
	if err := c.Revoke(ctx, d); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := c.Enroll(ctx, d, der); err == nil {
		t.Fatal("enroll after revoke must fail")
	}
	if err := palpclient.New(srv.URL, "wrong-token", nil).Wipe(ctx, d); err == nil {
		t.Fatal("a wrong token must be rejected")
	}
}
