package factory_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/factory"
	"github.com/LiusCraft/palpable/server/internal/factorydb"
)

type fakePlatform struct {
	mu       sync.Mutex
	enrolled map[string][]byte
	wiped    []string
	revoked  []string
	locks    map[string]bool
	failWipe error
	failLock error
}

func newFake() *fakePlatform {
	return &fakePlatform{enrolled: map[string][]byte{}, locks: map[string]bool{}}
}

func (f *fakePlatform) Enroll(_ context.Context, mac string, k []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enrolled[mac] = k
	return nil
}
func (f *fakePlatform) Revoke(_ context.Context, mac string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked = append(f.revoked, mac)
	return nil
}
func (f *fakePlatform) Wipe(_ context.Context, mac string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWipe != nil {
		return f.failWipe
	}
	f.wiped = append(f.wiped, mac)
	return nil
}
func (f *fakePlatform) SetLock(_ context.Context, mac string, l bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failLock != nil {
		return f.failLock
	}
	f.locks[mac] = l
	return nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type env struct {
	svc  *factory.Service
	db   *factorydb.DB
	plat *fakePlatform
	clk  *clock
	ctx  context.Context
}

var (
	admin     = factory.Actor{Name: "root", Role: factory.RoleAdmin}
	warehouse = factory.Actor{Name: "wh", Role: factory.RoleWarehouse}
	sales     = factory.Actor{Name: "sa", Role: factory.RoleSales}
	support   = factory.Actor{Name: "su", Role: factory.RoleSupport}
	viewer    = factory.Actor{Name: "vi", Role: factory.RoleViewer}
)

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	db, err := factorydb.Open(ctx, t.TempDir()+"/admin.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &env{db: db, plat: newFake(), clk: &clock{t: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}, ctx: ctx}
	e.svc = factory.NewService(db, e.plat, factory.Options{Now: e.clk.now})
	if err := e.svc.PutChannel(ctx, admin, factory.Channel{Code: "jd", Name: "京东自营", Kind: factory.ChannelOnline}); err != nil {
		t.Fatal(err)
	}
	return e
}

func newKey(t *testing.T) string {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func (e *env) batch(t *testing.T, code string, n int) []factory.Unit {
	t.Helper()
	in := factory.BatchInput{Code: code, Model: "可感 一代", Skin: "bear", Firmware: "1.0.0"}
	for i := 0; i < n; i++ {
		in.Units = append(in.Units, factory.UnitKey{MAC: fmt.Sprintf("AA-%02X-%02X-%02X-%02X-%02X", code[0], code[1], code[len(code)-2], code[len(code)-1], i), PublicKey: newKey(t)})
	}
	us, err := e.svc.ImportBatch(e.ctx, warehouse, in)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return us
}

func (e *env) do(t *testing.T, a factory.Actor, serial string, act factory.Action, p factory.Params) factory.Unit {
	t.Helper()
	u, err := e.svc.Apply(e.ctx, a, serial, act, p)
	if err != nil {
		t.Fatalf("%s %s: %v", act, serial, err)
	}
	return u
}

func (e *env) state(t *testing.T, serial string) factory.Unit {
	t.Helper()
	u, _, err := e.svc.Detail(e.ctx, serial)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// toShelf 把一台设备走到 on_shelf。
func (e *env) toShelf(t *testing.T, serial string) {
	t.Helper()
	e.do(t, warehouse, serial, factory.ActQCPass, factory.Params{})
	e.do(t, warehouse, serial, factory.ActShip, factory.Params{Channel: "jd"})
	e.do(t, sales, serial, factory.ActReceive, factory.Params{})
}

func TestImportBatch(t *testing.T) {
	e := newEnv(t)
	us := e.batch(t, "A2611", 3)
	if len(us) != 3 || us[0].Serial != "PPA2611-00001" || us[2].Serial != "PPA2611-00003" {
		t.Fatalf("serials: %+v", us)
	}
	if us[0].MAC != "aa:41:32:31:31:00" || us[0].State != factory.Produced || us[0].Grade != factory.GradeNew {
		t.Fatalf("unit: %+v", us[0])
	}
	if len(e.plat.enrolled) != 3 {
		t.Fatalf("enrolled %d keys, want 3", len(e.plat.enrolled))
	}
	bs, err := e.svc.Batches(e.ctx)
	if err != nil || len(bs) != 1 || bs[0].Units != 3 {
		t.Fatalf("batches: %+v %v", bs, err)
	}
	_, evs, _ := e.svc.Detail(e.ctx, "PPA2611-00001")
	if len(evs) != 1 || evs[0].Action != "imported" || evs[0].Actor != "wh" {
		t.Fatalf("events: %+v", evs)
	}
}

func TestImportBatchRejects(t *testing.T) {
	e := newEnv(t)
	key := newKey(t)
	ok := factory.UnitKey{MAC: "aa:bb:cc:dd:ee:01", PublicKey: key}
	base := factory.BatchInput{Code: "B1000", Model: "m", Skin: "bear", Units: []factory.UnitKey{ok}}
	with := func(f func(*factory.BatchInput)) factory.BatchInput {
		in := base
		in.Units = append([]factory.UnitKey(nil), base.Units...)
		f(&in)
		return in
	}

	for name, in := range map[string]factory.BatchInput{
		"bad code": with(func(in *factory.BatchInput) { in.Code = "ab" }),
		"bad skin": with(func(in *factory.BatchInput) { in.Skin = "Bear!" }),
		"no model": with(func(in *factory.BatchInput) { in.Model = " " }),
		"empty":    with(func(in *factory.BatchInput) { in.Units = nil }),
		"bad mac":  with(func(in *factory.BatchInput) { in.Units[0].MAC = "xyz" }),
		"dup mac": with(func(in *factory.BatchInput) {
			in.Units = append(in.Units, factory.UnitKey{MAC: "AA-BB-CC-DD-EE-01", PublicKey: key})
		}),
		"not base64": with(func(in *factory.BatchInput) { in.Units[0].PublicKey = "!!!" }),
		"not spki": with(func(in *factory.BatchInput) {
			in.Units[0].PublicKey = base64.StdEncoding.EncodeToString([]byte("nope"))
		}),
		"long firmware": with(func(in *factory.BatchInput) { in.Firmware = string(make([]byte, 41)) }),
	} {
		if _, err := e.svc.ImportBatch(e.ctx, admin, in); !errors.Is(err, factory.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	// P-384 的公钥不收。
	k, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	bad := with(func(in *factory.BatchInput) { in.Units[0].PublicKey = base64.StdEncoding.EncodeToString(der) })
	if _, err := e.svc.ImportBatch(e.ctx, admin, bad); !errors.Is(err, factory.ErrInvalid) {
		t.Errorf("P-384: err = %v", err)
	}
	if _, err := e.svc.ImportBatch(e.ctx, sales, base); !errors.Is(err, factory.ErrForbidden) {
		t.Errorf("sales import: err = %v", err)
	}
	if len(e.plat.enrolled) != 0 {
		t.Fatalf("rejected batches must not enroll keys, got %d", len(e.plat.enrolled))
	}

	if _, err := e.svc.ImportBatch(e.ctx, admin, base); err != nil {
		t.Fatal(err)
	}
	// 同一批次号、同一 MAC 再导入：整批回滚。
	if _, err := e.svc.ImportBatch(e.ctx, admin, base); !errors.Is(err, factory.ErrConflict) {
		t.Errorf("duplicate batch: err = %v", err)
	}
	other := with(func(in *factory.BatchInput) { in.Code = "B2000" })
	if _, err := e.svc.ImportBatch(e.ctx, admin, other); !errors.Is(err, factory.ErrConflict) {
		t.Errorf("duplicate mac: err = %v", err)
	}
	if bs, _ := e.svc.Batches(e.ctx); len(bs) != 1 {
		t.Errorf("a failed batch left %d batches behind", len(bs))
	}
}

func TestImportNeedsPlatform(t *testing.T) {
	e := newEnv(t)
	svc := factory.NewService(e.db, nil, factory.Options{})
	in := factory.BatchInput{Code: "C1000", Model: "m", Skin: "bear", Units: []factory.UnitKey{{MAC: "aa:bb:cc:dd:ee:09", PublicKey: newKey(t)}}}
	if _, err := svc.ImportBatch(e.ctx, admin, in); !errors.Is(err, factory.ErrNoPlatform) {
		t.Fatalf("err = %v, want ErrNoPlatform", err)
	}
}

func TestFullLifecycle(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "L1000", 1)[0].Serial

	e.do(t, warehouse, s, factory.ActQCPass, factory.Params{})
	if len(e.plat.wiped) != 1 {
		t.Fatal("qc_pass must wipe leftover test bindings")
	}
	u := e.do(t, warehouse, s, factory.ActShip, factory.Params{Channel: "jd"})
	if u.State != factory.InTransit || u.Channel != "jd" {
		t.Fatalf("ship: %+v", u)
	}
	e.do(t, sales, s, factory.ActReceive, factory.Params{})
	u = e.do(t, sales, s, factory.ActSell, factory.Params{OrderRef: "JD-2026/11-001"})
	if u.State != factory.Sold || !u.SoldAt.Equal(e.clk.t) || !u.WarrantyUntil.Equal(time.Date(2027, 11, 1, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("sell: %+v", u)
	}

	if err := e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: u.MAC, Kind: "activated", OwnerRef: "owner-A-0001"}); err != nil {
		t.Fatal(err)
	}
	u = e.state(t, s)
	if u.State != factory.Activated || u.OwnerRef != "owner-A-0001" {
		t.Fatalf("activated: %+v", u)
	}

	// 用户自己转让：原主解绑，新家长绑定。
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: u.MAC, Kind: "unbound"})
	if u = e.state(t, s); u.State != factory.Sold || u.OwnerRef != "" || u.PrevOwnerRef != "owner-A-0001" {
		t.Fatalf("unbound: %+v", u)
	}
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: u.MAC, Kind: "activated", OwnerRef: "owner-B-0002"})
	if u = e.state(t, s); u.State != factory.Activated || u.TransferCount != 1 {
		t.Fatalf("transfer: %+v", u)
	}
	if !u.WarrantyUntil.Equal(time.Date(2027, 11, 1, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("transfer must not restart the warranty")
	}

	// 退货 → 收货擦除 → 检测 → 翻新 → 翻新入库 → 再上架 → 再售出。
	e.do(t, support, s, factory.ActReturnRequest, factory.Params{Note: "七天无理由"})
	u = e.do(t, warehouse, s, factory.ActReceiveReturn, factory.Params{})
	if u.State != factory.Returned || u.OwnerRef != "" || u.PrevOwnerRef != "" || !u.WarrantyUntil.IsZero() || u.OrderRef != "" {
		t.Fatalf("receive_return: %+v", u)
	}
	wipes := len(e.plat.wiped)
	e.do(t, warehouse, s, factory.ActInspect, factory.Params{})
	e.do(t, warehouse, s, factory.ActRefurbish, factory.Params{})
	u = e.do(t, warehouse, s, factory.ActRefurbishDone, factory.Params{Grade: factory.GradeA})
	if u.State != factory.Refurbished || u.Grade != factory.GradeA || u.RefurbCount != 1 || len(e.plat.wiped) != wipes+1 {
		t.Fatalf("refurbish_done: %+v wipes %d->%d", u, wipes, len(e.plat.wiped))
	}
	e.do(t, warehouse, s, factory.ActShip, factory.Params{Channel: "jd"})
	e.do(t, sales, s, factory.ActReceive, factory.Params{})
	u = e.do(t, sales, s, factory.ActSell, factory.Params{OrderRef: "JD-2026/12-009"})
	if !u.WarrantyUntil.Equal(e.clk.t.AddDate(0, 6, 0)) {
		t.Fatalf("refurbished warranty: %v", u.WarrantyUntil)
	}
	// 新买家不算转让。
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: u.MAC, Kind: "activated", OwnerRef: "owner-C-0003"})
	if u = e.state(t, s); u.TransferCount != 1 || u.State != factory.Activated {
		t.Fatalf("resale: %+v", u)
	}

	_, evs, _ := e.svc.Detail(e.ctx, s)
	var last string
	for _, ev := range evs {
		if ev.Anomaly {
			t.Errorf("unexpected anomaly: %+v", ev)
		}
		last = ev.Action
	}
	if last != "activated" || len(evs) < 15 {
		t.Fatalf("event trail: %d events, last %s", len(evs), last)
	}
}

