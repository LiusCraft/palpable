package adminapi_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/LiusCraft/palpable/server/internal/adminapi"
	"github.com/LiusCraft/palpable/server/internal/factory"
	"github.com/LiusCraft/palpable/server/internal/factorydb"
)

type plat struct{ fail bool }

func (p plat) Enroll(context.Context, string, []byte) error { return p.err() }
func (p plat) Revoke(context.Context, string) error         { return p.err() }
func (p plat) Wipe(context.Context, string) error           { return p.err() }
func (p plat) SetLock(context.Context, string, bool) error  { return p.err() }
func (p plat) err() error {
	if p.fail {
		return http.ErrHandlerTimeout
	}
	return nil
}

type env struct {
	t   *testing.T
	srv *httptest.Server
	tok string
}

func newEnv(t *testing.T, p factory.Platform, inbound string) *env {
	t.Helper()
	ctx := context.Background()
	db, err := factorydb.Open(ctx, t.TempDir()+"/a.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := factory.NewService(db, p, factory.Options{})
	auth := factory.NewAuth(db, nil)
	if _, err := auth.Bootstrap(ctx, "boss", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	static := fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}, "assets/app.js": {Data: []byte("js")}}
	srv := httptest.NewServer(adminapi.New(svc, auth, inbound, static))
	t.Cleanup(srv.Close)
	e := &env{t: t, srv: srv}
	var out struct{ Token string }
	if code := e.call("", "POST", "/api/login", map[string]string{"username": "boss", "password": "correct horse battery"}, &out); code != 200 {
		t.Fatalf("login %d", code)
	}
	e.tok = out.Token
	return e
}

