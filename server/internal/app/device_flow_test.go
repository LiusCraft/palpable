package app

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/config"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/voice"
)

// 演示预置：新设备上线后自动绑定家长并带上声纹同意。
func TestDemoPresetBindsGuardian(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory()
	a, err := newApp(ctx, deps{store: s, demo: config.Demo{Guardian: "g1", GuardianName: "妈妈", VoiceConsent: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.catalog.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	connect(t, srv, mac)
	waitFor(t, func() bool {
		p, err := a.devices.Profile(ctx, mac)
		return err == nil && p.Guardian == "g1" && p.VoiceConsent
	})

	// 已绑定的设备不再改动。
	p, _ := a.devices.Profile(ctx, mac)
	if got := a.applyDemo(ctx, p); got.Guardian != "g1" {
		t.Fatalf("applyDemo = %+v", got)
	}
	if persona(device.Profile{Stage: device.StageFresh, VoiceConsent: true}).VoiceStep != device.VoiceStep(true) {
		t.Fatal("voice step only during onboarding with consent")
	}
	for _, p := range []device.Profile{{Stage: device.StageReady, VoiceConsent: true}, {Stage: device.StageFresh}} {
		if persona(p).VoiceStep != device.VoiceStep(false) {
			t.Fatalf("no voice step for %+v", p)
		}
	}
}

func TestSinkAndNoAI(t *testing.T) {
	a := mustApp(t, nil)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	ws := connect(t, srv, mac)

	if err := a.Send(id.Device(mac), &protocol.Sound{Type: protocol.TypeSound, Name: "busy"}); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, ws); got == "" {
		t.Fatal("Send did not reach the device")
	}
	if err := a.SendAudio(id.Device(mac), []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	if mt, _, err := ws.ReadMessage(); err != nil || mt != websocket.BinaryMessage {
		t.Fatalf("SendAudio: %v %v", mt, err)
	}
	// 不在线的参与者静默丢弃。
	if a.Send("dev:02:00:00:00:00:99", nil) != nil || a.SendAudio("dev:02:00:00:00:00:99", nil) != nil {
		t.Fatal("offline sink must not error")
	}

	var n noAI
	n.Forward(mac, nil)
	n.Interrupt(mac)
	n.Listen(mac, true)
	if n.Available(mac) {
		t.Fatal("noAI is never available")
	}
	a.onVoice(voice.Event{Kind: voice.BurstStarted, Burst: voice.Burst{Speaker: id.Device(mac)}})
	a.onVoice(voice.Event{Kind: voice.BurstEnded, Burst: voice.Burst{Speaker: id.Device(mac)}})
}
