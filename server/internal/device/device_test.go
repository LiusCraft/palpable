package device

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const mac = "02:00:00:00:00:0a"

func setup(t *testing.T) (*Devices, *Catalog, *linxapi.Fake, store.Store) {
	t.Helper()
	s := store.NewMemory()
	fake := linxapi.NewFake()
	cat := NewCatalog(s.Styles(), fake)
	if err := cat.EnsureAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	return New(s.Devices(), cat, fake), cat, fake, s
}

func TestEnsureAllIsIdempotentAndPrompted(t *testing.T) {
	_, cat, fake, s := setup(t)
	if len(fake.Agents) != 1+len(styles) {
		t.Fatalf("agents = %d", len(fake.Agents))
	}
	// 再启动一次：从 style 表读到，不再建智能体。
	cat2 := NewCatalog(s.Styles(), fake)
	if err := cat2.EnsureAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fake.Agents) != 1+len(styles) {
		t.Fatalf("agents after restart = %d", len(fake.Agents))
	}
	if cat.ForSkin("nope").ID != defaultSkin || cat.ForSkin("dragon").AgentID == "" || cat.Onboarding().AgentID == "" {
		t.Fatal("catalog lookups")
	}
	for _, a := range fake.Agents {
		if !strings.Contains(a.SystemPrompt, "家庭住址") {
			t.Fatalf("prompt for %s lacks safety rules", a.Name)
		}
	}
}

func TestOnboardingNamingAndHandoff(t *testing.T) {
	devs, cat, fake, _ := setup(t)
	ctx := context.Background()

	p, err := devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "dragon"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Stage != StageFresh || p.StyleID != "welcome" || fake.AgentOf(mac) != cat.Onboarding().AgentID {
		t.Fatalf("fresh profile = %+v, bound to %s", p, fake.AgentOf(mac))
	}
	again, err := devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "dragon"})
	if err != nil || again.Stage != StageFresh || len(fake.Devices) != 1 {
		t.Fatalf("second Ensure must not re-provision: %+v %v", again, err)
	}

	if _, err := devs.Handoff(ctx, mac); !errors.Is(err, ErrNotNamed) {
		t.Fatalf("handoff before naming: %v", err)
	}
	if err := devs.SetNames(ctx, mac, "", "朵朵"); !errors.Is(err, ErrBadName) {
		t.Fatalf("empty name: %v", err)
	}
	if err := devs.SetNames(ctx, mac, "小龙", "朵朵"); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-devs.Subscribe():
		if e.Device != mac || e.DollName != "小龙" {
			t.Fatalf("named = %+v", e)
		}
	default:
		t.Fatal("no Named event")
	}

	// named 状态重新上线：仍是引导智能体，不重新起名。
	p, _ = devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "dragon"})
	if p.Stage != StageNamed {
		t.Fatalf("stage = %v", p.Stage)
	}

	p, err = devs.Handoff(ctx, mac)
	if err != nil {
		t.Fatal(err)
	}
	dragon := cat.ForSkin("dragon")
	if p.Stage != StageReady || p.AgentID != dragon.AgentID || fake.AgentOf(mac) != dragon.AgentID {
		t.Fatalf("after handoff %+v bound=%s", p, fake.AgentOf(mac))
	}
	mem := fake.Memories[dragon.AgentID]
	if fake.Values[mem+"/"+mac+"/"+fake.Attentions[mem+"/"+attDollName]] != "小龙" {
		t.Fatalf("memory focus not written: %v", fake.Values)
	}
	switches := len(fake.SwitchCalls)
	if _, err := devs.Handoff(ctx, mac); err != nil || len(fake.SwitchCalls) != switches {
		t.Fatal("second handoff must be a no-op")
	}

	// 换皮套：切到新风格。
	p, err = devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "rabbit"})
	if err != nil || p.StyleID != "rabbit" || fake.AgentOf(mac) != cat.ForSkin("rabbit").AgentID {
		t.Fatalf("skin switch: %+v %v", p, err)
	}
}

func TestProvisionFailureAndAlreadyRegistered(t *testing.T) {
	devs, cat, fake, _ := setup(t)
	ctx := context.Background()
	fake.FailPrereg = errors.New("linx down")
	if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac}); err == nil {
		t.Fatal("expected provisioning error")
	}
	if _, err := devs.Profile(ctx, mac); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed provisioning must not leave a profile")
	}

	// 灵矽侧已有该 MAC（本地库被清空）：按已开通处理并核对绑定。
	fake.FailPrereg = nil
	fake.Devices[mac] = "some-old-agent"
	p, err := devs.Ensure(ctx, DeviceInfo{MAC: mac})
	if err != nil || p.Stage != StageFresh || fake.AgentOf(mac) != cat.Onboarding().AgentID {
		t.Fatalf("already registered: %+v %v bound=%s", p, err, fake.AgentOf(mac))
	}
}

func TestEnrollVoiceNeedsConsentAndOwnSample(t *testing.T) {
	devs, cat, fake, _ := setup(t)
	ctx := context.Background()
	if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac}); err != nil {
		t.Fatal(err)
	}
	if err := devs.EnrollVoice(ctx, mac); !errors.Is(err, ErrNoConsent) {
		t.Fatalf("without consent: %v", err)
	}
	if err := devs.SetVoiceConsent(ctx, mac, true); err != nil {
		t.Fatal(err)
	}
	welcome := cat.Onboarding().AgentID
	fake.Samples[welcome] = []linxapi.VoiceSample{{AudioURL: "other.wav", MAC: "02:00:00:00:00:ff", At: time.Now()}}
	if err := devs.EnrollVoice(ctx, mac); !errors.Is(err, ErrNoSample) {
		t.Fatalf("another device's sample must not be used: %v", err)
	}
	fake.Samples[welcome] = append(fake.Samples[welcome], linxapi.VoiceSample{AudioURL: "mine.wav", MAC: mac, At: time.Now()})
	if err := devs.EnrollVoice(ctx, mac); err != nil {
		t.Fatal(err)
	}
	p, _ := devs.Profile(ctx, mac)
	if p.SpeakerID == "" || fake.Speakers[p.SpeakerID] != "mine.wav" || p.Stage != StageFresh {
		t.Fatalf("enrolled profile = %+v", p)
	}
}