func (e *env) call(tok, method, path string, body, out any) int {
	e.t.Helper()
	var rd *bytes.Reader
	if s, ok := body.(string); ok {
		rd = bytes.NewReader([]byte(s))
	} else {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (e *env) as(method, path string, body, out any) int {
	e.t.Helper()
	return e.call(e.tok, method, path, body, out)
}

func key(t *testing.T) string {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	return base64.StdEncoding.EncodeToString(der)
}

func (e *env) importBatch(code string, n int) []string {
	e.t.Helper()
	units := []map[string]string{}
	for i := 0; i < n; i++ {
		units = append(units, map[string]string{"mac": fmt.Sprintf("aa:bb:%02x:%02x:%02x:%02x", code[0], code[1], code[2], i), "public_key": key(e.t)})
	}
	var out struct{ Serials []string }
	if c := e.as("POST", "/api/batches", map[string]any{"code": code, "model": "可感", "skin": "bear", "units": units}, &out); c != 201 {
		e.t.Fatalf("import %d", c)
	}
	return out.Serials
}

func TestAuthRequired(t *testing.T) {
	e := newEnv(t, plat{}, "")
	for _, p := range []string{"/api/me", "/api/stats", "/api/units", "/api/batches", "/api/channels", "/api/events", "/api/staff"} {
		if c := e.call("", "GET", p, nil, nil); c != 401 {
			t.Errorf("GET %s without token = %d", p, c)
		}
		if c := e.call("bad", "GET", p, nil, nil); c != 401 {
			t.Errorf("GET %s with bad token = %d", p, c)
		}
	}
	if c := e.call("", "POST", "/api/login", map[string]string{"username": "boss", "password": "nope-nope-nope"}, nil); c != 401 {
		t.Errorf("bad login = %d", c)
	}
	if c := e.call("", "POST", "/api/login", `{"username":"boss","extra":1}`, nil); c != 400 {
		t.Errorf("unknown field = %d", c)
	}
	if c := e.call("", "POST", "/api/login", `{"username":"boss"} {}`, nil); c != 400 {
		t.Errorf("trailing JSON = %d", c)
	}
	var h struct{}
	if c := e.call("", "GET", "/healthz", nil, &h); c != 204 {
		t.Errorf("healthz = %d", c)
	}
}

func TestLifecycleOverHTTP(t *testing.T) {
	e := newEnv(t, plat{}, "")
	if c := e.as("PUT", "/api/channels/jd", map[string]string{"name": "京东自营", "kind": "online"}, nil); c != 204 {
		t.Fatalf("channel %d", c)
	}
	var chs []map[string]string
	if e.as("GET", "/api/channels", nil, &chs); len(chs) != 1 || chs[0]["code"] != "jd" {
		t.Fatalf("channels %v", chs)
	}
	serials := e.importBatch("H1000", 2)
	if len(serials) != 2 {
		t.Fatal(serials)
	}
	s := serials[0]
	act := func(path string, body any) (int, map[string]any) {
		var out map[string]any
		c := e.as("POST", path, body, &out)
		return c, out
	}
	if c, out := act("/api/units/"+s+"/actions", map[string]any{"action": "qc_pass"}); c != 200 || out["state"] != "in_stock" {
		t.Fatalf("qc_pass %d %v", c, out)
	}
	if c, _ := act("/api/units/"+s+"/actions", map[string]any{"action": "sell", "order_ref": "A-1"}); c != 409 {
		t.Fatalf("sell from stock = %d", c)
	}
	if c, _ := act("/api/units/"+s+"/actions", map[string]any{"action": "ship", "channel": "ghost"}); c != 400 {
		t.Fatalf("unknown channel = %d", c)
	}
	if c, _ := act("/api/units/NOPE/actions", map[string]any{"action": "qc_pass"}); c != 404 {
		t.Fatalf("missing unit = %d", c)
	}
	// 批量发货：一台成功、一台（尚未质检）失败、一台重复。
	c, out := act("/api/bulk-actions", map[string]any{"serials": []string{s, serials[1], s}, "action": "ship", "channel": "jd"})
	results := out["results"].([]any)
	if c != 200 || len(results) != 2 {
		t.Fatalf("bulk %d %v", c, out)
	}
	if first := results[0].(map[string]any); first["ok"] != true || first["state"] != "in_transit" {
		t.Fatalf("bulk first: %v", first)
	}
	if second := results[1].(map[string]any); second["ok"] == true || second["error"] == "" {
		t.Fatalf("bulk second: %v", second)
	}
	if c, _ := act("/api/bulk-actions", map[string]any{"serials": []string{}, "action": "ship"}); c != 400 {
		t.Fatalf("empty bulk = %d", c)
	}

	var detail struct {
		Unit   map[string]any
		Events []map[string]any
	}
	if c := e.as("GET", "/api/units/"+s, nil, &detail); c != 200 || detail.Unit["state"] != "in_transit" || len(detail.Events) != 3 {
		t.Fatalf("detail %d %+v", c, detail)
	}
	if _, leaked := detail.Unit["public_key"]; leaked || detail.Unit["key_fingerprint"] == "" {
		t.Fatalf("unit view must show a fingerprint, not the key: %v", detail.Unit)
	}
	for k := range detail.Unit {
		if strings.Contains(k, "owner_ref") {
			t.Fatalf("owner_ref leaked: %v", detail.Unit)
		}
	}

	var list struct {
		Items []map[string]any
		Total int
	}
	if e.as("GET", "/api/units?state=in_transit&q=h1000&limit=10", nil, &list); list.Total != 1 {
		t.Fatalf("list %+v", list)
	}
	if c := e.as("GET", "/api/units?state=weird", nil, nil); c != 400 {
		t.Fatalf("bad state filter = %d", c)
	}
	if c := e.as("GET", "/api/units?locked=1", nil, &list); c != 200 || list.Total != 0 {
		t.Fatalf("locked filter %d %+v", c, list)
	}

	var st struct {
		Total   int
		ByState map[string]int `json:"by_state"`
	}
	if e.as("GET", "/api/stats", nil, &st); st.Total != 2 || st.ByState["in_transit"] != 1 || st.ByState["produced"] != 1 {
		t.Fatalf("stats %+v", st)
	}
	var bs []map[string]any
	if e.as("GET", "/api/batches", nil, &bs); len(bs) != 1 || bs[0]["units"].(float64) != 2 {
		t.Fatalf("batches %v", bs)
	}
}

func TestImportErrors(t *testing.T) {
	e := newEnv(t, plat{}, "")
	bad := map[string]any{"code": "x", "model": "m", "skin": "bear", "units": []any{}}
	if c := e.as("POST", "/api/batches", bad, nil); c != 400 {
		t.Fatalf("bad batch = %d", c)
	}
	e.importBatch("D1000", 1)
	again := map[string]any{"code": "D1000", "model": "m", "skin": "bear", "units": []map[string]string{{"mac": "aa:bb:cc:dd:ee:ee", "public_key": key(t)}}}
	if c := e.as("POST", "/api/batches", again, nil); c != 409 {
		t.Fatalf("duplicate batch = %d", c)
	}
	var msg map[string]string
	e.as("POST", "/api/batches", bad, &msg)
	if msg["error"] == "" || strings.Contains(msg["error"], "factory:") {
		t.Fatalf("error text leaks internals: %q", msg["error"])
	}
}

func TestPlatformFailureIsBadGateway(t *testing.T) {
	e := newEnv(t, plat{fail: true}, "")
	units := []map[string]string{{"mac": "aa:bb:cc:dd:ee:01", "public_key": key(t)}}
	var msg map[string]string
	if c := e.as("POST", "/api/batches", map[string]any{"code": "G1000", "model": "m", "skin": "bear", "units": units}, &msg); c != 502 {
		t.Fatalf("platform down on import = %d", c)
	}
	if strings.Contains(msg["error"], "timeout") {
		t.Fatalf("upstream error text leaked: %q", msg["error"])
	}
}

func TestStaffEndpoints(t *testing.T) {
	e := newEnv(t, plat{}, "")
	var me map[string]any
	if e.as("GET", "/api/me", nil, &me); me["is_admin"] != true || me["can_import"] != true {
		t.Fatalf("me %v", me)
	}
	if c := e.as("POST", "/api/staff", map[string]string{"username": "vera", "password": "long enough pw", "role": "viewer"}, nil); c != 201 {
		t.Fatalf("create %d", c)
	}
	if c := e.as("POST", "/api/staff", map[string]string{"username": "vera", "password": "long enough pw", "role": "viewer"}, nil); c != 409 {
		t.Fatalf("dup %d", c)
	}
	var list []map[string]any
	if e.as("GET", "/api/staff", nil, &list); len(list) != 2 {
		t.Fatalf("list %v", list)
	}
	for _, m := range list {
		if _, ok := m["password_hash"]; ok {
			t.Fatal("password hash leaked")
		}
	}
	vera := list[1]["id"].(float64)
	var login struct{ Token string }
	e.call("", "POST", "/api/login", map[string]string{"username": "vera", "password": "long enough pw"}, &login)

	// viewer 只读。
	if c := e.call(login.Token, "GET", "/api/units", nil, nil); c != 200 {
		t.Fatalf("viewer read %d", c)
	}
	if c := e.call(login.Token, "GET", "/api/staff", nil, nil); c != 403 {
		t.Fatalf("viewer staff list %d", c)
	}
	if c := e.call(login.Token, "PUT", "/api/channels/x1", map[string]string{"name": "x", "kind": "online"}, nil); c != 403 {
		t.Fatalf("viewer channel %d", c)
	}
	if c := e.call(login.Token, "POST", "/api/batches", map[string]any{"code": "ZZZ", "model": "m", "skin": "bear", "units": []any{}}, nil); c != 403 {
		t.Fatalf("viewer import %d", c)
	}
	if c := e.call(login.Token, "POST", "/api/events/1/resolve", nil, nil); c != 403 {
		t.Fatalf("viewer resolve %d", c)
	}

	path := "/api/staff/" + string(rune('0'+int(vera)))
	if c := e.as("PATCH", path, map[string]any{"role": "sales", "disabled": true}, nil); c != 204 {
		t.Fatalf("disable %d", c)
	}
	if c := e.call(login.Token, "GET", "/api/units", nil, nil); c != 401 {
		t.Fatalf("disabled staff still logged in: %d", c)
	}
	if c := e.as("PATCH", "/api/staff/abc", map[string]any{"role": "sales"}, nil); c != 400 {
		t.Fatalf("bad id %d", c)
	}
	if c := e.as("PATCH", "/api/staff/1", map[string]any{"role": "viewer"}, nil); c != 400 {
		t.Fatalf("self demote %d", c)
	}

	if c := e.as("POST", "/api/me/password", map[string]string{"old": "correct horse battery", "new": "another long password"}, nil); c != 204 {
		t.Fatalf("change password %d", c)
	}
	if c := e.as("GET", "/api/me", nil, nil); c != 401 {
		t.Fatalf("session survived password change: %d", c)
	}
}

func TestLogout(t *testing.T) {
	e := newEnv(t, plat{}, "")
	if c := e.as("POST", "/api/logout", nil, nil); c != 204 {
		t.Fatalf("logout %d", c)
	}
	if c := e.as("GET", "/api/me", nil, nil); c != 401 {
		t.Fatalf("token after logout %d", c)
	}
}

func TestPlatformEvents(t *testing.T) {
	e := newEnv(t, plat{}, "inbound-secret")
	serials := e.importBatch("E1000", 1)
	var d struct{ Unit map[string]any }
	e.as("GET", "/api/units/"+serials[0], nil, &d)
	mac := d.Unit["mac"].(string)

	ev := map[string]string{"mac": mac, "event": "activated", "owner_ref": "owner-E-00001"}
	if c := e.call("", "POST", "/internal/platform-events", ev, nil); c != 401 {
		t.Fatalf("no token %d", c)
	}
	if c := e.call(e.tok, "POST", "/internal/platform-events", ev, nil); c != 401 {
		t.Fatalf("staff token must not work for platform events: %d", c)
	}
	if c := e.call("inbound-secret", "POST", "/internal/platform-events", ev, nil); c != 204 {
		t.Fatalf("event %d", c)
	}
	if c := e.call("inbound-secret", "POST", "/internal/platform-events", map[string]string{"mac": "aa:bb:cc:dd:ee:99", "event": "unbound"}, nil); c != 404 {
		t.Fatalf("unknown mac %d", c)
	}
	if c := e.call("inbound-secret", "POST", "/internal/platform-events", map[string]string{"mac": mac, "event": "boom"}, nil); c != 400 {
		t.Fatalf("bad event %d", c)
	}
	var evs []map[string]any
	e.as("GET", "/api/events?anomaly=1", nil, &evs)
	if len(evs) != 0 {
		t.Fatalf("anomalies %v", evs)
	}
}

func TestAnomalyResolveOverHTTP(t *testing.T) {
	e := newEnv(t, plat{}, "tok-in")
	e.as("PUT", "/api/channels/jd", map[string]string{"name": "京东", "kind": "online"}, nil)
	s := e.importBatch("N1000", 1)[0]
	for _, a := range []map[string]any{{"action": "qc_pass"}, {"action": "ship", "channel": "jd"}, {"action": "receive"}} {
		if c := e.as("POST", "/api/units/"+s+"/actions", a, nil); c != 200 {
			t.Fatalf("%v: %d", a, c)
		}
	}
	var d struct{ Unit map[string]any }
	e.as("GET", "/api/units/"+s, nil, &d)
	e.call("tok-in", "POST", "/internal/platform-events", map[string]string{"mac": d.Unit["mac"].(string), "event": "activated", "owner_ref": "owner-N-00001"}, nil)

	var evs []map[string]any
	if e.as("GET", "/api/events?anomaly=1&limit=5", nil, &evs); len(evs) != 1 || evs[0]["anomaly"] != true {
		t.Fatalf("anomalies %v", evs)
	}
	id := int(evs[0]["id"].(float64))
	path := "/api/events/" + itoa(id) + "/resolve"
	if c := e.as("POST", path, nil, nil); c != 204 {
		t.Fatalf("resolve %d", c)
	}
	if c := e.as("POST", path, nil, nil); c != 404 {
		t.Fatalf("resolve twice %d", c)
	}
	if c := e.as("POST", "/api/events/x/resolve", nil, nil); c != 400 {
		t.Fatalf("bad id %d", c)
	}
	var st map[string]any
	if e.as("GET", "/api/stats", nil, &st); st["open_anomalies"].(float64) != 0 {
		t.Fatalf("stats %v", st)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestPlatformEventsDisabledWithoutToken(t *testing.T) {
	e := newEnv(t, plat{}, "")
	if c := e.call("x", "POST", "/internal/platform-events", map[string]string{}, nil); c == 204 || c == 401 {
		t.Fatalf("endpoint should not exist, got %d", c)
	}
}

func TestStaticApp(t *testing.T) {
	e := newEnv(t, plat{}, "")
	get := func(path string) (int, string) {
		resp, err := http.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		return resp.StatusCode, b.String()
	}
	if c, body := get("/"); c != 200 || !strings.Contains(body, "app") {
		t.Fatalf("/ = %d %q", c, body)
	}
	if c, body := get("/units/PP1"); c != 200 || !strings.Contains(body, "app") {
		t.Fatalf("spa fallback = %d %q", c, body)
	}
	if c, body := get("/assets/app.js"); c != 200 || body != "js" {
		t.Fatalf("asset = %d %q", c, body)
	}
	resp, _ := http.Get(e.srv.URL + "/api/me")
	if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers %v", resp.Header)
	}
	resp.Body.Close()
}