func TestWipeIsAHardGate(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "W1000", 1)[0].Serial
	e.plat.failWipe = errors.New("palpabled down")
	if _, err := e.svc.Apply(e.ctx, warehouse, s, factory.ActQCPass, factory.Params{}); err == nil {
		t.Fatal("qc_pass must fail when the wipe fails")
	}
	if u := e.state(t, s); u.State != factory.Produced || u.Version != 0 {
		t.Fatalf("state changed despite failure: %+v", u)
	}
	if _, evs, _ := e.svc.Detail(e.ctx, s); len(evs) != 1 {
		t.Fatalf("failed action left events: %+v", evs)
	}
	e.plat.failWipe = nil
	e.do(t, warehouse, s, factory.ActQCPass, factory.Params{})

	nop := factory.NewService(e.db, nil, factory.Options{})
	e.do(t, warehouse, s, factory.ActShip, factory.Params{Channel: "jd"})
	e.do(t, sales, s, factory.ActReceive, factory.Params{})
	e.do(t, sales, s, factory.ActSell, factory.Params{OrderRef: "X-1"})
	e.do(t, support, s, factory.ActReturnRequest, factory.Params{Note: "坏了"})
	if _, err := nop.Apply(e.ctx, warehouse, s, factory.ActReceiveReturn, factory.Params{}); !errors.Is(err, factory.ErrNoPlatform) {
		t.Fatalf("err = %v, want ErrNoPlatform", err)
	}
}

