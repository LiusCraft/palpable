package parentapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

type fakeBindings struct {
	claimErr  error
	unbindErr error
	consent   bool
	unbound   id.DeviceID
}

func (f *fakeBindings) Claim(_ context.Context, g id.GuardianID, code string, voice bool) (id.DeviceID, error) {
	f.consent = voice
	return kidA, f.claimErr
}
func (f *fakeBindings) Unbind(_ context.Context, _ id.GuardianID, d id.DeviceID) error {
	f.unbound = d
	return f.unbindErr
}

func bindServer(t *testing.T, fb *fakeBindings) *httptest.Server {
	t.Helper()
	s := store.NewMemory()
	if err := s.Devices().Create(context.Background(), store.Device{ID: kidA, Guardian: "g1"}); err != nil {
		t.Fatal(err)
	}
	fake := linxapi.NewFake()
	api := New(FixedToken(token, "g1"), device.New(s.Devices(), device.NewCatalog(s.Styles(), fake), fake), nil, nil, nil, nil)
	api.EnableBinding(fb)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path, body string) int {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestBindEndpoint(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"ok", `{"code":"123456","ai_companion":true,"voice_consent":true}`, nil, http.StatusCreated},
		{"no consent", `{"code":"123456"}`, nil, http.StatusBadRequest},
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"unknown field", `{"code":"123456","ai_companion":true,"x":1}`, nil, http.StatusBadRequest},
		{"wrong code", `{"code":"123456","ai_companion":true}`, device.ErrBadBindCode, http.StatusBadRequest},
		{"rate limited", `{"code":"123456","ai_companion":true}`, device.ErrTooManyTries, http.StatusTooManyRequests},
		{"already bound", `{"code":"123456","ai_companion":true}`, device.ErrAlreadyBound, http.StatusConflict},
		{"internal", `{"code":"123456","ai_companion":true}`, errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fb := &fakeBindings{claimErr: c.err}
			if got := call(t, bindServer(t, fb), "POST", "/api/bindings", c.body); got != c.want {
				t.Fatalf("status %d, want %d", got, c.want)
			}
			if c.name == "ok" && !fb.consent {
				t.Fatal("voice consent not passed through")
			}
		})
	}
}

func TestUnbindEndpoint(t *testing.T) {
	fb := &fakeBindings{}
	srv := bindServer(t, fb)
	if got := call(t, srv, "DELETE", "/api/kids/02:00:00:00:00:99", ""); got != http.StatusNotFound {
		t.Fatalf("unknown kid: %d", got)
	}
	if got := call(t, srv, "DELETE", "/api/kids/"+string(kidA), ""); got != http.StatusNoContent || fb.unbound != kidA {
		t.Fatalf("unbind: %d %s", got, fb.unbound)
	}
	fb.unbindErr = errors.New("boom")
	if got := call(t, srv, "DELETE", "/api/kids/"+string(kidA), ""); got != http.StatusInternalServerError {
		t.Fatalf("internal: %d", got)
	}
	fb.unbindErr = device.ErrNotYourDevice
	if got := call(t, srv, "DELETE", "/api/kids/"+string(kidA), ""); got != http.StatusNotFound {
		t.Fatalf("not yours: %d", got)
	}
}
