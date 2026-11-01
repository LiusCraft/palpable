package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/linxapi"
)

// enrolled 建一台已登记声纹的设备，返回它的说话人与当前智能体。
func enrolled(t *testing.T) (*Devices, *linxapi.Fake, string, string) {
	t.Helper()
	devs, cat, fake, _ := setup(t)
	ctx := context.Background()
	if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac}); err != nil {
		t.Fatal(err)
	}
	if err := devs.GrantVoice(ctx, mac); err != nil {
		t.Fatal(err)
	}
	welcome := cat.Onboarding().AgentID
	fake.Samples[welcome] = []linxapi.VoiceSample{{AudioURL: "mine.wav", MAC: mac, At: time.Now()}}
	if err := devs.EnrollVoice(ctx, mac); err != nil {
		t.Fatal(err)
	}
	p, _ := devs.Profile(ctx, mac)
	return devs, fake, p.SpeakerID, p.AgentID
}

func TestRevokeVoiceDeletesSpeakerEvenWhenBound(t *testing.T) {
	devs, fake, speaker, agent := enrolled(t)
	ctx := context.Background()
	// 同一智能体上还有别的孩子的说话人：只解绑这一个。
	fake.Bound[agent] = []string{"spk-other", speaker}
	if err := devs.RevokeVoice(ctx, mac); err != nil {
		t.Fatal(err)
	}
	p, _ := devs.Profile(ctx, mac)
	if p.VoiceConsent || p.SpeakerID != "" {
		t.Fatalf("profile after revoke = %+v", p)
	}
	if _, ok := fake.Speakers[speaker]; ok || len(fake.DeletedSpeakers) != 1 || fake.DeletedSpeakers[0] != speaker {
		t.Fatalf("speaker not deleted: speakers=%v deleted=%v", fake.Speakers, fake.DeletedSpeakers)
	}
	if b := fake.Bound[agent]; len(b) != 1 || b[0] != "spk-other" {
		t.Fatalf("other speakers must stay bound: %v", b)
	}
	// 再撤回一次没有说话人可删，也成功。
	if err := devs.RevokeVoice(ctx, mac); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeVoiceKeepsProfileWhenLinxFails(t *testing.T) {
	devs, fake, speaker, _ := enrolled(t)
	ctx := context.Background()
	fake.FailDelete = errors.New("linx down")
	if err := devs.RevokeVoice(ctx, mac); err == nil {
		t.Fatal("revoke succeeded although linx failed")
	}
	if p, _ := devs.Profile(ctx, mac); p.SpeakerID != speaker || !p.VoiceConsent {
		t.Fatalf("profile changed on failure: %+v", p)
	}
	fake.FailDelete = linxapi.ErrNotFound // 平台那边已经没有了：算删除成功
	if err := devs.RevokeVoice(ctx, mac); err != nil {
		t.Fatal(err)
	}
}

func TestGrantVoiceOnlyBeforeNaming(t *testing.T) {
	devs, _, _, _ := setup(t)
	ctx := context.Background()
	if err := devs.GrantVoice(ctx, mac); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown device: %v", err)
	}
	if _, err := devs.Ensure(ctx, DeviceInfo{MAC: mac, Skin: "dragon"}); err != nil {
		t.Fatal(err)
	}
	if err := devs.SetNames(ctx, mac, "小龙", "朵朵"); err != nil {
		t.Fatal(err)
	}
	if err := devs.GrantVoice(ctx, mac); !errors.Is(err, ErrNotOnboarding) {
		t.Fatalf("grant after naming: %v", err)
	}
}
