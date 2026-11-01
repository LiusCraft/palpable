package device

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
)

type testKey struct {
	priv *ecdsa.PrivateKey
	der  []byte
}

func newTestKey(t *testing.T) testKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return testKey{priv, der}
}

func (k testKey) sign(t *testing.T, d id.DeviceID, nonce string) []byte {
	t.Helper()
	h := sha256.Sum256(SignedMessage(d, nonce))
	r, s, err := ecdsa.Sign(rand.Reader, k.priv, h[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return sig
}

const authMAC id.DeviceID = "02:00:00:00:00:aa"

func TestAuthOpenEnrollsOnFirstValidProof(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory().DeviceKeys()
	a, err := NewAuthenticator(repo, "")
	if err != nil || !a.Required() {
		t.Fatalf("default mode should be open: %v", err)
	}
	k := newTestKey(t)

	if err := a.Verify(ctx, authMAC, nil, "n1", k.sign(t, authMAC, "n1")); !errors.Is(err, ErrNoPublicKey) {
		t.Fatalf("first contact without pubkey: %v", err)
	}
	// 签名不对时不登记。
	if err := a.Verify(ctx, authMAC, k.der, "n1", k.sign(t, authMAC, "other")); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("bad sig: %v", err)
	}
	if _, err := repo.Get(ctx, authMAC); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("key must not be enrolled after a failed proof")
	}
	if err := a.Verify(ctx, authMAC, k.der, "n2", k.sign(t, authMAC, "n2")); err != nil {
		t.Fatal(err)
	}
	// 之后可以不带公钥。
	if err := a.Verify(ctx, authMAC, nil, "n3", k.sign(t, authMAC, "n3")); err != nil {
		t.Fatal(err)
	}
	// 别人的钥匙冒充同一 MAC：公钥不符。
	evil := newTestKey(t)
	if err := a.Verify(ctx, authMAC, evil.der, "n4", evil.sign(t, authMAC, "n4")); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("impostor: %v", err)
	}
	// 不带公钥、拿自己的钥匙签：签名不符。
	if err := a.Verify(ctx, authMAC, nil, "n5", evil.sign(t, authMAC, "n5")); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("impostor without key: %v", err)
	}
	// 签名绑定了设备 ID：拿别的设备的签名不行。
	if err := a.Verify(ctx, authMAC, nil, "n6", k.sign(t, "02:00:00:00:00:bb", "n6")); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("cross-device signature: %v", err)
	}
}

func TestAuthManifestRejectsUnknown(t *testing.T) {
	ctx := context.Background()
	a, _ := NewAuthenticator(store.NewMemory().DeviceKeys(), EnrollManifest)
	k := newTestKey(t)
	if err := a.Verify(ctx, authMAC, k.der, "n", k.sign(t, authMAC, "n")); !errors.Is(err, ErrUnknownDevice) {
		t.Fatalf("unknown device: %v", err)
	}
	if err := a.Enroll(ctx, authMAC, k.der); err != nil {
		t.Fatal(err)
	}
	if err := a.Enroll(ctx, authMAC, k.der); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("re-enroll: %v", err)
	}
	if err := a.Verify(ctx, authMAC, nil, "n", k.sign(t, authMAC, "n")); err != nil {
		t.Fatal(err)
	}
}

func TestAuthRejectsBadKeys(t *testing.T) {
	ctx := context.Background()
	a, _ := NewAuthenticator(store.NewMemory().DeviceKeys(), EnrollOpen)
	if err := a.Verify(ctx, authMAC, []byte("junk"), "n", make([]byte, 64)); !errors.Is(err, ErrBadPublicKey) {
		t.Fatalf("junk key: %v", err)
	}
	rk, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&rk.PublicKey)
	if err := a.Enroll(ctx, authMAC, der); !errors.Is(err, ErrBadPublicKey) {
		t.Fatalf("rsa key: %v", err)
	}
	k := newTestKey(t)
	if err := a.Verify(ctx, authMAC, k.der, "n", []byte{1, 2}); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("short sig: %v", err)
	}
	if _, err := NewAuthenticator(nil, "nope"); err == nil {
		t.Fatal("unknown mode should fail")
	}
	off, _ := NewAuthenticator(nil, EnrollOff)
	if off.Required() {
		t.Fatal("off mode should not require auth")
	}
}
