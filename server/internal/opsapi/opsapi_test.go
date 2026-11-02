package opsapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/opsapi"
)

type fake struct {
	calls []string
	err   error
}

func (f *fake) rec(s string, d id.DeviceID) error {
	f.calls = append(f.calls, s+" "+string(d))
	return f.err
}
func (f *fake) Enroll(_ context.Context, d id.DeviceID, k []byte) error {
	return f.rec("enroll:"+string(k), d)
}
func (f *fake) Revoke(_ context.Context, d id.DeviceID) error { return f.rec("revoke", d) }
func (f *fake) Wipe(_ context.Context, d id.DeviceID) error   { return f.rec("wipe", d) }
func (f *fake) SetLock(_ context.Context, d id.DeviceID, l bool) error {
	if l {
		return f.rec("lock", d)
	}
	return f.rec("unlock", d)
}

func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRoutes(t *testing.T) {
	f := &fake{}
	h := opsapi.New("secret", f)
	const mac = "AA:BB:CC:DD:EE:01"
	key := base64.StdEncoding.EncodeToString([]byte("k"))
	for _, c := range []struct {
		method, path, body string
		want               string
	}{
		{"PUT", "/internal/devices/" + mac + "/key", `{"public_key":"` + key + `"}`, "enroll:k aa:bb:cc:dd:ee:01"},
		{"DELETE", "/internal/devices/" + mac + "/key", "", "revoke aa:bb:cc:dd:ee:01"},
		{"POST", "/internal/devices/" + mac + "/wipe", "", "wipe aa:bb:cc:dd:ee:01"},
		{"PUT", "/internal/devices/" + mac + "/lock", `{"locked":true}`, "lock aa:bb:cc:dd:ee:01"},
		{"PUT", "/internal/devices/" + mac + "/lock", `{"locked":false}`, "unlock aa:bb:cc:dd:ee:01"},
	} {
		rec := do(t, h, c.method, c.path, "secret", c.body)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s %s = %d %s", c.method, c.path, rec.Code, rec.Body)
		}
		if got := f.calls[len(f.calls)-1]; got != c.want {
			t.Fatalf("call = %q, want %q", got, c.want)
		}
	}
	if do(t, h, "PUT", "/internal/devices/"+mac+"/lock", "secret", `{"locked":true}`).Header().Get("Cache-Control") != "no-store" {
		t.Fatal("responses must not be cached")
	}
}

func TestAuthAndValidation(t *testing.T) {
	f := &fake{}
	h := opsapi.New("secret", f)
	const p = "/internal/devices/aa:bb:cc:dd:ee:01/wipe"
	for name, tok := range map[string]string{"none": "", "wrong": "nope"} {
		if rec := do(t, h, "POST", p, tok, ""); rec.Code != 401 {
			t.Errorf("%s token = %d", name, rec.Code)
		}
	}
	if rec := do(t, opsapi.New("", f), "POST", p, "", ""); rec.Code != 401 {
		t.Errorf("empty configured token must never authenticate, got %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/internal/devices/not-a-mac/wipe", "secret", ""); rec.Code != 400 {
		t.Errorf("bad mac = %d", rec.Code)
	}
	for name, body := range map[string]string{
		"not json":   "x",
		"not base64": `{"public_key":"!!"}`,
		"extra":      `{"public_key":"AA==","x":1}`,
	} {
		if rec := do(t, h, "PUT", "/internal/devices/aa:bb:cc:dd:ee:01/key", "secret", body); rec.Code != 400 {
			t.Errorf("enroll %s = %d", name, rec.Code)
		}
	}
	for name, body := range map[string]string{"empty": `{}`, "not json": "x", "string": `{"locked":"yes"}`} {
		if rec := do(t, h, "PUT", "/internal/devices/aa:bb:cc:dd:ee:01/lock", "secret", body); rec.Code != 400 {
			t.Errorf("lock %s = %d", name, rec.Code)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("rejected requests reached the implementation: %v", f.calls)
	}
}

func TestErrorMapping(t *testing.T) {
	for _, c := range []struct {
		err  error
		want int
	}{
		{device.ErrBadPublicKey, 400},
		{device.ErrKeyMismatch, 409},
		{device.ErrRevoked, 409},
		{errors.New("db down"), 500},
	} {
		h := opsapi.New("s", &fake{err: c.err})
		for _, r := range [][3]string{
			{"PUT", "/key", `{"public_key":"AA=="}`},
			{"DELETE", "/key", ""},
			{"POST", "/wipe", ""},
			{"PUT", "/lock", `{"locked":true}`},
		} {
			rec := do(t, h, r[0], "/internal/devices/aa:bb:cc:dd:ee:01"+r[1], "s", r[2])
			if rec.Code != c.want {
				t.Errorf("%v on %s = %d, want %d", c.err, r[1], rec.Code, c.want)
			}
			if c.want == 500 && bytes.Contains(rec.Body.Bytes(), []byte("db down")) {
				t.Errorf("internal error text leaked: %s", rec.Body)
			}
		}
	}
}
