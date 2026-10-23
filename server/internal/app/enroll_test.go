package app

import (
	"context"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/device"
)

// 有声音登记时，交棒要等登记尝试结束；没有同意或超时则直接交棒。
func TestHandoffWaitsForEnrollAttempt(t *testing.T) {
	a := mustApp(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Start(ctx)
	if _, err := a.devices.Ensure(ctx, device.DeviceInfo{MAC: string(mac)}); err != nil {
		t.Fatal(err)
	}

	// 没有同意：不等。
	start := time.Now()
	a.waitEnroll(ctx, mac)
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("must not wait without consent")
	}

	if err := a.devices.SetVoiceConsent(ctx, mac, true); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		a.waitEnroll(ctx, mac)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("handed off before enrollment was attempted")
	case <-time.After(150 * time.Millisecond):
	}
	_ = a.devices.EnrollVoice(ctx, mac) // 没有样本，失败也算尝试过
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enroll attempt did not release the handoff")
	}

	// 一直没尝试：超时后照常交棒。
	old := enrollWait
	enrollWait = 50 * time.Millisecond
	defer func() { enrollWait = old }()
	a.waitEnroll(ctx, mac)
}
