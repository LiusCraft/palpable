package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/LiusCraft/palpable/server/internal/account"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/parentapi"
)

// parentAuth 组合家长端鉴权：微信登录的会话令牌，以及（配置了时）演示固定令牌。都没有时不开放 /api。
func (a *App) parentAuth() parentapi.Auth {
	var demo parentapi.Auth
	if a.demo.ParentToken != "" && a.demo.Guardian != "" {
		demo = parentapi.FixedToken(a.demo.ParentToken, id.GuardianID(a.demo.Guardian))
	}
	switch {
	case a.accounts == nil:
		return demo
	case demo == nil:
		return a.accounts.Verify
	}
	return func(ctx context.Context, token string) (id.GuardianID, bool) {
		if g, ok := a.accounts.Verify(ctx, token); ok {
			return g, true
		}
		return demo(ctx, token)
	}
}

// accountAPI 实现 parentapi.Accounts：注销要先把名下设备逐台解绑（清好友、离频道、恢复出厂），再删账号。
type accountAPI struct {
	*account.Accounts
	app *App
}

func (x accountAPI) DeleteAccount(ctx context.Context, g id.GuardianID) error {
	if _, err := x.Profile(ctx, g); err != nil {
		return err // 演示家长没有账号，不能注销
	}
	kids, err := x.app.devices.ByGuardian(ctx, g)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	for _, k := range kids {
		if err := x.app.Unbind(ctx, g, k.ID); err != nil && !errors.Is(err, device.ErrNotFound) {
			return fmt.Errorf("delete account: unbind %s: %w", k.ID, err)
		}
	}
	return x.Delete(ctx, g)
}

func (a *App) sweepSessions(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := a.accounts.Sweep(ctx); err != nil {
				slog.Warn("sweep sessions", "error", err)
			}
		}
	}
}
