package device

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
)

// GrantVoice 是家长在绑定后补开声音登记。登记只在引导期进行（见 EnrollVoice），
// 已经起好名字的设备补开也不会登记，所以直接拒绝，免得家长以为开了。
func (d *Devices) GrantVoice(ctx context.Context, dev id.DeviceID) error {
	p, err := d.Profile(ctx, dev)
	if err != nil {
		return err
	}
	if p.Stage != StageFresh {
		return ErrNotOnboarding
	}
	return d.SetVoiceConsent(ctx, dev, true)
}

// RevokeVoice 撤回声纹同意并删除灵矽侧的声纹与说话人（docs/trust.md §1"关闭即删除"）。
// 灵矽删除失败时档案不变、返回错误，由调用方决定重试还是记下人工清理。
func (d *Devices) RevokeVoice(ctx context.Context, dev id.DeviceID) error {
	p, err := d.Profile(ctx, dev)
	if err != nil {
		return err
	}
	if p.SpeakerID != "" {
		if err := d.forgetSpeaker(ctx, p.AgentID, p.SpeakerID); err != nil {
			return fmt.Errorf("revoke voice %s: %w", dev, err)
		}
	}
	return d.update(ctx, dev, func(p *Profile) {
		p.VoiceConsent = false
		p.SpeakerID = ""
	})
}

// forgetSpeaker 按声纹 V2 的要求先从智能体解绑（仍被绑定的说话人不能删），再删声纹与说话人。
// 说话人已经不存在时视为删除成功。
func (d *Devices) forgetSpeaker(ctx context.Context, agentID, speaker string) error {
	if agentID != "" {
		bound, err := d.linx.BoundSpeakers(ctx, agentID)
		if err != nil && !errors.Is(err, linxapi.ErrNotFound) {
			return err
		}
		if slices.Contains(bound, speaker) {
			rest := slices.DeleteFunc(slices.Clone(bound), func(s string) bool { return s == speaker })
			if err := d.linx.BindSpeakers(ctx, agentID, rest); err != nil {
				return err
			}
		}
	}
	if err := d.linx.DeleteVoiceprint(ctx, speaker); err != nil && !errors.Is(err, linxapi.ErrNotFound) {
		return err
	}
	if err := d.linx.DeleteSpeaker(ctx, speaker); err != nil && !errors.Is(err, linxapi.ErrNotFound) {
		return err
	}
	return nil
}
