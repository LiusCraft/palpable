// Package adminapp 读取厂家后台的配置并把各模块组装起来。cmd/palpable-admin 只调用它。
package adminapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/LiusCraft/palpable/server/internal/adminapi"
	"github.com/LiusCraft/palpable/server/internal/factory"
	"github.com/LiusCraft/palpable/server/internal/factorydb"
	"github.com/LiusCraft/palpable/server/internal/palpclient"
)

type Config struct {
	Addr      string `yaml:"addr"`
	DBPath    string `yaml:"db_path"`
	StaticDir string `yaml:"static_dir"` // 管理页面的构建产物（admin/dist）；空则只提供 API

	Platform Platform `yaml:"platform"`
	// InboundToken 是 palpabled 推送绑定事件时带的令牌；空则不开放该入口。
	InboundToken string `yaml:"inbound_token"`

	BootstrapAdmin struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"bootstrap_admin"`

	Warranty struct {
		NewMonths    int `yaml:"new_months"`
		RefurbMonths int `yaml:"refurb_months"`
	} `yaml:"warranty"`
}

// Platform 决定怎么对接 palpabled。
//   - http：调用 palpabled 的 /internal 接口（生产）
//   - noop：什么都不做就报成功（只给本地开发；擦除与挂失会"假成功"）
//   - none：不对接（默认）；需要擦除、吊销、挂失、登记公钥的动作会被明确拒绝
type Platform struct {
	Mode    string `yaml:"mode"`
	BaseURL string `yaml:"base_url"`
	Token   string `yaml:"token"`
}

// Load 读取 path；文件不存在返回默认配置，未知字段报错。
func Load(path string) (Config, error) {
	cfg := Config{Addr: ":8090", DBPath: "palpable-admin.db"}
	cfg.Platform.Mode = "none"
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch c.Platform.Mode {
	case "none", "noop":
	case "http":
		if c.Platform.BaseURL == "" || c.Platform.Token == "" {
			return errors.New("platform.mode http needs platform.base_url and platform.token")
		}
	default:
		return fmt.Errorf("unknown platform.mode %q (want http, noop or none)", c.Platform.Mode)
	}
	return nil
}

func (c Config) platform() factory.Platform {
	switch c.Platform.Mode {
	case "http":
		return palpclient.New(c.Platform.BaseURL, c.Platform.Token, nil)
	case "noop":
		slog.Warn("platform.mode is noop: wipes, locks and key revocations are NOT sent to palpabled. Development only.")
		return factory.NoopPlatform{}
	}
	slog.Warn("platform.mode is none: actions that need palpabled (QC pass, return, scrap, lock, batch import) will be refused")
	return nil
}

// Handler 组装全部模块并返回 HTTP 处理器和清理函数。
func Handler(ctx context.Context, cfg Config) (http.Handler, func() error, error) {
	if err := cfg.validate(); err != nil {
		return nil, nil, err
	}
	db, err := factorydb.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	svc := factory.NewService(db, cfg.platform(), factory.Options{
		NewWarrantyMonths: cfg.Warranty.NewMonths, RefurbWarrantyMonths: cfg.Warranty.RefurbMonths})
	auth := factory.NewAuth(db, nil)
	made, err := auth.Bootstrap(ctx, cfg.BootstrapAdmin.Username, cfg.BootstrapAdmin.Password)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	switch n, _ := db.CountStaff(ctx); {
	case made:
		slog.Warn("created the first admin from bootstrap_admin; change the password after logging in", "username", cfg.BootstrapAdmin.Username)
	case n == 0:
		slog.Warn("no staff accounts exist: set bootstrap_admin.username and password in the config and restart")
	}
	var static fs.FS
	if cfg.StaticDir != "" {
		static = os.DirFS(cfg.StaticDir)
	}
	return adminapi.New(svc, auth, cfg.InboundToken, static), db.Close, nil
}

// Run 监听并服务，直到 ctx 结束。
func Run(ctx context.Context, cfg Config) error {
	h, closeDB, err := Handler(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := closeDB(); err != nil {
			slog.Error("close db", "error", err)
		}
	}()
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Addr, err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	slog.Info("palpable-admin listening", "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
