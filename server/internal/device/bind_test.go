package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

func newBinder(t *testing.T) (*Binder, *Devices, *time.Time) {
	t.Helper()
	ctx := context.Background()
	s := store.NewMemory()
	fake := linxapi.NewFake()
	cat := NewCatalog(s.Styles(), fake)
	if err := cat.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	devs := New(s.Devices(), cat, fake)
	for _, mac := range []string{"02:00:00:00:00:01", "02:00:00:00:00:02"} {
		if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "bear"}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	b := NewBinder(devs)
	b.now = func() time.Time { return now }
	return b, devs, &now
}

func TestBindClaimOnceThenAlreadyBound(t *testing.T) {
	ctx := context.Background()
	b, devs, _ := newBinder(t)
	d := id.DeviceID("02:00:00:00:00:01")
	code, ttl := b.Issue(d)
	if len(code) != 6 || ttl != BindCodeTTL {
		t.Fatalf("code %q ttl %v", code, ttl)
	}
	got, err := b.Claim(ctx, "g1", code, true)
	if err != nil || got != d {
		t.Fatalf("claim: %v %v", got, err)
	}
	if e := <-b.Subscribe(); e.Device != d || e.Guardian != "g1" {
		t.Fatalf("event %+v", e)
	}
	p, _ := devs.Profile(ctx, d)
	if p.Guardian != "g1" || !p.VoiceConsent {
		t.Fatalf("profile %+v", p)
	}
	// 码只能用一次。
	if _, err := b.Claim(ctx, "g2", code, false); !errors.Is(err, ErrBadBindCode) {
		t.Fatalf("reuse: %v", err)
	}
	// 已绑定的设备不能被另一位家长认领（即使拿到了新码）。
	code2, _ := b.Issue(d)
	if _, err := b.Claim(ctx, "g2", code2, false); !errors.Is(err, ErrAlreadyBound) {
		t.Fatalf("rebind: %v", err)
	}
}

func TestBindCodeExpiresAndRevokes(t *testing.T) {
	ctx := context.Background()
	b, _, now := newBinder(t)
	d := id.DeviceID("02:00:00:00:00:01")
	code, _ := b.Issue(d)
	*now = now.Add(BindCodeTTL)
	if _, err := b.Claim(ctx, "g1", code, false); !errors.Is(err, ErrBadBindCode) {
		t.Fatalf("expired: %v", err)
	}
	code, _ = b.Issue(d)
	b.Revoke(d)
	if _, err := b.Claim(ctx, "g1", code, false); !errors.Is(err, ErrBadBindCode) {
		t.Fatalf("revoked: %v", err)
	}
	// 重新发码时旧码作废。
	old, _ := b.Issue(d)
	fresh, _ := b.Issue(d)
	if old != fresh {
		if _, err := b.Claim(ctx, "g1", old, false); !errors.Is(err, ErrBadBindCode) {
			t.Fatalf("old code after reissue: %v", err)
		}
	}
	if _, err := b.Claim(ctx, "", fresh, false); err == nil {
		t.Fatal("empty guardian should fail")
	}
}

func TestBindRateLimitsWrongCodes(t *testing.T) {
	ctx := context.Background()
	b, _, now := newBinder(t)
	code, _ := b.Issue("02:00:00:00:00:02")
	wrong := "000000"
	if wrong == code {
		wrong = "000001"
	}
	for range bindMaxFails {
		if _, err := b.Claim(ctx, "g1", wrong, false); !errors.Is(err, ErrBadBindCode) {
			t.Fatalf("wrong code: %v", err)
		}
	}
	// 猜够次数后，正确的码也暂时不收。
	if _, err := b.Claim(ctx, "g1", code, false); !errors.Is(err, ErrTooManyTries) {
		t.Fatalf("rate limit: %v", err)
	}
	// 别的家长不受影响；时间过去后恢复。
	*now = now.Add(bindWindow)
	code, _ = b.Issue("02:00:00:00:00:02")
	if _, err := b.Claim(ctx, "g1", code, false); err != nil {
		t.Fatalf("after window: %v", err)
	}
}

func TestUnbindOnlyByOwner(t *testing.T) {
	ctx := context.Background()
	b, devs, _ := newBinder(t)
	d := id.DeviceID("02:00:00:00:00:01")
	code, _ := b.Issue(d)
	if _, err := b.Claim(ctx, "g1", code, false); err != nil {
		t.Fatal(err)
	}
	if _, err := devs.Unbind(ctx, d, "g2"); !errors.Is(err, ErrNotYourDevice) {
		t.Fatalf("stranger unbind: %v", err)
	}
	if _, err := devs.Unbind(ctx, d, "g1"); err != nil {
		t.Fatal(err)
	}
	if _, err := devs.Profile(ctx, d); !errors.Is(err, ErrNotFound) {
		t.Fatalf("profile should be gone: %v", err)
	}
	if _, err := devs.Unbind(ctx, d, "g1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second unbind: %v", err)
	}
	if err := devs.Claim(ctx, d, "g1", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim missing: %v", err)
	}
}

func TestFriendPolicy(t *testing.T) {
	ctx := context.Background()
	_, devs, _ := newBinder(t)
	d := id.DeviceID("02:00:00:00:00:01")
	if devs.AutoApproveFriends(ctx, d) {
		t.Fatal("default must require guardian approval")
	}
	if err := devs.SetFriendPolicy(ctx, d, FriendNotify); err != nil {
		t.Fatal(err)
	}
	if !devs.AutoApproveFriends(ctx, d) {
		t.Fatal("notify policy should auto approve")
	}
	if err := devs.SetFriendPolicy(ctx, d, 9); err == nil {
		t.Fatal("unknown policy should fail")
	}
	if devs.AutoApproveFriends(ctx, "02:00:00:00:00:99") {
		t.Fatal("unknown device must not auto approve")
	}
}
