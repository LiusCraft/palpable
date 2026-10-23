// Package app 是编排层：创建各模块、组装端口，把设备消息分发到对应模块。
// 它只做"收到 X 就调用 Y"，不持有业务状态；出现业务判断说明规则放错了地方。
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/LiusCraft/palpable/server/internal/bridge/linx"
	"github.com/LiusCraft/palpable/server/internal/config"
	"github.com/LiusCraft/palpable/server/internal/gateway"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

// ai 是 app 用到的 bridge/linx 能力。voice（S5）落地后，音频与收音改由 voice 调用。
type ai interface {
	Open(ctx context.Context, dev linx.DeviceLink, p linx.Persona) error
	Close(d id.DeviceID)
	Forward(d id.DeviceID, frame []byte)
	Listen(d id.DeviceID, on bool)
}

type App struct {
	ai ai // 未配置灵矽时为 nil
	gw *gateway.Gateway
}

var _ gateway.Handler = (*App)(nil)

func New(cfg config.Config) *App {
	var bridge ai
	if cfg.Linx.Enabled() {
		bridge = linx.New(linx.Config{URL: cfg.Linx.WSURL, Token: cfg.Linx.Token})
		slog.Info("linx bridge enabled")
	} else {
		slog.Warn("linx.ws_url not set, AI conversation disabled")
	}
	return newApp(bridge)
}

func newApp(bridge ai) *App {
	a := &App{ai: bridge}
	a.gw = gateway.New(a, allowDevOrigin)
	return a
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/ws", a.gw)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// Run 组装并运行 palpabled，直到 ctx 结束。
func Run(ctx context.Context, cfg config.Config) error {
	srv := &http.Server{Addr: cfg.Addr, Handler: New(cfg).Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("palpabled starting", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (a *App) OnConnect(s *gateway.Session) {
	if a.ai == nil {
		return
	}
	if err := a.ai.Open(s.Context(), s, persona(s)); err != nil {
		slog.Warn("open ai session", "device_id", s.DeviceID(), "error", err)
		_ = s.Send(&protocol.Error{Type: protocol.TypeError, Code: protocol.CodeInternal, Message: "ai service unavailable"})
	}
}

// persona 在 device（S6）落地前没有档案可读：不指定智能体，
// 由灵矽按平台上该 MAC 已有的绑定选择。S6 起改为 device.Ensure 的结果。
func persona(*gateway.Session) linx.Persona { return linx.Persona{} }

func (a *App) OnMessage(s *gateway.Session, msg protocol.Message) {
	switch m := msg.(type) {
	case *protocol.MicState:
		if a.ai != nil {
			a.ai.Listen(s.DeviceID(), m.State == "start")
		}
	case *protocol.Wake:
		if a.ai != nil {
			a.ai.Listen(s.DeviceID(), true)
		}
	default:
		// proximity → relation（S3），location → zone（S4）。
		slog.Debug("device message not handled yet", "device_id", s.DeviceID(), "type", msg.MsgType())
	}
}

func (a *App) OnAudio(s *gateway.Session, frame []byte) {
	if a.ai != nil {
		a.ai.Forward(s.DeviceID(), frame)
	}
}

func (a *App) OnDisconnect(s *gateway.Session) {
	if a.ai != nil {
		a.ai.Close(s.DeviceID())
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