func TestTransitionsAndRoles(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "T1000", 1)[0].Serial
	cases := []struct {
		name string
		a    factory.Actor
		act  factory.Action
		p    factory.Params
		want error
	}{
		{"sell from produced", sales, factory.ActSell, factory.Params{OrderRef: "X"}, factory.ErrBadTransition},
		{"viewer cannot qc", viewer, factory.ActQCPass, factory.Params{}, factory.ErrForbidden},
		{"sales cannot qc", sales, factory.ActQCPass, factory.Params{}, factory.ErrForbidden},
		{"unknown action", admin, "teleport", factory.Params{}, factory.ErrInvalid},
		{"qc_fail needs a note", warehouse, factory.ActQCFail, factory.Params{}, factory.ErrInvalid},
		{"note too long", warehouse, factory.ActQCFail, factory.Params{Note: string(make([]rune, 201))}, factory.ErrInvalid},
		{"unlock when unlocked", support, factory.ActUnlock, factory.Params{}, factory.ErrBadTransition},
		{"lock needs a reason", support, factory.ActLock, factory.Params{}, factory.ErrInvalid},
	}
	for _, c := range cases {
		if _, err := e.svc.Apply(e.ctx, c.a, s, c.act, c.p); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := e.svc.Apply(e.ctx, admin, "NOPE", factory.ActQCPass, factory.Params{}); !errors.Is(err, factory.ErrNotFound) {
		t.Errorf("missing unit: %v", err)
	}

	e.toShelf(t, s)
	for name, p := range map[string]factory.Params{
		"no order":     {},
		"order spaces": {OrderRef: "张三 13800138000"},
		"order long":   {OrderRef: string(make([]byte, 41))},
	} {
		if _, err := e.svc.Apply(e.ctx, sales, s, factory.ActSell, p); !errors.Is(err, factory.ErrInvalid) {
			t.Errorf("sell %s: %v", name, err)
		}
	}
	if _, err := e.svc.Apply(e.ctx, warehouse, s, factory.ActShip, factory.Params{Channel: "jd"}); !errors.Is(err, factory.ErrBadTransition) {
		t.Errorf("ship from on_shelf: %v", err)
	}
	e.do(t, sales, s, factory.ActUnshelf, factory.Params{})
	if _, err := e.svc.Apply(e.ctx, warehouse, s, factory.ActShip, factory.Params{Channel: "nowhere"}); !errors.Is(err, factory.ErrInvalid) {
		t.Errorf("unknown channel: %v", err)
	}
	if u := e.state(t, s); u.State != factory.InStock || u.Channel != "" {
		t.Fatalf("unshelf: %+v", u)
	}
}

