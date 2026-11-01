package device

import (
	"context"
	"errors"
	"slices"

	"github.com/LiusCraft/palpable/server/internal/id"
)

// 小伙伴进频道的家长控制，见 docs/trust.md §5。

var ErrAgentOff = errors.New("device: guardian has not allowed the companion to speak in channels")

// SetAgentInChannel 由自家家长决定自家小伙伴能否在频道里发言。
func (d *Devices) SetAgentInChannel(ctx context.Context, dev id.DeviceID, on bool) error {
	return d.update(ctx, dev, func(p *Profile) { p.AgentOff = !on })
}

// BlockAgent 由 listener 的家长屏蔽（或解除屏蔽）owner 的小伙伴。
func (d *Devices) BlockAgent(ctx context.Context, listener, owner id.DeviceID, blocked bool) error {
	return d.update(ctx, listener, func(p *Profile) {
		p.BlockedAgents = slices.DeleteFunc(slices.Clone(p.BlockedAgents), func(x id.DeviceID) bool { return x == owner })
		if blocked {
			p.BlockedAgents = append(p.BlockedAgents, owner)
		}
	})
}

// AgentAudience 回答 owner 的小伙伴此刻能否在频道发言，以及 listeners 里哪些设备的家长屏蔽了它。
// 由 app 在小伙伴开口前算好交给 voice，voice 的锁内不查存储。查不到档案的听者按屏蔽处理。
func (d *Devices) AgentAudience(ctx context.Context, owner id.DeviceID, listeners []id.DeviceID) (map[id.DeviceID]bool, error) {
	p, err := d.Profile(ctx, owner)
	if err != nil {
		return nil, err
	}
	if p.AgentOff {
		return nil, ErrAgentOff
	}
	blocked := map[id.DeviceID]bool{}
	for _, l := range listeners {
		lp, err := d.Profile(ctx, l)
		if err != nil || slices.Contains(lp.BlockedAgents, owner) {
			blocked[l] = true
		}
	}
	return blocked, nil
}
