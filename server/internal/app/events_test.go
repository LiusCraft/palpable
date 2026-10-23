package app

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/relation"
)

// 领域事件经 Start 订阅后翻译成下行消息：好友确认 / 解除、频道离开 / 切换。
func TestEventLoopsTranslateAllKinds(t *testing.T) {
	a := mustApp(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Start(ctx)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	const other id.DeviceID = "02:00:00:00:00:02"
	ws := connect(t, srv, mac)
	connect(t, srv, other)

	f := relation.Friendship{ID: "f1", A: mac, B: other}
	a.onRelation(ctx, relation.Event{Kind: relation.Confirmed, Friendship: f})
	if got := readText(t, ws); !strings.Contains(got, `"event":"confirmed"`) {
		t.Fatalf("confirmed = %s", got)
	}
	readText(t, ws) // haptic
	a.onRelation(ctx, relation.Event{Kind: relation.Removed, Friendship: f})
	if got := readText(t, ws); !strings.Contains(got, `"event":"removed"`) {
		t.Fatalf("removed = %s", got)
	}

	// 经设备上行触发：进公园中心、加入、建群（切到群）、离开群（回到公园）。
	_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"location","lat":31.2270,"lng":121.4730}`))
	_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"proximity","peers":[{"device_id":"02:00:00:00:00:02","strength":0.9}]}`))
	waitFor(t, func() bool { _, ok := a.zones.Current(mac); return ok })
	if err := a.zones.JoinCurrent(ctx, mac); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, ws); !strings.Contains(got, `"event":"joined"`) || !strings.Contains(got, "#4caf50") {
		t.Fatalf("joined = %s", got)
	}
	readText(t, ws) // sound
	code, err := a.chans.CreateGroup(ctx, mac, channel.TTLToday)
	if err != nil {
		t.Fatal(err)
	}
	readText(t, ws)
	readText(t, ws)
	info, _ := a.chans.Current(ctx, mac)
	if info.Channel.Code != code || channelColor(info.Channel) == "#4caf50" {
		t.Fatalf("group color = %s", channelColor(info.Channel))
	}
	if err := a.chans.Leave(ctx, mac, info.Channel.ID); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, ws); !strings.Contains(got, `"event":"left"`) {
		t.Fatalf("left = %s", got)
	}
	readText(t, ws) // sound left
	if got := readText(t, ws); !strings.Contains(got, `"event":"switched"`) {
		t.Fatalf("switched = %s", got)
	}

	// 发给不在线的设备静默丢弃。
	a.send("02:00:00:00:00:99", nil)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
