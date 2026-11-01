package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/LiusCraft/palpable/server/internal/bridge/linx"
	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/voice"
)

// AgentSpeak 实现 mcp.Agents：主人点名后，让小伙伴以 agt:<mac> 在前台频道说下一段话。
// 两边家长的设置（自家是否允许、对方是否屏蔽）在这里查好，voice 的锁内不查存储。见 docs/trust.md §5。
func (a *App) AgentSpeak(ctx context.Context, owner id.DeviceID) error {
	ch, ok := a.chans.ForegroundOf(id.Device(owner))
	if !ok {
		return channel.ErrNotInChannel
	}
	var listeners []id.DeviceID
	for _, m := range a.chans.Members(ch) {
		if d := id.OwnerDevice(m); d != owner {
			listeners = append(listeners, d)
		}
	}
	blocked, err := a.devices.AgentAudience(ctx, owner, listeners)
	if err != nil {
		return err
	}
	if err := a.voice.ArmAgent(owner, blocked); errors.Is(err, voice.ErrNotInChannel) {
		return channel.ErrNotInChannel
	} else if err != nil {
		return fmt.Errorf("arm agent %s: %w", owner, err)
	}
	return nil
}

// agentTap 包住设备会话交给 bridge/linx：AI 下行音频照常给主人，小伙伴在频道发言时 voice 同时转给好友。
type agentTap struct {
	linx.DeviceLink
	voice *voice.Router
}

func (t agentTap) SendAudio(frame []byte) error {
	err := t.DeviceLink.SendAudio(frame)
	t.voice.OnAgentFrame(t.DeviceID(), frame)
	return err
}

func (a *App) tap(dev linx.DeviceLink) linx.DeviceLink {
	return agentTap{DeviceLink: dev, voice: a.voice}
}
