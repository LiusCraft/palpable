package device

import (
	"context"
	"errors"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

func TestBindRenameDeleteAndUnknown(t *testing.T) {
	devs, cat, fake, s := setup(t)
	ctx := context.Background()
	if _, err := devs.Profile(ctx, mac); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown profile: %v", err)
	}
	if err := devs.Bind(ctx, mac, "g1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bind unknown: %v", err)
	}
	if err := devs.SetNames(ctx, mac, "小龙", "朵朵"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("name unknown: %v", err)
	}
	if _, err := devs.Handoff(ctx, mac); !errors.Is(err, ErrNotFound) {
		t.Fatalf("handoff unknown: %v", err)
	}
	if err := devs.EnrollVoice(ctx, mac); !errors.Is(err, ErrNotFound) {
		t.Fatalf("enroll unknown: %v", err)
	}

	if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "cat"}); err != nil {
		t.Fatal(err)
	}
	if err := devs.Bind(ctx, mac, "g1"); err != nil {
		t.Fatal(err)
	}
	// 引导期换皮套只记下皮套，交棒时按当时的皮套选风格。
	if p, _ := devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "rabbit"}); p.Skin != "rabbit" || p.StyleID != "welcome" {
		t.Fatalf("fresh skin change = %+v", p)
	}
	_ = devs.SetNames(ctx, mac, "兔兔", "乐乐")
	<-devs.Subscribe()
	p, err := devs.Handoff(ctx, mac)
	if err != nil || p.StyleID != "rabbit" || p.Guardian != "g1" {
		t.Fatalf("handoff = %+v %v", p, err)
	}
	// ready 后改名：不再发 Named，但写入新名字与记忆。
	if err := devs.SetNames(ctx, mac, "跳跳", "乐乐"); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-devs.Subscribe():
		t.Fatalf("rename must not emit Named: %+v", e)
	default:
	}
	mem := fake.Memories[cat.ForSkin("rabbit").AgentID]
	if fake.Values[mem+"/"+mac+"/"+fake.Attentions[mem+"/"+attDollName]] != "跳跳" {
		t.Fatal("rename not written to memory")
	}
	// 引导期已结束：不再登记声纹。
	_ = devs.SetVoiceConsent(ctx, mac, true)
	if err := devs.EnrollVoice(ctx, mac); !errors.Is(err, ErrNotOnboarding) {
		t.Fatalf("enroll after onboarding: %v", err)
	}
	if err := devs.Delete(ctx, mac); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Devices().Get(ctx, id.DeviceID(mac)); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("device not deleted")
	}
}

func TestProvisionAndHandoffFailures(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	fake := linxapi.NewFake()
	cat := NewCatalog(s.Styles(), fake)
	devs := New(s.Devices(), cat, fake)
	// 目录还没就绪：拒绝开通。
	if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac}); err == nil {
		t.Fatal("must fail before catalog is ready")
	}
	fake.FailCreate = errors.New("linx down")
	if err := cat.EnsureAll(ctx); err == nil {
		t.Fatal("EnsureAll must surface create errors")
	}
	fake.FailCreate = nil
	if err := cat.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac}); err != nil {
		t.Fatal(err)
	}
	_ = devs.SetNames(ctx, mac, "小熊", "朵朵")
	<-devs.Subscribe()
	// 灵矽丢了设备：交棒失败，阶段保持 named，下次重试。
	delete(fake.Devices, mac)
	if _, err := devs.Handoff(ctx, mac); err == nil {
		t.Fatal("handoff must fail when switch fails")
	}
	if p, _ := devs.Profile(ctx, mac); p.Stage != StageNamed {
		t.Fatalf("stage after failed handoff = %v", p.Stage)
	}
}
