package device

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

// Enrollment 决定未知设备的公钥怎么登记，见 docs/trust.md §2.3。
type Enrollment string

const (
	// EnrollOpen 首次上线即登记（TOFU），用于演示与开发。
	EnrollOpen Enrollment = "open"
	// EnrollManifest 只接受出厂清单里登记过的设备。
	EnrollManifest Enrollment = "manifest"
	// EnrollOff 不认证；只为固件实现认证前过渡。
	EnrollOff Enrollment = "off"
)

// authContext 写进签名内容，防止签名被挪作他用。
const authContext = "palpable-auth-v1"

var (
	ErrUnknownDevice = errors.New("device: not enrolled")
	ErrNoPublicKey   = errors.New("device: public key required for first enrollment")
	ErrKeyMismatch   = errors.New("device: public key differs from the enrolled one")
	ErrBadPublicKey  = errors.New("device: public key must be ECDSA P-256 SPKI")
	ErrBadSignature  = errors.New("device: signature does not verify")
)

// Authenticator 校验设备持有登记过的私钥。服务端只存公钥，库泄露也无法冒充设备。
type Authenticator struct {
	repo store.DeviceKeyRepo
	mode Enrollment
	now  func() time.Time
}

func NewAuthenticator(repo store.DeviceKeyRepo, mode Enrollment) (*Authenticator, error) {
	switch mode {
	case EnrollOpen, EnrollManifest, EnrollOff:
	case "":
		mode = EnrollOpen
	default:
		return nil, fmt.Errorf("unknown enrollment mode %q", mode)
	}
	return &Authenticator{repo: repo, mode: mode, now: time.Now}, nil
}

// Required 为 false 时网关跳过挑战。
func (a *Authenticator) Required() bool { return a.mode != EnrollOff }

// SignedMessage 是设备要签名的字节串。
func SignedMessage(d id.DeviceID, nonce string) []byte {
	return []byte(authContext + "\n" + string(d) + "\n" + nonce)
}

// Verify 用登记过的公钥（或 open 模式下首次带来的公钥）验证 sig 是对 nonce 的签名；
// 首次验证通过后才登记公钥，拿不出私钥的一方登记不了。
func (a *Authenticator) Verify(ctx context.Context, d id.DeviceID, pubkey []byte, nonce string, sig []byte) error {
	rec, err := a.repo.Get(ctx, d)
	known := err == nil
	switch {
	case known:
		if pubkey != nil && !bytes.Equal(pubkey, rec.PublicKey) {
			return ErrKeyMismatch
		}
		pubkey = rec.PublicKey
	case !errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("verify %s: %w", d, err)
	case a.mode == EnrollManifest:
		return ErrUnknownDevice
	case pubkey == nil:
		return ErrNoPublicKey
	}
	key, err := parseKey(pubkey)
	if err != nil {
		return err
	}
	if !verify(key, SignedMessage(d, nonce), sig) {
		return ErrBadSignature
	}
	if known {
		return nil
	}
	err = a.repo.Create(ctx, store.DeviceKey{Device: d, PublicKey: pubkey, CreatedAt: a.now()})
	if errors.Is(err, store.ErrConflict) {
		// 同一 MAC 并发首次登记：以先写入的为准，再核对一次。
		return a.Verify(ctx, d, pubkey, nonce, sig)
	}
	if err != nil {
		return fmt.Errorf("enroll key %s: %w", d, err)
	}
	return nil
}

// Enroll 由运维导入出厂清单时调用；已登记的设备不能经此换钥。
func (a *Authenticator) Enroll(ctx context.Context, d id.DeviceID, pubkey []byte) error {
	if _, err := parseKey(pubkey); err != nil {
		return err
	}
	return a.repo.Create(ctx, store.DeviceKey{Device: d, PublicKey: pubkey, CreatedAt: a.now()})
}

func parseKey(der []byte) (*ecdsa.PublicKey, error) {
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, ErrBadPublicKey
	}
	ek, ok := k.(*ecdsa.PublicKey)
	if !ok || ek.Curve != elliptic.P256() {
		return nil, ErrBadPublicKey
	}
	return ek, nil
}

// verify 接受 WebCrypto 与 mbedTLS 都能给出的 r‖s 定长格式。
func verify(k *ecdsa.PublicKey, msg, sig []byte) bool {
	if len(sig) != 64 {
		return false
	}
	h := sha256.Sum256(msg)
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(k, h[:], r, s)
}
