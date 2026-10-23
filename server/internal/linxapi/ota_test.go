package linxapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActivateDeviceHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Device-Id") == "02:00:00:00:00:03" {
			_, _ = io.WriteString(w, "not json")
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := New("", "t").WithOTA(srv.URL)
	if err := c.ActivateDevice(context.Background(), "02:00:00:00:00:01"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("403: %v", err)
	}
	if err := c.ActivateDevice(context.Background(), "02:00:00:00:00:03"); err == nil {
		t.Fatal("bad body must error")
	}
	if New("", "t").WithOTA("").ota != DefaultOTA {
		t.Fatal("empty OTA keeps default")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := New("", "t").WithOTA("http://127.0.0.1:1/ota").ActivateDevice(ctx, "02:00:00:00:00:01"); err == nil {
		t.Fatal("unreachable ota must error")
	}
	if err := NewFake().ActivateDevice(ctx, "x"); err != nil {
		t.Fatal(err)
	}
}
