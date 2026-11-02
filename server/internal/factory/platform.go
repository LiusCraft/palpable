package factory

import "context"

// Platform 是后台对 palpabled 的窄接口（docs/factory.md §5）。所有方法必须幂等：
// 后台先调用它、再落库，落库失败时会被重试。
type Platform interface {
	// Enroll 把设备公钥写入 palpabled 的出厂清单；同一 MAC 同一公钥重复调用返回 nil，公钥不同返回错误。
	Enroll(ctx context.Context, mac string, pubkey []byte) error
	// Revoke 吊销公钥，设备此后连不上。
	Revoke(ctx context.Context, mac string) error
	// Wipe 恢复出厂：删除档案、好友、历史、声纹与绑定。设备不存在时返回 nil。
	Wipe(ctx context.Context, mac string) error
	// SetLock 锁定或解锁：锁定的设备所有连接被拒。
	SetLock(ctx context.Context, mac string, locked bool) error
}

// NoopPlatform 什么都不做就报成功，只给开发环境（platform.mode: noop）用。
// 生产环境若误用，擦除与挂失会"假成功"，所以必须在配置里显式选择。
type NoopPlatform struct{}

func (NoopPlatform) Enroll(context.Context, string, []byte) error { return nil }
func (NoopPlatform) Revoke(context.Context, string) error         { return nil }
func (NoopPlatform) Wipe(context.Context, string) error           { return nil }
func (NoopPlatform) SetLock(context.Context, string, bool) error  { return nil }

// PlatformEvent 是 palpabled 推给后台的事件。OwnerRef 已经是 HMAC 后的值。
type PlatformEvent struct {
	MAC      string
	Kind     string // activated | unbound
	OwnerRef string
}
