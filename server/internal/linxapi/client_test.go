package linxapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type call struct {
	method, path string
	body         map[string]any
}

// server 返回固定响应；handler 按 "METHOD path" 取响应体。
func server(t *testing.T, responses map[string]string, calls chan<- call) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			_, _ = io.WriteString(w, `{"code":401,"msg":"未登录","data":[]}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if calls != nil {
			calls <- call{r.Method, r.URL.RequestURI(), body}
		}
		resp, ok := responses[r.Method+" "+r.URL.Path]
		if !ok {
			_, _ = io.WriteString(w, `{"code":404,"msg":"nope"}`)
			return
		}
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok")
}

func TestCreateAndFindAgent(t *testing.T) {
	calls := make(chan call, 4)
	c := server(t, map[string]string{
		"POST /xiaozhi/agent":     `{"code":0,"msg":"success","data":"6f99512f6b55429f8d2e3ddd0bcbe23f"}`,
		"GET /xiaozhi/agent/list": `{"code":0,"data":[{"id":"a1","agentName":"可感·小龙"},{"id":"a2","agentName":"别的"}],"nextCursor":null}`,
	}, calls)
	ctx := context.Background()
	id, err := c.CreateAgent(ctx, AgentSpec{Name: "可感·小龙", AssistantName: "小龙", SystemPrompt: "p", MemoryModel: MemoryLongTerm, VoiceID: "v1"})
	if err != nil || id != "6f99512f6b55429f8d2e3ddd0bcbe23f" {
		t.Fatalf("CreateAgent = %q %v", id, err)
	}
	got := <-calls
	if got.body["agentName"] != "可感·小龙" || got.body["memModelId"] != MemoryLongTerm || got.body["ttsVoiceId"] != "v1" {
		t.Fatalf("body = %v", got.body)
	}
	if id, ok, err := c.FindAgentByName(ctx, "可感·小龙"); err != nil || !ok || id != "a1" {
		t.Fatalf("FindAgentByName = %q %v %v", id, ok, err)
	}
	if _, ok, _ := c.FindAgentByName(ctx, "没有"); ok {
		t.Fatal("found missing agent")
	}
}

func TestPreregisterConflictAndSwitch(t *testing.T) {
	calls := make(chan call, 4)
	c := server(t, map[string]string{
		"POST /xiaozhi/device/preregister":           `{"code":0,"data":{"success_count":0,"failed":[{"value":"02:00:00:00:00:01","reason":"已存在"}]}}`,
		"PUT /v1/devices/02:00:00:00:00:01/agent/a1": `{"code":0,"reqid":"r","data":{}}`,
	}, calls)
	ctx := context.Background()
	if err := c.PreregisterDevice(ctx, "a1", "02:00:00:00:00:01"); !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("preregister existing: %v", err)
	}
	got := <-calls
	if macs, _ := got.body["mac_addresses"].([]any); len(macs) != 1 || got.body["agent_id"] != "a1" {
		t.Fatalf("preregister body = %v", got.body)
	}
	if err := c.SwitchAgent(ctx, "02:00:00:00:00:01", "a1", false); err != nil {
		t.Fatal(err)
	}
	if got := <-calls; got.body["disable_chat_history_migration"] != true {
		t.Fatalf("switch body = %v", got.body)
	}
}

func TestActivateDevice(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		if r.Header.Get("Device-Id") == "02:00:00:00:00:02" {
			_, _ = io.WriteString(w, `{"activation":{"code":"123456"},"websocket":{"url":"wss://x"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"websocket":{"url":"wss://x","token":"t"},"server_time":{"timestamp":1}}`)
	}))
	defer srv.Close()
	c := New("", "tok").WithOTA(srv.URL)
	if err := c.ActivateDevice(context.Background(), "02:00:00:00:00:01"); err != nil {
		t.Fatal(err)
	}
	if got.Get("Device-Id") != "02:00:00:00:00:01" || got.Get("Client-Id") != deviceClientID("02:00:00:00:00:01") {
		t.Fatalf("headers = %v", got)
	}
	if err := c.ActivateDevice(context.Background(), "02:00:00:00:00:02"); !errors.Is(err, ErrActivationRequired) {
		t.Fatalf("activation required: %v", err)
	}
}

func TestMemoryFlow(t *testing.T) {
	c := server(t, map[string]string{
		"POST /v1/memories":                 `{"code":614,"msg":"already exists"}`,
		"GET /v1/agents/a1/memory":          `{"code":0,"data":{"id":"mem1","agent_id":"a1"}}`,
		"GET /v1/memories/mem1/attentions":  `{"code":0,"data":[{"id":"att_kid","name":"孩子的称呼"}]}`,
		"POST /v1/memories/mem1/attentions": `{"code":0,"data":{"id":"att_new"}}`,
		"POST /v1/memories/mem1/identifiers/02:00:00:00:00:01/attention_values/att_kid": `{"code":0,"data":{}}`,
	}, nil)
	ctx := context.Background()
	mem, err := c.EnableMemory(ctx, "a1")
	if err != nil || mem != "mem1" {
		t.Fatalf("EnableMemory = %q %v", mem, err)
	}
	if a, err := c.UpsertAttention(ctx, mem, "孩子的称呼", "d"); err != nil || a != "att_kid" {
		t.Fatalf("existing attention = %q %v", a, err)
	}
	if a, err := c.UpsertAttention(ctx, mem, "小伙伴的名字", "d"); err != nil || a != "att_new" {
		t.Fatalf("new attention = %q %v", a, err)
	}
	if err := c.SetAttentionValue(ctx, mem, "02:00:00:00:00:01", "att_kid", "朵朵"); err != nil {
		t.Fatal(err)
	}
}

func TestErrorsAndRetry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"speaker_id":"spk1"}}`)
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	if id, err := c.CreateSpeaker(context.Background(), "朵朵"); err != nil || id != "spk1" || hits.Load() != 2 {
		t.Fatalf("retry: %q %v hits=%d", id, err, hits.Load())
	}

	bad := server(t, map[string]string{}, nil)
	bad.token = "wrong"
	if _, err := bad.CreateSpeaker(context.Background(), "x"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthorized: %v", err)
	}
	var apiErr *APIError
	c2 := server(t, map[string]string{"POST /v1/speakers": `{"code":500,"msg":"boom"}`}, nil)
	if _, err := c2.CreateSpeaker(context.Background(), "x"); !errors.As(err, &apiErr) || apiErr.Code != 500 {
		t.Fatalf("api error: %v", err)
	}
}