func TestQCFailReworkScrap(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "Q1000", 2)
	e.do(t, warehouse, s[0].Serial, factory.ActQCFail, factory.Params{Note: "喇叭无声"})
	e.do(t, warehouse, s[0].Serial, factory.ActRework, factory.Params{})
	if e.state(t, s[0].Serial).State != factory.Produced {
		t.Fatal("rework should return to produced")
	}
	e.do(t, warehouse, s[1].Serial, factory.ActQCFail, factory.Params{Note: "进水"})
	e.do(t, warehouse, s[1].Serial, factory.ActScrap, factory.Params{Note: "无法修复"})
	if len(e.plat.revoked) != 1 || e.plat.revoked[0] != s[1].MAC {
		t.Fatalf("revoked: %v", e.plat.revoked)
	}
	if _, err := e.svc.Apply(e.ctx, admin, s[1].Serial, factory.ActRework, factory.Params{}); !errors.Is(err, factory.ErrBadTransition) {
		t.Fatalf("scrapped is terminal, got %v", err)
	}
	// 被报废的设备又被激活：只记异常，并再次吊销。
	if err := e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: s[1].MAC, Kind: "activated", OwnerRef: "owner-X-0009"}); err != nil {
		t.Fatal(err)
	}
	if e.state(t, s[1].Serial).State != factory.Scrapped || len(e.plat.revoked) != 2 {
		t.Fatalf("scrapped unit reactivated: %+v revoked %v", e.state(t, s[1].Serial), e.plat.revoked)
	}
	st, _ := e.svc.Stats(e.ctx)
	if st.OpenAnomalies != 1 || st.ByState[factory.Scrapped] != 1 || st.Total != 2 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestLockLostAndRecovered(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "K1000", 1)[0]
	e.toShelf(t, s.Serial)
	e.do(t, sales, s.Serial, factory.ActSell, factory.Params{OrderRef: "O-1"})

	u := e.do(t, support, s.Serial, factory.ActLock, factory.Params{LockReason: factory.LockLost, Note: "家长电话"})
	if !u.Locked || u.LockReason != factory.LockLost || !e.plat.locks[s.MAC] {
		t.Fatalf("lock: %+v locks %v", u, e.plat.locks)
	}
	if _, err := e.svc.Apply(e.ctx, support, s.Serial, factory.ActLock, factory.Params{LockReason: factory.LockLost}); !errors.Is(err, factory.ErrBadTransition) {
		t.Errorf("double lock: %v", err)
	}
	if _, err := e.svc.Apply(e.ctx, support, s.Serial, factory.ActRepair, factory.Params{Note: "x"}); !errors.Is(err, factory.ErrLocked) {
		t.Errorf("repair while locked: %v", err)
	}
	if got := factory.Available(e.state(t, s.Serial), factory.RoleSupport); len(got) != 1 || got[0] != factory.ActUnlock {
		t.Errorf("available while locked: %v", got)
	}
	// 失败时本地状态不变。
	e.plat.failLock = errors.New("down")
	if _, err := e.svc.Apply(e.ctx, support, s.Serial, factory.ActUnlock, factory.Params{}); err == nil {
		t.Fatal("unlock must fail when palpabled is unreachable")
	}
	if !e.state(t, s.Serial).Locked {
		t.Fatal("unit unlocked locally although palpabled refused")
	}
	e.plat.failLock = nil
	u = e.do(t, support, s.Serial, factory.ActUnlock, factory.Params{})
	if u.Locked || e.plat.locks[s.MAC] {
		t.Fatalf("unlock: %+v", u)
	}
	// 被盗的设备在货架上也能锁。
	s2 := e.batch(t, "K2000", 1)[0]
	e.toShelf(t, s2.Serial)
	e.do(t, support, s2.Serial, factory.ActLock, factory.Params{LockReason: factory.LockStolen})
	if st, _ := e.svc.Stats(e.ctx); st.Locked != 1 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestReturnCancelAndRepairResume(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "R1000", 1)[0]
	e.toShelf(t, s.Serial)
	e.do(t, sales, s.Serial, factory.ActSell, factory.Params{OrderRef: "O-2"})
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: s.MAC, Kind: "activated", OwnerRef: "owner-R-0001"})

	e.do(t, support, s.Serial, factory.ActReturnRequest, factory.Params{Note: "不想要了"})
	u := e.do(t, support, s.Serial, factory.ActReturnCancel, factory.Params{})
	if u.State != factory.Activated || u.ResumeState != "" {
		t.Fatalf("cancel: %+v", u)
	}
	e.do(t, support, s.Serial, factory.ActRepair, factory.Params{Note: "按键失灵"})
	// 维修期间家长解绑：修好后回到 sold，而不是 activated。
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: s.MAC, Kind: "unbound"})
	u = e.do(t, support, s.Serial, factory.ActRepairDone, factory.Params{})
	if u.State != factory.Sold || u.OwnerRef != "" {
		t.Fatalf("repair_done: %+v", u)
	}
}

