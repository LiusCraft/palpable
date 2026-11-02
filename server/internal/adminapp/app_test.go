package adminapp_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/adminapp"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	cfg, err := adminapp.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || cfg.Addr != ":8090" || cfg.Platform.Mode != "none" {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	cfg, err = adminapp.Load(write(t, "addr: ':9'\nplatform:\n  mode: http\n  base_url: http://x\n  token: t\nwarranty:\n  new_months: 24\n"))
	if err != nil || cfg.Addr != ":9" || cfg.Warranty.NewMonths != 24 {
		t.Fatalf("parsed: %+v %v", cfg, err)
	}
	if _, err := adminapp.Load(write(t, "")); err != nil {
		t.Fatalf("empty file: %v", err)
	}
	for name, body := range map[string]string{
		"unknown key":  "adress: ':1'\n",
		"http no url":  "platform:\n  mode: http\n",
		"unknown mode": "platform:\n  mode: magic\n",
		"not yaml":     "addr: [",
	} {
		if _, err := adminapp.Load(write(t, body)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := adminapp.Load(t.TempDir()); err == nil {
		t.Error("a directory is not a config file")
	}
}

func TestHandlerWiring(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>ui</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := adminapp.Config{DBPath: filepath.Join(dir, "a.db"), StaticDir: dir, InboundToken: "in"}
	cfg.Platform.Mode = "noop"
	cfg.BootstrapAdmin.Username, cfg.BootstrapAdmin.Password = "boss", "correct horse battery"

	h, closeDB, err := adminapp.Handler(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(`{"username":"boss","password":"correct horse battery"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Token string }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != 200 || out.Token == "" {
		t.Fatalf("login %d", resp.StatusCode)
	}
	if r, _ := http.Get(srv.URL + "/"); r.StatusCode != 200 {
		t.Fatalf("static %d", r.StatusCode)
	}
	if err := closeDB(); err != nil {
		t.Fatal(err)
	}

	// 重启后库里已有账号，bootstrap 不再创建；没有账号也不报错。
	cfg2 := adminapp.Config{DBPath: filepath.Join(dir, "b.db")}
	cfg2.Platform.Mode = "none"
	_, closeB, err := adminapp.Handler(ctx, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	_ = closeB()
	cfg.BootstrapAdmin.Password = ""
	_, closeC, err := adminapp.Handler(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = closeC()

	bad := adminapp.Config{DBPath: filepath.Join(dir, "c.db")}
	bad.Platform.Mode = "http"
	if _, _, err := adminapp.Handler(ctx, bad); err == nil {
		t.Error("invalid platform config must fail")
	}
	weak := adminapp.Config{DBPath: filepath.Join(dir, "d.db")}
	weak.Platform.Mode = "none"
	weak.BootstrapAdmin.Username, weak.BootstrapAdmin.Password = "boss", "short"
	if _, _, err := adminapp.Handler(ctx, weak); err == nil {
		t.Error("weak bootstrap password must fail")
	}
	unwritable := adminapp.Config{DBPath: filepath.Join(dir, "no-such-dir", "x.db")}
	unwritable.Platform.Mode = "none"
	if _, _, err := adminapp.Handler(ctx, unwritable); err == nil {
		t.Error("unopenable db must fail")
	}
}

func TestRunAndShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cfg := adminapp.Config{Addr: addr, DBPath: filepath.Join(t.TempDir(), "a.db")}
	cfg.Platform.Mode = "none"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- adminapp.Run(ctx, cfg) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	// 端口被占用时报错。
	busy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer busy.Close()
	cfg.Addr = busy.Addr().String()
	if err := adminapp.Run(context.Background(), cfg); err == nil {
		t.Error("listening on a busy port must fail")
	}
	cfg.Platform.Mode = "http"
	if err := adminapp.Run(context.Background(), cfg); err == nil {
		t.Error("bad config must fail")
	}
}
