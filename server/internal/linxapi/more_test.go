package linxapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRemainingEndpoints(t *testing.T) {
	calls := make(chan call, 8)
	c := server(t, map[string]string{
		"POST /v1/memories/m1/identifiers/02:00:00:00:00:01/nodes": `{"code":0,"data":{"id":"node_1"}}`,
		"POST /v1/speakers/spk1/voiceprint":                        `{"code":0,"data":{"voiceprint_id":"vp"}}`,
		"PUT /v1/agents/a1/speaker-config":                         `{"code":0,"data":{}}`,
		"PUT /xiaozhi/agent/a1":                                    `{"code":0,"data":null}`,
		"POST /v1/memories":                                        `{"code":0,"data":{"id":"mem_new"}}`,
	}, calls)
	ctx := context.Background()
	if err := c.AddMemoryNode(ctx, "m1", "02:00:00:00:00:01", "喜欢恐龙"); err != nil {
		t.Fatal(err)
	}
	if got := <-calls; got.body["content"] != "喜欢恐龙" {
		t.Fatalf("node body = %v", got.body)
	}
	if err := c.EnrollVoiceprint(ctx, "spk1", "https://x/a.wav"); err != nil {
		t.Fatal(err)
	}
	if got := <-calls; got.body["audio_url"] != "https://x/a.wav" {
		t.Fatalf("voiceprint body = %v", got.body)
	}
	if err := c.BindSpeakers(ctx, "a1", nil); err != nil {
		t.Fatal(err)
	}
	if got := <-calls; got.body["speaker_ids"] == nil {
		t.Fatalf("nil speakers must be sent as an empty list: %v", got.body)
	}
	if err := c.SetOwnerOnly(ctx, "a1", true); err != nil {
		t.Fatal(err)
	}
	<-calls
	if m, err := c.EnableMemory(ctx, "a1"); err != nil || m != "mem_new" {
		t.Fatalf("EnableMemory new = %q %v", m, err)
	}
}

func TestPreregisterSuccessAndOtherFailure(t *testing.T) {
	c := server(t, map[string]string{"POST /xiaozhi/device/preregister": `{"code":0,"data":{"success_count":1,"failed":[]}}`}, nil)
	if err := c.PreregisterDevice(context.Background(), "a1", "02:00:00:00:00:01"); err != nil {
		t.Fatal(err)
	}
	c = server(t, map[string]string{"POST /xiaozhi/device/preregister": `{"code":0,"data":{"success_count":0,"failed":[{"value":"02:00:00:00:00:01","reason":"格式错误"}]}}`}, nil)
	var apiErr *APIError
	if err := c.PreregisterDevice(context.Background(), "a1", "02:00:00:00:00:01"); !errors.As(err, &apiErr) || !strings.Contains(apiErr.Error(), "格式错误") {
		t.Fatalf("other failure: %v", err)
	}
}

func TestEnvelopeEdgeCases(t *testing.T) {
	ctx := context.Background()
	// 业务码 404 → ErrNotFound；用 message 字段的错误信息；记忆为空 → ErrNotFound。
	c := server(t, map[string]string{
		"GET /v1/agents/a1/memory":       `{"code":0,"data":null}`,
		"POST /v1/memories":              `{"code":614,"message":"exists"}`,
		"GET /xiaozhi/agent/list":        `{"code":500,"message":"内部错误"}`,
		"GET /xiaozhi/agent/a1/sessions": `not json`,
	}, nil)
	if _, err := c.EnableMemory(ctx, "a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty memory: %v", err)
	}
	var apiErr *APIError
	if _, _, err := c.FindAgentByName(ctx, "x"); !errors.As(err, &apiErr) || apiErr.Msg != "内部错误" {
		t.Fatalf("message field: %v", err)
	}
	if _, err := c.Sessions(ctx, "a1", 1, 10); err == nil {
		t.Fatal("bad json must error")
	}
	if _, err := c.SessionMessages(ctx, "a1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404: %v", err)
	}
	if _, err := c.UpsertAttention(ctx, "nope", "n", "d"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attention list 404: %v", err)
	}

	// 默认地址与连不上时的错误。
	if New("", "t").base != DefaultBase {
		t.Fatal("default base")
	}
	dead := New("http://127.0.0.1:1", "t")
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := dead.CreateSpeaker(cctx, "x"); err == nil {
		t.Fatal("unreachable server must error")
	}
}

func TestParseTimeAndHelpers(t *testing.T) {
	if parseTime("2026-10-23T09:00:00.123456").IsZero() || !parseTime("garbage").IsZero() {
		t.Fatal("parseTime layouts")
	}
	if firstNonEmpty("", "") != "" || firstNonEmpty("", "b") != "b" {
		t.Fatal("firstNonEmpty")
	}
	if (&APIError{Path: "/p", Code: 1, Msg: "m"}).Error() == "" {
		t.Fatal("APIError text")
	}
}

// Fake 要与 HTTPClient 的约定一致，上层测试才可信。
func TestFakeBehaves(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	agent, _ := f.CreateAgent(ctx, AgentSpec{Name: "可感·小龙"})
	if id, ok, _ := f.FindAgentByName(ctx, "可感·小龙"); !ok || id != agent {
		t.Fatal("FindAgentByName")
	}
	if _, ok, _ := f.FindAgentByName(ctx, "无"); ok {
		t.Fatal("missing agent found")
	}
	f.FailCreate = errors.New("down")
	if _, err := f.CreateAgent(ctx, AgentSpec{}); err == nil {
		t.Fatal("FailCreate")
	}
	mac := "02:00:00:00:00:01"
	if err := f.SwitchAgent(ctx, mac, agent, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("switch unregistered")
	}
	_ = f.PreregisterDevice(ctx, agent, mac)
	if !errors.Is(f.PreregisterDevice(ctx, agent, mac), ErrAlreadyRegistered) || f.AgentOf(mac) != agent {
		t.Fatal("preregister twice")
	}
	_ = f.SwitchAgent(ctx, mac, "other", true)
	mem, _ := f.EnableMemory(ctx, agent)
	if again, _ := f.EnableMemory(ctx, agent); again != mem {
		t.Fatal("memory not stable")
	}
	att, _ := f.UpsertAttention(ctx, mem, "n", "d")
	if again, _ := f.UpsertAttention(ctx, mem, "n", "d"); again != att {
		t.Fatal("attention not stable")
	}
	_ = f.SetAttentionValue(ctx, mem, mac, att, "v")
	_ = f.AddMemoryNode(ctx, mem, mac, "c")
	spk, _ := f.CreateSpeaker(ctx, "x")
	_ = f.EnrollVoiceprint(ctx, spk, "u")
	_ = f.BindSpeakers(ctx, agent, []string{spk})
	_ = f.SetOwnerOnly(ctx, agent, true)
	if s, _ := f.RecentVoiceSamples(ctx, agent); len(s) != 0 {
		t.Fatal("no samples yet")
	}
	if ss, _ := f.Sessions(ctx, agent, 1, 10); len(ss) != 0 {
		t.Fatal("no sessions yet")
	}
	f.Messages[agent] = []Message{{Role: "user", Text: "hi"}}
	if ss, _ := f.Sessions(ctx, agent, 1, 10); len(ss) != 1 {
		t.Fatal("one session")
	}
	if ms, _ := f.SessionMessages(ctx, agent, "s1"); len(ms) != 1 {
		t.Fatal("messages")
	}
	if _, err := f.SessionMessages(ctx, agent, "s2"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown session")
	}
}
