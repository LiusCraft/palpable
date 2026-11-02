package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/LiusCraft/palpable/server/internal/adminhook"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

// 以下实现 opsapi.Ops，由厂家后台经 /internal 调用，见 docs/factory.md §5。

func (a *App) Enroll(ctx context.Context, d id.DeviceID, pubkey []byte) error {
	return a.registry.Enroll(ctx, d, pubkey)
}

// Revoke 吊销公钥并踢掉在线连接。
func (a *App) Revoke(ctx context.Context, d id.DeviceID) error {
	if err := a.registry.Revoke(ctx, d); err != nil {
		return err
	}
	a.kick(d, "device revoked")
	return nil
}

// SetLock 锁定或解锁。锁定时在线的连接立刻断开，数据保留（找回后解锁即可继续用）。
func (a *App) SetLock(ctx context.Context, d id.DeviceID, locked bool) error {
	if err := a.registry.SetLock(ctx, d, locked); err != nil {
		return err
	}
	if locked {
		a.kick(d, "device locked")
	}
	return nil
}

// Wipe 恢复出厂，不要求家长在场：用于退货翻新。设备不存在视为已擦除。
func (a *App) Wipe(ctx context.Context, d id.DeviceID) error {
	p, err := a.devices.Profile(ctx, d)
	if errors.Is(err, device.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("wipe %s: %w", d, err)
	}
	return a.reset(ctx, d, p)
}

func (a *App) kick(d id.DeviceID, why string) {
	sess, ok := a.gw.Session(d)
	if !ok {
		return
	}
	_ = sess.Send(&protocol.Error{Type: protocol.TypeError, Code: protocol.CodeUnauthorized, Message: why})
	sess.Close()
}

// ownerRef 把家长标识做成 HMAC：后台只能判断"是不是同一个人"，还原不出是谁。
func (a *App) ownerRef(g id.GuardianID) string {
	m := hmac.New(sha256.New, a.ownerKey)
	m.Write([]byte("guardian:" + string(g)))
	return hex.EncodeToString(m.Sum(nil))
}

func (a *App) notifyActivated(d id.DeviceID, g id.GuardianID) {
	if a.hook != nil {
		a.hook.Notify(adminhook.Event{MAC: string(d), Event: "activated", OwnerRef: a.ownerRef(g)})
	}
}

func (a *App) notifyUnbound(d id.DeviceID) {
	if a.hook != nil {
		a.hook.Notify(adminhook.Event{MAC: string(d), Event: "unbound"})
	}
}
