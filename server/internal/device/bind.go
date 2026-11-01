package device

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
)

// 绑定码规则见 docs/trust.md §3。
const (
	BindCodeTTL = 10 * time.Minute
	// 同一家长在 bindWindow 内最多输错 bindMaxFails 次，防止猜码。
	bindWindow   = 10 * time.Minute
	bindMaxFails = 5
)

var (
	ErrBadBindCode    = errors.New("device: bind code invalid or expired")
	ErrTooManyTries   = errors.New("device: too many wrong bind codes, try later")
	ErrAlreadyBound   = errors.New("device: already bound to a guardian")
	ErrNotYourDevice  = errors.New("device: not bound to this guardian")
	errNoGuardianSlot = errors.New("device: guardian required")
)

// Bound 在设备被家长绑定时发出，app 据此让设备开始提供服务。
type Bound struct {
	Device   id.DeviceID
	Guardian id.GuardianID
}

type bindCode struct {
	dev     id.DeviceID
	expires time.Time
}

// Binder 管理在线未绑定设备的短码。码只在内存里：设备掉线即作废，重连换新码。
type Binder struct {
	devices *Devices
	now     func() time.Time
	events  chan Bound

	mu     sync.Mutex
	codes  map[string]bindCode
	byDev  map[id.DeviceID]string
	misses map[id.GuardianID][]time.Time
}

func NewBinder(devices *Devices) *Binder {
	return &Binder{devices: devices, now: time.Now, events: make(chan Bound, eventBuffer),
		codes: map[string]bindCode{}, byDev: map[id.DeviceID]string{}, misses: map[id.GuardianID][]time.Time{}}
}

func (b *Binder) Subscribe() <-chan Bound { return b.events }

// Issue 给设备发一个新码，旧码作废。
func (b *Binder) Issue(d id.DeviceID) (string, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revokeLocked(d)
	now := b.now()
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			panic(fmt.Sprintf("crypto/rand: %v", err)) // 系统随机源不可用时无法安全继续
		}
		code := fmt.Sprintf("%06d", n.Int64())
		if c, taken := b.codes[code]; taken && now.Before(c.expires) {
			continue
		}
		b.codes[code] = bindCode{dev: d, expires: now.Add(BindCodeTTL)}
		b.byDev[d] = code
		return code, BindCodeTTL
	}
}

// Revoke 在设备掉线或已绑定时作废它的码。
func (b *Binder) Revoke(d id.DeviceID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revokeLocked(d)
}

func (b *Binder) revokeLocked(d id.DeviceID) {
	if code, ok := b.byDev[d]; ok {
		delete(b.codes, code)
		delete(b.byDev, d)
	}
}

// Claim 把码对应的设备绑定到家长，并记下声纹登记的单独同意。码用一次即作废。
func (b *Binder) Claim(ctx context.Context, g id.GuardianID, code string, voiceConsent bool) (id.DeviceID, error) {
	if g == "" {
		return "", errNoGuardianSlot
	}
	b.mu.Lock()
	now := b.now()
	recent := b.misses[g][:0]
	for _, t := range b.misses[g] {
		if now.Sub(t) < bindWindow {
			recent = append(recent, t)
		}
	}
	b.misses[g] = recent
	if len(recent) >= bindMaxFails {
		b.mu.Unlock()
		return "", ErrTooManyTries
	}
	c, ok := b.codes[code]
	if !ok || !now.Before(c.expires) {
		b.misses[g] = append(b.misses[g], now)
		b.mu.Unlock()
		return "", ErrBadBindCode
	}
	b.revokeLocked(c.dev)
	b.mu.Unlock()

	if err := b.devices.Claim(ctx, c.dev, g, voiceConsent); err != nil {
		return "", err
	}
	select {
	case b.events <- Bound{Device: c.dev, Guardian: g}:
	default:
	}
	return c.dev, nil
}

// Claim 只给未绑定的设备记上家长；已绑定的设备必须先由原家长解绑（类似激活锁）。
func (d *Devices) Claim(ctx context.Context, dev id.DeviceID, g id.GuardianID, voiceConsent bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, err := d.Profile(ctx, dev)
	if err != nil {
		return err
	}
	if p.Guardian != "" {
		return ErrAlreadyBound
	}
	p.Guardian, p.VoiceConsent = g, voiceConsent
	return d.repo.Update(ctx, toRecord(p))
}

// Unbind 由原家长解绑，相当于恢复出厂：删除档案及其好友、群成员与历史。
// 灵矽侧的声纹与对话由调用方另行清理。
func (d *Devices) Unbind(ctx context.Context, dev id.DeviceID, g id.GuardianID) (Profile, error) {
	p, err := d.Profile(ctx, dev)
	if err != nil {
		return Profile{}, err
	}
	if p.Guardian != g {
		return Profile{}, ErrNotYourDevice
	}
	if err := d.repo.Delete(ctx, dev); err != nil {
		return Profile{}, fmt.Errorf("unbind %s: %w", dev, err)
	}
	return p, nil
}
