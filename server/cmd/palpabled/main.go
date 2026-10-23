package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LiusCraft/palpable/server/internal/gateway"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gw := gateway.New(nil, allowDevOrigin)

	mux := http.NewServeMux()
	mux.Handle("/ws", gw)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("palpabled starting", "addr", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

// allowDevOrigin 开发期只放行本机前端；上线前改为配置白名单。
func allowDevOrigin(r *http.Request) bool {
	switch r.Header.Get("Origin") {
	case "", "http://localhost:5173", "http://127.0.0.1:5173":
		return true
	}
	return false
}
