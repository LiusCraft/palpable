// palpable-admin 是厂家后台：设备从出厂到报废的台账。设计见 docs/factory.md。
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/LiusCraft/palpable/server/internal/adminapp"
)

func main() {
	configPath := flag.String("config", "admin.config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := adminapp.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := adminapp.Run(ctx, cfg); err != nil {
		slog.Error("admin exited", "error", err)
		os.Exit(1)
	}
}