func TestHistoryParsing(t *testing.T) {
	c := server(t, map[string]string{
		"GET /xiaozhi/agent/a1/sessions":        `{"code":0,"data":{"total":1,"list":[{"sessionId":"s1","macAddress":"02:00:00:00:00:01","createdAt":"2026-10-23 09:00:00","chatCount":2}]}}`,
		"GET /xiaozhi/agent/a1/chat-history/s1": `{"code":0,"data":[{"createdAt":"2026-10-23 09:00:01","chatType":1,"content":"你好","audioUrl":"u","macAddress":"02:00:00:00:00:01"},{"createdAt":"2026-10-23 09:00:02","chatType":2,"content":"你好呀"}]}`,
		"GET /v1/agents/a1/chat-history":        `{"code":0,"data":{"records":[{"chat_type":1,"audio_url":"https://x/a.wav","device_id":"02:00:00:00:00:01","created_at":"2026-10-23T23:25:34.389+08:00"},{"chat_type":2,"audio_url":"https://x/tts.wav","device_id":"02:00:00:00:00:01"},{"chat_type":1,"audio_url":""}]}}`,
	}, nil)
	ctx := context.Background()
	ss, err := c.Sessions(ctx, "a1", 1, 20)
	if err != nil || len(ss) != 1 || ss[0].ID != "s1" || ss[0].MAC != "02:00:00:00:00:01" || !ss[0].CreatedAt.Equal(time.Date(2026, 10, 23, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("sessions = %+v %v", ss, err)
	}
	ms, err := c.SessionMessages(ctx, "a1", "s1")
	if err != nil || len(ms) != 2 || ms[0].Role != "user" || ms[0].MAC != "02:00:00:00:00:01" || ms[1].Role != "assistant" {
		t.Fatalf("messages = %+v %v", ms, err)
	}
	vs, err := c.RecentVoiceSamples(ctx, "a1")
	if err != nil || len(vs) != 1 || vs[0].MAC != "02:00:00:00:00:01" {
		t.Fatalf("samples = %+v %v", vs, err)
	}
}