func TestPlatformEventAnomalies(t *testing.T) {
	e := newEnv(t)
	us := e.batch(t, "P1000", 3)
	// 货架上的设备被激活：未登记售出。
	e.toShelf(t, us[0].Serial)
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: us[0].MAC, Kind: "activated", OwnerRef: "owner-P-0001"})
	if e.state(t, us[0].Serial).State != factory.Activated {
		t.Fatal("unrecorded sale should end up activated")
	}
	// 产线测试绑定：不算异常，状态不变。
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: us[1].MAC, Kind: "activated", OwnerRef: "tester-0001"})
	if u := e.state(t, us[1].Serial); u.State != factory.Produced || u.OwnerRef != "" {
		t.Fatalf("lab bind: %+v", u)
	}
	st, _ := e.svc.Stats(e.ctx)
	if st.OpenAnomalies != 1 {
		t.Fatalf("open anomalies = %d, want 1", st.OpenAnomalies)
	}
	evs, _ := e.svc.Events(e.ctx, factory.EventFilter{OpenAnomaly: true})
	if len(evs) != 1 {
		t.Fatalf("anomalies: %+v", evs)
	}
	if err := e.svc.ResolveAnomaly(e.ctx, viewer, evs[0].ID); !errors.Is(err, factory.ErrForbidden) {
		t.Fatalf("viewer resolve: %v", err)
	}
	if err := e.svc.ResolveAnomaly(e.ctx, sales, evs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ResolveAnomaly(e.ctx, sales, evs[0].ID); !errors.Is(err, factory.ErrNotFound) {
		t.Fatalf("resolve twice: %v", err)
	}
	if st, _ := e.svc.Stats(e.ctx); st.OpenAnomalies != 0 {
		t.Fatalf("anomaly still open: %+v", st)
	}
}

