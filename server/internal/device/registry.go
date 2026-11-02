package device

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

var (
	ErrRevoked      = errors.New("device: revoked")
	ErrDeviceLocked = errors.New("device: locked")
)

// Registry 管出厂清单里的公钥与设备封锁，由厂家后台经 /internal 接口驱动，规则见 docs/factory.md §5。
// 所有方法幂等：后台先调用、再落库，落库失败时会重试。
type Registry struct {
	keys   store.DeviceKeyRepo
	blocks store.DeviceBlockRepo
	now    func() time.Time
}

func NewRegistry(keys store.DeviceKeyRepo, blocks store.DeviceBlockRepo) *Registry {
	return &Registry{keys: keys, blocks: blocks, now: time.Now}
}

// Enroll 登记公钥。同一设备同一公钥重复登记返回 nil；公钥不同返回 ErrKeyMismatch（换钥只能先吊销再另换设备，
// 不能直接覆盖）；已吊销的设备返回 ErrRevoked。
func (r *Registry) Enroll(ctx context.Context, d id.DeviceID, pubkey []byte) error {
	if _, err := parseKey(pubkey); err != nil {
		return err
	}
	switch b, err := r.blocks.Get(ctx, d); {
	case err == nil && b.Reason == store.BlockRevoked:
		return ErrRevoked
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("enroll %s: %w", d, err)
	}
	err := r.keys.Create(ctx, store.DeviceKey{Device: d, PublicKey: pubkey, CreatedAt: r.now()})
	if !errors.Is(err, store.ErrConflict) {
		if err != nil {
			return fmt.Errorf("enroll %s: %w", d, err)
		}
		return nil
	}
	cur, err := r.keys.Get(ctx, d)
	if err != nil {
		return fmt.Errorf("enroll %s: %w", d, err)
	}
	if !bytes.Equal(cur.PublicKey, pubkey) {
		return ErrKeyMismatch
	}
	return nil
}

// Revoke 吊销：删公钥并永久封锁。封锁让 open 模式下的"首次上线即登记"也无法让它回来。
func (r *Registry) Revoke(ctx context.Context, d id.DeviceID) error {
	if err := r.blocks.Put(ctx, store.DeviceBlock{Device: d, Reason: store.BlockRevoked, CreatedAt: r.now()}); err != nil {
		return fmt.Errorf("revoke %s: %w", d, err)
	}
	if err := r.keys.Delete(ctx, d); err != nil {
		return fmt.Errorf("revoke %s: %w", d, err)
	}
	return nil
}

// SetLock 锁定或解锁。已吊销的设备保持吊销：锁定不降级它，解锁返回 ErrRevoked。
func (r *Registry) SetLock(ctx context.Context, d id.DeviceID, locked bool) error {
	cur, err := r.blocks.Get(ctx, d)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("lock %s: %w", d, err)
	}
	revoked := err == nil && cur.Reason == store.BlockRevoked
	switch {
	case revoked && !locked:
		return ErrRevoked
	case revoked:
		return nil
	case locked:
		return r.blocks.Put(ctx, store.DeviceBlock{Device: d, Reason: store.BlockLocked, CreatedAt: r.now()})
	}
	return r.blocks.Delete(ctx, d)
}

// Admit 供网关在握手后、认证前调用：被封锁的设备不允许连接。
func (r *Registry) Admit(ctx context.Context, d id.DeviceID) error {
	b, err := r.blocks.Get(ctx, d)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("admit %s: %w", d, err)
	case b.Reason == store.BlockRevoked:
		return ErrRevoked
	}
	return ErrDeviceLocked
}
