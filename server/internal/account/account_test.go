package account

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/wechat"
)

type stubWX struct {
	err error
}

func (s stubWX) Code2Session(ctx context.Context, code string) (wechat.Identity, error) {
	if s.err != nil {
		return wechat.Identity{}, s.err
	}
	return wechat.Fake{}.Code2Session(ctx, code)
}

func newAccounts(t *testing.T, wx wechat.Client) (*Accounts, *time.Time) {
	t.Helper()
	st := store.NewMemory()
	a := New(st.Guardians(), st.Sessions(), wx)
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	return a, &now
}

func TestSignInCreatesAccountOnceAndIssuesTokens(t *testing.T) {
	ctx := context.Background()
	a, _ := newAccounts(t, wechat.Fake{})
	t1, err := a.SignInWeChat(ctx, "mom")
	if err != nil {
		t.Fatal(err)
	}
	t2, err := a.SignInWeChat(ctx, "mom")
	if err != nil {
		t.Fatal(err)
	}
	if t1.Guardian != t2.Guardian || t1.Value == t2.Value {
		t.Fatalf("same openid should map to one guardian with distinct tokens: %+v %+v", t1, t2)
	}
	other, err := a.SignInWeChat(ctx, "dad")
	if err != nil {
		t.Fatal(err)
	}
	if other.Guardian == t1.Guardian {
		t.Fatal("different openids share a guardian")
	}
	for _, tok := range []Token{t1, t2} {
		if g, ok := a.Verify(ctx, tok.Value); !ok || g != t1.Guardian {
			t.Fatalf("verify(%s) = %s %v", tok.Value, g, ok)
		}
	}
}

func TestConcurrentFirstSignInSharesGuardian(t *testing.T) {
	ctx := context.Background()
	a, _ := newAccounts(t, wechat.Fake{})
	var wg sync.WaitGroup
	got := make([]Token, 8)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := a.SignInWeChat(ctx, "same")
			if err != nil {
				t.Error(err)
			}
			got[i] = tok
		}()
	}
	wg.Wait()
	for _, tok := range got[1:] {
		if tok.Guardian != got[0].Guardian {
			t.Fatalf("concurrent sign-in created two guardians: %s vs %s", tok.Guardian, got[0].Guardian)
		}
	}
}

func TestSignInMapsWeChatErrors(t *testing.T) {
	cases := []struct {
		wx   error
		want error
	}{
		{wechat.ErrInvalidCode, ErrBadCode},
		{wechat.ErrRiskyUser, ErrRejected},
		{wechat.ErrRateLimited, ErrRateLimited},
	}
	for _, c := range cases {
		a, _ := newAccounts(t, stubWX{err: c.wx})
		if _, err := a.SignInWeChat(context.Background(), "x"); !errors.Is(err, c.want) {
			t.Errorf("wx %v: err = %v, want %v", c.wx, err, c.want)
		}
	}
	a, _ := newAccounts(t, wechat.Fake{})
	if _, err := a.SignInWeChat(context.Background(), ""); !errors.Is(err, ErrBadCode) {
		t.Fatalf("empty code: %v", err)
	}
}

func TestVerifyRejectsExpiredSignedOutAndGarbage(t *testing.T) {
	ctx := context.Background()
	a, now := newAccounts(t, wechat.Fake{})
	tok, err := a.SignInWeChat(ctx, "mom")
	if err != nil {
		t.Fatal(err)
	}
	// 篡改最后一位：必须换成不同的字符，否则恰好相同时"篡改"后仍是原令牌。
	last := "A"
	if tok.Value[42:] == "A" {
		last = "B"
	}
	for _, bad := range []string{"", "short", tok.Value[:42] + last} {
		if _, ok := a.Verify(ctx, bad); ok {
			t.Fatalf("verify(%q) accepted", bad)
		}
	}
	keep, _ := a.SignInWeChat(ctx, "mom")
	if err := a.SignOut(ctx, tok.Value); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Verify(ctx, tok.Value); ok {
		t.Fatal("signed-out token still valid")
	}
	if err := a.SignOut(ctx, tok.Value); err != nil {
		t.Fatalf("second sign-out: %v", err)
	}
	*now = now.Add(SessionTTL)
	if _, ok := a.Verify(ctx, keep.Value); ok {
		t.Fatal("expired token still valid")
	}
	if n, err := a.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
}

func TestDisplayNameAndDelete(t *testing.T) {
	ctx := context.Background()
	a, _ := newAccounts(t, wechat.Fake{})
	tok, _ := a.SignInWeChat(ctx, "mom")
	for _, bad := range []string{"", "   ", "一二三四五六七八九十一二三四五六七八九十多"} {
		if err := a.SetDisplayName(ctx, tok.Guardian, bad); !errors.Is(err, ErrBadName) {
			t.Fatalf("name %q: %v", bad, err)
		}
	}
	if err := a.SetDisplayName(ctx, tok.Guardian, " 朵朵妈妈 "); err != nil {
		t.Fatal(err)
	}
	p, err := a.Profile(ctx, tok.Guardian)
	if err != nil || p.DisplayName != "朵朵妈妈" {
		t.Fatalf("profile = %+v, %v", p, err)
	}
	if err := a.Delete(ctx, tok.Guardian); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Verify(ctx, tok.Value); ok {
		t.Fatal("token survives account deletion")
	}
	if _, err := a.Profile(ctx, tok.Guardian); !errors.Is(err, ErrNotFound) {
		t.Fatalf("profile after delete: %v", err)
	}
	// 注销后同一个微信再登录是全新的账号。
	again, _ := a.SignInWeChat(ctx, "mom")
	if again.Guardian == tok.Guardian {
		t.Fatal("deleted guardian id reused")
	}
}