func TestPlatformEventIdempotentAndValidated(t *testing.T) {
	e := newEnv(t)
	u := e.batch(t, "I1000", 1)[0]
	e.toShelf(t, u.Serial)
	e.do(t, sales, u.Serial, factory.ActSell, factory.Params{OrderRef: "O-3"})
	ev := factory.PlatformEvent{MAC: u.MAC, Kind: "activated", OwnerRef: "owner-I-0001"}
	for i := 0; i < 3; i++ {
		if err := e.svc.OnPlatformEvent(e.ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: u.MAC, Kind: "unbound"})
	_ = e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: u.MAC, Kind: "unbound"})
	// 同一个人解绑后又绑回来：不是转让。
	_ = e.svc.OnPlatformEvent(e.ctx, ev)
	got := e.state(t, u.Serial)
	if got.TransferCount != 0 || got.State != factory.Activated {
		t.Fatalf("rebind by the same owner: %+v", got)
	}
	_, evs, _ := e.svc.Detail(e.ctx, u.Serial)
	var act, unb int
	for _, x := range evs {
		switch x.Action {
		case "activated":
			act++
		case "unbound":
			unb++
		}
	}
	if act != 2 || unb != 1 {
		t.Fatalf("activated=%d unbound=%d, want 2 and 1", act, unb)
	}
	for name, bad := range map[string]factory.PlatformEvent{
		"bad mac":   {MAC: "zz", Kind: "activated", OwnerRef: "owner-I-0001"},
		"bad kind":  {MAC: u.MAC, Kind: "exploded"},
		"bad owner": {MAC: u.MAC, Kind: "activated", OwnerRef: "x y"},
	} {
		if err := e.svc.OnPlatformEvent(e.ctx, bad); !errors.Is(err, factory.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := e.svc.OnPlatformEvent(e.ctx, factory.PlatformEvent{MAC: "aa:bb:cc:dd:ee:99", Kind: "unbound"}); !errors.Is(err, factory.ErrNotFound) {
		t.Errorf("unknown device: %v", err)
	}
}

func TestOptimisticConflict(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "V1000", 1)[0]
	stale, _ := e.db.Unit(e.ctx, s.Serial)
	e.do(t, warehouse, s.Serial, factory.ActQCPass, factory.Params{})
	stale.State = factory.QCFailed
	if err := e.db.SaveUnit(e.ctx, stale); !errors.Is(err, factory.ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	stale.Serial = "GONE"
	if err := e.db.SaveUnit(e.ctx, stale); !errors.Is(err, factory.ErrNotFound) {
		t.Fatalf("missing save: %v", err)
	}
}

func TestListAndFilters(t *testing.T) {
	e := newEnv(t)
	us := e.batch(t, "F1000", 5)
	e.batch(t, "F2000", 2)
	e.toShelf(t, us[0].Serial)

	got, total, err := e.svc.List(e.ctx, factory.Filter{State: factory.OnShelf})
	if err != nil || total != 1 || got[0].Serial != us[0].Serial {
		t.Fatalf("by state: %v %d %+v", err, total, got)
	}
	if _, total, _ = e.svc.List(e.ctx, factory.Filter{Batch: "F1000", Limit: 2}); total != 5 {
		t.Fatalf("batch total = %d", total)
	}
	if got, _, _ = e.svc.List(e.ctx, factory.Filter{Batch: "F1000", Limit: 2, Offset: 4}); len(got) != 1 {
		t.Fatalf("offset page: %d", len(got))
	}
	if got, _, _ = e.svc.List(e.ctx, factory.Filter{Q: "ppf1000-0000"}); len(got) != 5 {
		t.Fatalf("serial search: %d", len(got))
	}
	if got, _, _ = e.svc.List(e.ctx, factory.Filter{Q: us[3].MAC}); len(got) != 1 {
		t.Fatalf("mac search: %d", len(got))
	}
	if got, _, _ = e.svc.List(e.ctx, factory.Filter{Q: "%"}); len(got) != 0 {
		t.Fatalf("LIKE wildcard must be escaped, got %d", len(got))
	}
	if got, _, _ = e.svc.List(e.ctx, factory.Filter{Channel: "jd"}); len(got) != 1 {
		t.Fatalf("channel: %d", len(got))
	}
	yes := true
	if got, _, _ = e.svc.List(e.ctx, factory.Filter{Locked: &yes}); len(got) != 0 {
		t.Fatalf("locked: %d", len(got))
	}
	if _, _, err = e.svc.List(e.ctx, factory.Filter{State: "weird"}); !errors.Is(err, factory.ErrInvalid) {
		t.Fatalf("bad state: %v", err)
	}
	if _, _, err = e.svc.List(e.ctx, factory.Filter{Offset: -5, Limit: 9999}); err != nil {
		t.Fatal(err)
	}
}

func TestChannels(t *testing.T) {
	e := newEnv(t)
	for name, c := range map[string]factory.Channel{
		"bad code": {Code: "A", Name: "x", Kind: factory.ChannelOnline},
		"no name":  {Code: "shop", Name: " ", Kind: factory.ChannelOnline},
		"bad kind": {Code: "shop", Name: "店", Kind: "moon"},
	} {
		if err := e.svc.PutChannel(e.ctx, admin, c); !errors.Is(err, factory.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := e.svc.PutChannel(e.ctx, warehouse, factory.Channel{Code: "shop", Name: "店", Kind: factory.ChannelOffline}); !errors.Is(err, factory.ErrForbidden) {
		t.Errorf("non-admin: %v", err)
	}
	_ = e.svc.PutChannel(e.ctx, admin, factory.Channel{Code: "jd", Name: "京东", Kind: factory.ChannelOnline})
	cs, err := e.svc.Channels(e.ctx)
	if err != nil || len(cs) != 1 || cs[0].Name != "京东" {
		t.Fatalf("channels: %+v %v", cs, err)
	}
}

func TestAvailableByRoleAndState(t *testing.T) {
	u := factory.Unit{State: factory.OnShelf, Grade: factory.GradeNew}
	if got := factory.Available(u, factory.RoleSales); len(got) != 2 || got[0] != factory.ActUnshelf || got[1] != factory.ActSell {
		t.Errorf("sales on shelf: %v", got)
	}
	if got := factory.Available(u, factory.RoleViewer); len(got) != 0 {
		t.Errorf("viewer: %v", got)
	}
	if got := factory.Available(factory.Unit{State: factory.Scrapped}, factory.RoleAdmin); len(got) != 0 {
		t.Errorf("scrapped: %v", got)
	}
	locked := factory.Unit{State: factory.Inspecting, Locked: true}
	if got := factory.Available(locked, factory.RoleAdmin); len(got) != 2 {
		t.Errorf("locked inspecting: %v", got) // unlock + scrap
	}
	if factory.RoleAdmin.Can("teleport") || factory.RoleViewer.CanImport() || !factory.RoleWarehouse.CanImport() {
		t.Error("permission helpers wrong")
	}
	if factory.Role("x").Valid() || factory.State("x").Valid() || !factory.RoleSupport.Valid() {
		t.Error("Valid should reject unknown values")
	}
}

func TestRefurbishedUnshelfReturnsToRefurbished(t *testing.T) {
	e := newEnv(t)
	s := e.batch(t, "U1000", 1)[0]
	e.toShelf(t, s.Serial)
	e.do(t, sales, s.Serial, factory.ActSell, factory.Params{OrderRef: "O-9"})
	e.do(t, support, s.Serial, factory.ActReturnRequest, factory.Params{Note: "质量问题"})
	e.do(t, warehouse, s.Serial, factory.ActReceiveReturn, factory.Params{})
	e.do(t, warehouse, s.Serial, factory.ActInspect, factory.Params{})
	if _, err := e.svc.Apply(e.ctx, warehouse, s.Serial, factory.ActInspectPass, factory.Params{Grade: "C"}); !errors.Is(err, factory.ErrInvalid) {
		t.Fatalf("bad grade: %v", err)
	}
	e.do(t, warehouse, s.Serial, factory.ActInspectPass, factory.Params{Grade: factory.GradeB})
	e.do(t, warehouse, s.Serial, factory.ActShip, factory.Params{Channel: "jd"})
	e.do(t, sales, s.Serial, factory.ActReceive, factory.Params{})
	if u := e.do(t, sales, s.Serial, factory.ActUnshelf, factory.Params{}); u.State != factory.Refurbished {
		t.Fatalf("unshelf refurbished: %+v", u)
	}
}
