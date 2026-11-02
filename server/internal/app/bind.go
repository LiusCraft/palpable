package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/gateway"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

// 码到期前这么久换新码，免得家长输到一半失效。
const bindRefreshEarly = 30 * time.Second

// awaitBind 让未绑定的设备播报绑定码，到期前换码，直到被绑定或掉线。见 docs/trust.md §3。
func (a *App) awaitBind(s *gateway.Session) {
	d := s.DeviceID()
	done := make(chan struct{})
	a.bindWait.Store(d, done)
	for {
		code, ttl := a.binder.Issue(d)
		_ = s.Send(&protocol.BindCode{Type: protocol.TypeBindCode, Code: code, ExpiresIn: int(ttl / time.Second)})
		_ = s.Send(&protocol.Light{Type: protocol.TypeLight, State: "idle", Color: "#ffd166", Pattern: "breathe"})
		select {
		case <-s.Context().Done():
			return
		case <-done:
			return
		case <-time.After(ttl - bindRefreshEarly):
		}
	}
}

func (a *App) bindEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.binder.Subscribe():
			a.onBound(e)
		}
	}
}

// onBound 家长输入绑定码后：提示设备，然后照常开始服务（小可欢迎 → 起名）。
func (a *App) onBound(e device.Bound) {
	a.notifyActivated(e.Device, e.Guardian)
	if ch, ok := a.bindWait.LoadAndDelete(e.Device); ok {
		close(ch.(chan struct{}))
	}
	sess, ok := a.gw.Session(e.Device)
	if !ok {
		return // 掉线了：下次上线时已是绑定状态
	}
	p, err := a.devices.Profile(sess.Context(), e.Device)
	if err != nil {
		slog.Warn("load bound device", "device_id", e.Device, "error", err)
		return
	}
	a.send(e.Device, &protocol.BindState{Type: protocol.TypeBindState, Event: "bound"},
		&protocol.Sound{Type: protocol.TypeSound, Name: "connected"})
	go a.serve(sess, p)
}

// Claim 实现 parentapi.Bindings。
func (a *App) Claim(ctx context.Context, g id.GuardianID, code string, voiceConsent bool) (id.DeviceID, error) {
	return a.binder.Claim(ctx, g, code, voiceConsent)
}

// Unbind 实现 parentapi.Bindings：家长解绑，相当于恢复出厂。
func (a *App) Unbind(ctx context.Context, g id.GuardianID, d id.DeviceID) error {
	p, err := a.devices.Profile(ctx, d)
	if err != nil {
		return err
	}
	if p.Guardian != g {
		return device.ErrNotYourDevice
	}
	if err := a.reset(ctx, d, p); err != nil {
		return err
	}
	a.notifyUnbound(d)
	return nil
}

// reset 恢复出厂：先解除好友、离开频道（让在线的好友立刻听不到），再删档案；
// 设备在线时通知它并断开，它重连后会以新设备身份播报绑定码。家长解绑与厂家后台擦除共用。
func (a *App) reset(ctx context.Context, d id.DeviceID, p device.Profile) error {
	a.active.Delete(d)
	if err := a.rel.RemoveAll(ctx, d); err != nil {
		return fmt.Errorf("reset %s: %w", d, err)
	}
	for _, ch := range a.chans.List(ctx, d) {
		if err := a.chans.Leave(ctx, d, ch.Channel.ID); err != nil {
			slog.Warn("leave channel on reset", "device_id", d, "channel", ch.Channel.ID, "error", err)
		}
	}
	if p.SpeakerID != "" {
		// 恢复出厂要删掉灵矽侧的声纹；灵矽暂时不可用时不拦着，记下说话人供人工清理。
		if err := a.devices.RevokeVoice(ctx, d); err != nil {
			slog.Error("voiceprint left in linx after reset", "device_id", d, "speaker_id", p.SpeakerID, "error", err)
		}
	}
	if _, err := a.devices.Unbind(ctx, d, p.Guardian); err != nil && !errors.Is(err, device.ErrNotFound) {
		return err
	}
	if sess, ok := a.gw.Session(d); ok {
		_ = sess.Send(&protocol.BindState{Type: protocol.TypeBindState, Event: "unbound"})
		sess.Close()
	}
	return nil
}
