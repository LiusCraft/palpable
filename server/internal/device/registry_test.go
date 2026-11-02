package device

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/store"
)

func spki(t *testing.T) []byte {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestRegistryEnroll(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	r := NewRegistry(st.DeviceKeys(), st.DeviceBlocks())
	key := spki(t)
	if err := r.Enroll(ctx, "aa:bb:cc:dd:ee:01", key); err != nil {
		t.Fatal(err)
	}
	if err := r.Enroll(ctx, "aa:bb:cc:dd:ee:01", key); err != nil {
		t.Fatalf("same key twice must be idempotent: %v", err)
	}
	if err := r.Enroll(ctx, "aa:bb:cc:dd:ee:01", spki(t)); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("different key: %v", err)
	}
	if err := r.Enroll(ctx, "aa:bb:cc:dd:ee:02", []byte("junk")); !errors.Is(err, ErrBadPublicKey) {
		t.Fatalf("junk key: %v", err)
	}
	// 清单里登记的设备能通过 manifest 模式的认证查找。
	if k, err := st.DeviceKeys().Get(ctx, "aa:bb:cc:dd:ee:01"); err != nil || string(k.PublicKey) != string(key) {
		t.Fatalf("stored key: %v", err)
	}
}

func TestRegistryLockAndRevoke(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	r := NewRegistry(st.DeviceKeys(), st.DeviceBlocks())
	const d = "aa:bb:cc:dd:ee:03"
	if err := r.Admit(ctx, d); err != nil {
		t.Fatalf("fresh device: %v", err)
	}
	if err := r.SetLock(ctx, d, false); err != nil {
		t.Fatalf("unlock of an unlocked device: %v", err)
	}
	if err := r.SetLock(ctx, d, true); err != nil {
		t.Fatal(err)
	}
	if err := r.SetLock(ctx, d, true); err != nil {
		t.Fatalf("double lock: %v", err)
	}
	if err := r.Admit(ctx, d); !errors.Is(err, ErrDeviceLocked) {
		t.Fatalf("locked device admitted: %v", err)
	}
	if err := r.SetLock(ctx, d, false); err != nil {
		t.Fatal(err)
	}
	if err := r.Admit(ctx, d); err != nil {
		t.Fatalf("unlocked device refused: %v", err)
	}

	key := spki(t)
	_ = r.Enroll(ctx, d, key)
	if err := r.Revoke(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := r.Revoke(ctx, d); err != nil {
		t.Fatalf("double revoke: %v", err)
	}
	if _, err := st.DeviceKeys().Get(ctx, d); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("key survives revoke: %v", err)
	}
	if err := r.Admit(ctx, d); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked device admitted: %v", err)
	}
	if err := r.SetLock(ctx, d, false); !errors.Is(err, ErrRevoked) {
		t.Fatalf("unlock must not undo a revoke: %v", err)
	}
	if err := r.SetLock(ctx, d, true); err != nil {
		t.Fatalf("lock of a revoked device: %v", err)
	}
	if err := r.Admit(ctx, d); !errors.Is(err, ErrRevoked) {
		t.Fatalf("lock downgraded a revoke: %v", err)
	}
	if err := r.Enroll(ctx, d, key); !errors.Is(err, ErrRevoked) {
		t.Fatalf("enroll after revoke: %v", err)
	}
}
