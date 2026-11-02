package palpclient_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/palpclient"
)

type call struct{ method, path, auth, body string }

func server(t *testing.T, status int) (*palpclient.Client, *[]call) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, call{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(b)})
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return palpclient.New(srv.URL+"/", "tok", nil), &calls
}

func TestRequests(t *testing.T) {
	c, calls := server(t, http.StatusNoContent)
	ctx := context.Background()
	if err := c.Enroll(ctx, "aa:bb:cc:dd:ee:ff", []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := c.Revoke(ctx, "aa:bb:cc:dd:ee:ff"); err != nil {
		t.Fatal(err)
	}
	if err := c.Wipe(ctx, "aa:bb:cc:dd:ee:ff"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetLock(ctx, "aa:bb:cc:dd:ee:ff", true); err != nil {
		t.Fatal(err)
	}
	want := []struct{ method, path string }{
		{"PUT", "/internal/devices/aa:bb:cc:dd:ee:ff/key"},
		{"DELETE", "/internal/devices/aa:bb:cc:dd:ee:ff/key"},
		{"POST", "/internal/devices/aa:bb:cc:dd:ee:ff/wipe"},
		{"PUT", "/internal/devices/aa:bb:cc:dd:ee:ff/lock"},
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.method != w.method || got.path != w.path || got.auth != "Bearer tok" {
			t.Errorf("call %d = %+v, want %+v", i, got, w)
		}
	}
	var enroll struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal([]byte((*calls)[0].body), &enroll); err != nil || enroll.PublicKey != base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) {
		t.Errorf("enroll body = %q", (*calls)[0].body)
	}
	if !strings.Contains((*calls)[3].body, `"locked":true`) {
		t.Errorf("lock body = %q", (*calls)[3].body)
	}
}

func TestNotFoundIsSuccessForRevokeAndWipeOnly(t *testing.T) {
	c, _ := server(t, http.StatusNotFound)
	ctx := context.Background()
	if err := c.Revoke(ctx, "m"); err != nil {
		t.Errorf("revoke 404: %v", err)
	}
	if err := c.Wipe(ctx, "m"); err != nil {
		t.Errorf("wipe 404: %v", err)
	}
	if err := c.SetLock(ctx, "m", true); err == nil {
		t.Error("lock 404 must fail: the device is not known to palpabled")
	}
	if err := c.Enroll(ctx, "m", []byte{1}); err == nil {
		t.Error("enroll 404 must fail")
	}
}

func TestErrors(t *testing.T) {
	c, _ := server(t, http.StatusConflict)
	if err := c.Enroll(context.Background(), "m", []byte{1}); err == nil || !strings.Contains(err.Error(), "409") {
		t.Errorf("conflict: %v", err)
	}
	down := palpclient.New("http://127.0.0.1:1", "tok", nil)
	if err := down.Wipe(context.Background(), "m"); err == nil {
		t.Error("unreachable server must fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Wipe(ctx, "m"); err == nil {
		t.Error("canceled context must fail")
	}
	if err := c.Wipe(context.Background(), "bad\x7fmac"); err == nil {
		t.Error("invalid url must fail")
	}
}
