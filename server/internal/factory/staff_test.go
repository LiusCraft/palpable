package factory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/factory"
	"github.com/LiusCraft/palpable/server/internal/factorydb"
)

func newAuth(t *testing.T) (*factory.Auth, *clock, *factorydb.DB) {
	t.Helper()
	db, err := factorydb.Open(context.Background(), t.TempDir()+"/a.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clk := &clock{t: time.Now()}
	return factory.NewAuth(db, clk.now), clk, db
}

func TestBootstrapAndLogin(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newAuth(t)
	if made, err := a.Bootstrap(ctx, "", ""); made || err != nil {
		t.Fatalf("empty bootstrap: %v %v", made, err)
	}
	if _, err := a.Bootstrap(ctx, "boss", "short"); !errors.Is(err, factory.ErrInvalid) {
		t.Fatalf("weak bootstrap password: %v", err)
	}
	if made, err := a.Bootstrap(ctx, "boss", "correct horse battery"); !made || err != nil {
		t.Fatalf("bootstrap: %v %v", made, err)
	}
	if made, _ := a.Bootstrap(ctx, "other", "correct horse battery"); made {
		t.Fatal("bootstrap must only run on an empty database")
	}

	tok, st, err := a.Login(ctx, " Boss ", "correct horse battery")
	if err != nil || st.Role != factory.RoleAdmin || tok == "" {
		t.Fatalf("login: %v %+v", err, st)
	}
	got, err := a.Authenticate(ctx, tok)
	if err != nil || got.ID != st.ID {
		t.Fatalf("authenticate: %v %+v", err, got)
	}
	if _, err := a.Authenticate(ctx, ""); !errors.Is(err, factory.ErrUnauth) {
		t.Fatalf("empty token: %v", err)
	}
	if _, err := a.Authenticate(ctx, "garbage"); !errors.Is(err, factory.ErrUnauth) {
		t.Fatalf("unknown token: %v", err)
	}
	if err := a.Logout(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, tok); !errors.Is(err, factory.ErrUnauth) {
		t.Fatalf("token after logout: %v", err)
	}
}

func TestSessionExpiresAndLoginThrottle(t *testing.T) {
	ctx := context.Background()
	a, clk, _ := newAuth(t)
	_, _ = a.Bootstrap(ctx, "boss", "correct horse battery")
	tok, _, _ := a.Login(ctx, "boss", "correct horse battery")
	clk.t = clk.t.Add(13 * time.Hour)
	if _, err := a.Authenticate(ctx, tok); !errors.Is(err, factory.ErrUnauth) {
		t.Fatalf("expired session: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, _, err := a.Login(ctx, "boss", "wrong password!"); !errors.Is(err, factory.ErrBadLogin) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	// 锁定后连正确口令也不行。
	if _, _, err := a.Login(ctx, "boss", "correct horse battery"); !errors.Is(err, factory.ErrTooManyTry) {
		t.Fatalf("throttle: %v", err)
	}
	clk.t = clk.t.Add(11 * time.Minute)
	if _, _, err := a.Login(ctx, "boss", "correct horse battery"); err != nil {
		t.Fatalf("after window: %v", err)
	}
	// 不存在的用户同样计数，错误与口令错误一致。
	if _, _, err := a.Login(ctx, "ghost", "whatever-password"); !errors.Is(err, factory.ErrBadLogin) {
		t.Fatalf("unknown user: %v", err)
	}
}

func TestStaffManagement(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newAuth(t)
	_, _ = a.Bootstrap(ctx, "boss", "correct horse battery")
	_, boss, _ := a.Login(ctx, "boss", "correct horse battery")

	if _, err := a.Create(ctx, factory.Staff{Role: factory.RoleSales}, "x-user", "long enough pw", factory.RoleViewer); !errors.Is(err, factory.ErrForbidden) {
		t.Fatalf("non-admin create: %v", err)
	}
	for name, c := range map[string]struct {
		user, pw string
		role     factory.Role
	}{
		"short name": {"ab", "long enough pw", factory.RoleViewer},
		"bad chars":  {"Bad Name", "long enough pw", factory.RoleViewer},
		"weak pw":    {"alice", "123", factory.RoleViewer},
		"bad role":   {"alice", "long enough pw", "god"},
	} {
		if _, err := a.Create(ctx, boss, c.user, c.pw, c.role); !errors.Is(err, factory.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	alice, err := a.Create(ctx, boss, "alice", "long enough pw", factory.RoleWarehouse)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Create(ctx, boss, "alice", "long enough pw", factory.RoleWarehouse); !errors.Is(err, factory.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	if list, err := a.List(ctx, boss); err != nil || len(list) != 2 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if _, err := a.List(ctx, alice); !errors.Is(err, factory.ErrForbidden) {
		t.Fatalf("non-admin list: %v", err)
	}

	tok, _, _ := a.Login(ctx, "alice", "long enough pw")
	if err := a.Update(ctx, boss, boss.ID, factory.RoleViewer, false); !errors.Is(err, factory.ErrInvalid) {
		t.Fatalf("self demote: %v", err)
	}
	if err := a.Update(ctx, boss, boss.ID, factory.RoleAdmin, true); !errors.Is(err, factory.ErrInvalid) {
		t.Fatalf("self disable: %v", err)
	}
	if err := a.Update(ctx, alice, alice.ID, factory.RoleAdmin, false); !errors.Is(err, factory.ErrForbidden) {
		t.Fatalf("non-admin update: %v", err)
	}
	if err := a.Update(ctx, boss, 999, factory.RoleViewer, false); !errors.Is(err, factory.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := a.Update(ctx, boss, alice.ID, "god", false); !errors.Is(err, factory.ErrInvalid) {
		t.Fatalf("bad role: %v", err)
	}
	if err := a.Update(ctx, boss, alice.ID, factory.RoleSupport, true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, tok); !errors.Is(err, factory.ErrUnauth) {
		t.Fatalf("disabled staff keeps working: %v", err)
	}
	if _, _, err := a.Login(ctx, "alice", "long enough pw"); !errors.Is(err, factory.ErrBadLogin) {
		t.Fatalf("disabled login: %v", err)
	}
	if err := a.Update(ctx, boss, alice.ID, factory.RoleSupport, false); err != nil {
		t.Fatal(err)
	}
	if _, st, err := a.Login(ctx, "alice", "long enough pw"); err != nil || st.Role != factory.RoleSupport {
		t.Fatalf("re-enabled: %v %+v", err, st)
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newAuth(t)
	_, _ = a.Bootstrap(ctx, "boss", "correct horse battery")
	tok, boss, _ := a.Login(ctx, "boss", "correct horse battery")
	if err := a.ChangePassword(ctx, boss, "wrong old pass", "brand new password"); !errors.Is(err, factory.ErrBadLogin) {
		t.Fatalf("wrong old: %v", err)
	}
	if err := a.ChangePassword(ctx, boss, "correct horse battery", "short"); !errors.Is(err, factory.ErrInvalid) {
		t.Fatalf("weak new: %v", err)
	}
	if err := a.ChangePassword(ctx, boss, "correct horse battery", "brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, tok); !errors.Is(err, factory.ErrUnauth) {
		t.Fatalf("old session survives a password change: %v", err)
	}
	if _, _, err := a.Login(ctx, "boss", "correct horse battery"); !errors.Is(err, factory.ErrBadLogin) {
		t.Fatalf("old password still works: %v", err)
	}
	if _, _, err := a.Login(ctx, "boss", "brand new password"); err != nil {
		t.Fatal(err)
	}
}

func TestNoopPlatformAlwaysSucceeds(t *testing.T) {
	var p factory.Platform = factory.NoopPlatform{}
	ctx := context.Background()
	if p.Enroll(ctx, "m", nil) != nil || p.Revoke(ctx, "m") != nil || p.Wipe(ctx, "m") != nil || p.SetLock(ctx, "m", true) != nil {
		t.Fatal("noop platform must never fail")
	}
}
