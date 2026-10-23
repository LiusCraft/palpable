package history

import (
	"context"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	me    id.DeviceID = "02:00:00:00:00:0a"
	other id.DeviceID = "02:00:00:00:00:0b"
)

func TestTodayDigest(t *testing.T) {
	s := store.NewMemory()
	fake := linxapi.NewFake()
	h := New(s.History(), fake, func(context.Context, id.DeviceID) (string, error) { return "agent-dragon", nil })
	now := time.Date(2026, 10, 23, 15, 0, 0, 0, h.loc)
	h.now = func() time.Time { return now }
	ctx := context.Background()

	yesterday := now.AddDate(0, 0, -1)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(h.RecordMet(ctx, me, MetFriend{Friend: other, Name: "兔兔", Channel: "zone:park", At: yesterday}))
	must(h.RecordMet(ctx, me, MetFriend{Friend: other, Name: "兔兔", Channel: "zone:park"}))
	must(h.RecordMet(ctx, me, MetFriend{Friend: other, Name: "兔兔", Channel: "zone:park"})) // 重复只算一次
	must(h.RecordShared(ctx, me, SharedScene{From: me, Name: "小龙", Text: "一棵很高的大树", Channel: "zone:park"}))
	must(h.RecordMet(ctx, other, MetFriend{Friend: me, Name: "小龙"}))

	fake.Messages["agent-dragon"] = []linxapi.Message{
		{Role: "user", Text: "你好", MAC: string(me), At: now.Add(-time.Hour)},
		{Role: "assistant", Text: "你好呀", MAC: string(me), At: now.Add(-time.Hour + time.Second)},
		{Role: "user", Text: "别的小朋友的话", MAC: string(other), At: now.Add(-time.Hour)},
		{Role: "user", Text: "没有 MAC 的话", At: now.Add(-time.Hour)},
		{Role: "tool", Text: "工具调用", MAC: string(me), At: now.Add(-time.Hour)},
	}

	dg, err := h.Today(ctx, me)
	must(err)
	if len(dg.Met) != 1 || dg.Met[0].Name != "兔兔" {
		t.Fatalf("met = %+v", dg.Met)
	}
	if len(dg.Shared) != 1 || dg.Shared[0].Text != "一棵很高的大树" {
		t.Fatalf("shared = %+v", dg.Shared)
	}
	if len(dg.Chats) != 2 || dg.Chats[0].Text != "你好" || dg.Chats[1].Role != "assistant" {
		t.Fatalf("chats must only include this device's lines: %+v", dg.Chats)
	}
}
