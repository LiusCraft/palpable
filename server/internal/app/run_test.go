package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/config"
)

// Run 组装真实依赖（SQLite、不接灵矽），ctx 结束后优雅退出。
func TestRunStartsAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.Config{Addr: "127.0.0.1:0", DBPath: filepath.Join(t.TempDir(), "palpable.db")}
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestRunFailsOnBadDBPath(t *testing.T) {
	cfg := config.Config{Addr: "127.0.0.1:0", DBPath: filepath.Join(t.TempDir(), "missing", "dir", "p.db")}
	if err := Run(context.Background(), cfg); err == nil {
		t.Fatal("expected error for unusable db path")
	}
}

func TestNewWithLinxConfigured(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "p.db"), Linx: config.Linx{WSURL: "ws://127.0.0.1:1", Token: "t"}}
	a, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.ai == nil {
		t.Fatal("linx bridge not assembled")
	}
}

func TestAllowDevOrigin(t *testing.T) {
	for origin, want := range map[string]bool{"": true, "http://localhost:5173": true, "http://127.0.0.1:5173": true, "https://evil.example": false} {
		r := httptest.NewRequest(http.MethodGet, "/ws", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if got := allowDevOrigin(r); got != want {
			t.Errorf("allowDevOrigin(%q) = %v", origin, got)
		}
	}
}
