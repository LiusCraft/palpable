package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/bridge/linx"
	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/voice"
)

// 主人点名 → 小伙伴下一段语音作为 agt:<mac> 播给频道里的好友；对方家长屏蔽后听不到。
func TestAgentSpeaksInChannel(t *testing.T) {
	const friend id.DeviceID = "02:00:00:00:00:02"
	ctx := context.Background()
	s := store.NewMemory()
	if err := s.Friendships().Create(ctx, store.Friendship{ID: "f1", A: mac, B: friend, Status: int(relation.StatusConfirmed), CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	a, err := newApp(ctx, deps{store: s})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.catalog.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	a.chans.DefineZone("zone:park", "公园")
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	owner := connect(t, srv, mac)
	wsFriend := connect(t, srv, friend)
	waitFor(t, func() bool {
		_, e1 := a.devices.Profile(ctx, mac)
		_, e2 := a.devices.Profile(ctx, friend)
		return e1 == nil && e2 == nil
	})

	speak := func() (string, bool) {
		r, err := a.mcp.Call(ctx, mac, "self.channel.speak", json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		return r.Text, r.IsError
	}
	if _, isErr := speak(); !isErr {
		t.Fatal("not in channel yet: should fail")
	}
	for _, d := range []id.DeviceID{mac, friend} {
		if err := a.chans.JoinZone(ctx, d, "zone:park"); err != nil {
			t.Fatal(err)
		}
	}

	if _, isErr := speak(); isErr {
		t.Fatal("speak should be armed")
	}
	tap := a.tap(mustSession(t, a, mac))
	a.voice.OnAIState(mac, voice.AISpeaking)
	_ = tap.SendAudio([]byte{42})
	if got := readUntil(t, wsFriend, "speaker.state"); !strings.Contains(got, `"source":"agent"`) || !strings.Contains(got, string(mac)) {
		t.Fatalf("speaker.state = %s", got)
	}
	_ = wsFriend.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		mt, data, err := wsFriend.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if mt == websocket.BinaryMessage {
			if data[0] != 42 {
				t.Fatalf("frame %v", data)
			}
			break
		}
	}
	a.voice.OnAIState(mac, voice.AIIdle)
	// 主人自己不经频道再听一遍（它直接听自己设备上的 AI 音频）。
	_ = owner.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	for {
		mt, data, err := owner.ReadMessage()
		if err != nil {
			break
		}
		if mt == websocket.TextMessage && strings.Contains(string(data), `"source":"agent"`) {
			t.Fatalf("owner got agent relay: %s", data)
		}
	}

	// 对方家长屏蔽；自家家长关闭。
	if err := a.devices.BlockAgent(ctx, friend, mac, true); err != nil {
		t.Fatal(err)
	}
	if err := a.AgentSpeak(ctx, mac); err != nil {
		t.Fatal(err)
	}
	if err := a.devices.SetAgentInChannel(ctx, mac, false); err != nil {
		t.Fatal(err)
	}
	if err := a.AgentSpeak(ctx, mac); !errors.Is(err, device.ErrAgentOff) {
		t.Fatalf("agent off: %v", err)
	}
	if text, isErr := speak(); !isErr || !strings.Contains(text, "爸爸妈妈") {
		t.Fatalf("speak with agent off = %q", text)
	}
	if err := a.AgentSpeak(ctx, "02:00:00:00:00:77"); !errors.Is(err, channel.ErrNotInChannel) {
		t.Fatalf("unknown owner: %v", err)
	}
}

func mustSession(t *testing.T, a *App, d id.DeviceID) linx.DeviceLink {
	t.Helper()
	s, ok := a.gw.Session(d)
	if !ok {
		t.Fatal("no session")
	}
	return s
}
