// Package linxapi 是灵矽 REST 的薄客户端，只做协议转换，不含业务。
// 灵矽的 HTTP 状态恒为 200，结果看响应体的 code；令牌只来自服务端 YAML 配置。
package linxapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBase    = "https://xrobo.qiniu.com"
	requestTimeout = 10 * time.Second

	codeMemoryExists = 614 // 智能体已有记忆容器
)

var (
	ErrUnauthorized = errors.New("linxapi: token invalid or expired")
	ErrNotFound     = errors.New("linxapi: not found")
	// ErrAlreadyRegistered 表示 MAC 已经预注册过；调用方按"已开通"处理。
	ErrAlreadyRegistered = errors.New("linxapi: device already registered")
)

// APIError 是灵矽返回的非 0 业务码。
type APIError struct {
	Path string
	Code int
	Msg  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("linxapi %s: code %d: %s", e.Path, e.Code, e.Msg)
}

type Client interface {
	CreateAgent(ctx context.Context, a AgentSpec) (agentID string, err error)
	FindAgentByName(ctx context.Context, name string) (agentID string, found bool, err error)
	PreregisterDevice(ctx context.Context, agentID, mac string) error
	SwitchAgent(ctx context.Context, mac, agentID string, migrateHistory bool) error
	EnableMemory(ctx context.Context, agentID string) (memoryID string, err error)
	UpsertAttention(ctx context.Context, memoryID, name, description string) (attentionID string, err error)
	SetAttentionValue(ctx context.Context, memoryID, mac, attentionID, value string) error
	AddMemoryNode(ctx context.Context, memoryID, mac, content string) error
	CreateSpeaker(ctx context.Context, name string) (speakerID string, err error)
	EnrollVoiceprint(ctx context.Context, speakerID, audioURL string) error
	BindSpeakers(ctx context.Context, agentID string, speakerIDs []string) error
	SetOwnerOnly(ctx context.Context, agentID string, on bool) error
	RecentVoiceSamples(ctx context.Context, agentID string) ([]VoiceSample, error)
	Sessions(ctx context.Context, agentID string, page, limit int) ([]Session, error)
	SessionMessages(ctx context.Context, agentID, sessionID string) ([]Message, error)
}

type AgentSpec struct {
	Name, AssistantName, SystemPrompt, MemoryModel string
	VoiceID                                        string // ttsVoiceId；空则用平台默认
}

const (
	MemoryLongTerm = "Memory_long_term_memory"
	MemoryNone     = "Memory_nomem"
)

type VoiceSample struct {
	AudioURL string
	MAC      string // 样本来自哪台设备；平台没给时为空，调用方据此决定能否使用
	At       time.Time
}

type Session struct {
	ID        string
	CreatedAt time.Time
	ChatCount int
}

type Message struct {
	Role     string // user / assistant / tool
	Text     string
	AudioURL string
	MAC      string
	At       time.Time
}

type HTTPClient struct {
	base  string
	token string
	http  *http.Client
}

var _ Client = (*HTTPClient)(nil)

func New(base, token string) *HTTPClient {
	if base == "" {
		base = DefaultBase
	}
	return &HTTPClient{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: requestTimeout}}
}

type envelope struct {
	Code       int             `json:"code"`
	Msg        string          `json:"msg"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
	NextCursor *string         `json:"nextCursor"`
}

// do 发请求并解出 data；网络错误与 5xx 重试一次，业务错误不重试。
func (c *HTTPClient) do(ctx context.Context, method, path string, body, out any) error {
	var err error
	for attempt := range 2 {
		var retry bool
		retry, err = c.once(ctx, method, path, body, out)
		if err == nil || !retry || attempt == 1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return err
}

func (c *HTTPClient) once(ctx context.Context, method, path string, body, out any) (retry bool, err error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return false, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ctx.Err() == nil, fmt.Errorf("linxapi %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return true, fmt.Errorf("linxapi %s %s: http %d", method, path, resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return false, fmt.Errorf("linxapi %s %s: decode: %w", method, path, err)
	}
	switch env.Code {
	case 0:
	case 401, 403:
		return false, fmt.Errorf("linxapi %s: %w", path, ErrUnauthorized)
	case 404:
		return false, fmt.Errorf("linxapi %s: %w", path, ErrNotFound)
	default:
		msg := env.Msg
		if msg == "" {
			msg = env.Message
		}
		return false, &APIError{Path: path, Code: env.Code, Msg: msg}
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return false, fmt.Errorf("linxapi %s %s: decode data: %w", method, path, err)
		}
	}
	return false, nil
}

func (c *HTTPClient) CreateAgent(ctx context.Context, a AgentSpec) (string, error) {
	body := map[string]any{"agentName": a.Name, "assistantName": a.AssistantName}
	if a.SystemPrompt != "" {
		body["systemPrompt"] = a.SystemPrompt
	}
	if a.MemoryModel != "" {
		body["memModelId"] = a.MemoryModel
	}
	if a.VoiceID != "" {
		body["ttsVoiceId"] = a.VoiceID
	}
	var id string
	if err := c.do(ctx, http.MethodPost, "/xiaozhi/agent", body, &id); err != nil {
		return "", err
	}
	return id, nil
}

// FindAgentByName 遍历智能体列表按名字精确匹配；平台的搜索只支持 MAC 与 ID。
func (c *HTTPClient) FindAgentByName(ctx context.Context, name string) (string, bool, error) {
	var agents []struct {
		ID        string `json:"id"`
		AgentName string `json:"agentName"`
	}
	if err := c.do(ctx, http.MethodGet, "/xiaozhi/agent/list", nil, &agents); err != nil {
		return "", false, err
	}
	for _, a := range agents {
		if a.AgentName == name {
			return a.ID, true, nil
		}
	}
	return "", false, nil
}

func (c *HTTPClient) PreregisterDevice(ctx context.Context, agentID, mac string) error {
	var res struct {
		SuccessCount int `json:"success_count"`
		Failed       []struct {
			Value  string `json:"value"`
			Reason string `json:"reason"`
		} `json:"failed"`
	}
	body := map[string]any{"agent_id": agentID, "mac_addresses": []string{mac}, "serial_numbers": []string{}}
	if err := c.do(ctx, http.MethodPost, "/xiaozhi/device/preregister", body, &res); err != nil {
		return err
	}
	for _, f := range res.Failed {
		if strings.EqualFold(f.Value, mac) {
			if strings.Contains(f.Reason, "已存在") || strings.Contains(strings.ToLower(f.Reason), "exist") {
				return ErrAlreadyRegistered
			}
			return &APIError{Path: "/xiaozhi/device/preregister", Code: -1, Msg: f.Reason}
		}
	}
	return nil
}

func (c *HTTPClient) SwitchAgent(ctx context.Context, mac, agentID string, migrateHistory bool) error {
	path := "/v1/devices/" + url.PathEscape(mac) + "/agent/" + url.PathEscape(agentID)
	return c.do(ctx, http.MethodPut, path, map[string]any{"disable_chat_history_migration": !migrateHistory}, nil)
}

// EnableMemory 为智能体建记忆容器；已有时返回现有容器。
func (c *HTTPClient) EnableMemory(ctx context.Context, agentID string) (string, error) {
	var mem struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/memories", map[string]any{"agent_id": agentID, "enabled": true}, &mem)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == codeMemoryExists {
		err = c.do(ctx, http.MethodGet, "/v1/agents/"+url.PathEscape(agentID)+"/memory", nil, &mem)
	}
	if err != nil {
		return "", err
	}
	if mem.ID == "" {
		return "", fmt.Errorf("linxapi: agent %s has no memory: %w", agentID, ErrNotFound)
	}
	return mem.ID, nil
}

// UpsertAttention 按名字查找记忆焦点，没有就创建。
func (c *HTTPClient) UpsertAttention(ctx context.Context, memoryID, name, description string) (string, error) {
	path := "/v1/memories/" + url.PathEscape(memoryID) + "/attentions"
	var list []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &list); err != nil {
		return "", err
	}
	for _, a := range list {
		if a.Name == name {
			return a.ID, nil
		}
	}
	var created struct {
		ID string `json:"id"`
	}
	body := map[string]any{"name": name, "description": description, "is_single_value": true}
	if err := c.do(ctx, http.MethodPost, path, body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (c *HTTPClient) SetAttentionValue(ctx context.Context, memoryID, mac, attentionID, value string) error {
	path := fmt.Sprintf("/v1/memories/%s/identifiers/%s/attention_values/%s", url.PathEscape(memoryID), url.PathEscape(mac), url.PathEscape(attentionID))
	return c.do(ctx, http.MethodPost, path, map[string]any{"value": value}, nil)
}

func (c *HTTPClient) AddMemoryNode(ctx context.Context, memoryID, mac, content string) error {
	path := fmt.Sprintf("/v1/memories/%s/identifiers/%s/nodes", url.PathEscape(memoryID), url.PathEscape(mac))
	return c.do(ctx, http.MethodPost, path, map[string]any{"content": content}, nil)
}

func (c *HTTPClient) CreateSpeaker(ctx context.Context, name string) (string, error) {
	var res struct {
		SpeakerID string `json:"speaker_id"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/speakers", map[string]any{"name": name}, &res); err != nil {
		return "", err
	}
	return res.SpeakerID, nil
}

func (c *HTTPClient) EnrollVoiceprint(ctx context.Context, speakerID, audioURL string) error {
	return c.do(ctx, http.MethodPost, "/v1/speakers/"+url.PathEscape(speakerID)+"/voiceprint", map[string]any{"audio_url": audioURL}, nil)
}

func (c *HTTPClient) BindSpeakers(ctx context.Context, agentID string, speakerIDs []string) error {
	if speakerIDs == nil {
		speakerIDs = []string{}
	}
	return c.do(ctx, http.MethodPut, "/v1/agents/"+url.PathEscape(agentID)+"/speaker-config", map[string]any{"speaker_ids": speakerIDs}, nil)
}

func (c *HTTPClient) SetOwnerOnly(ctx context.Context, agentID string, on bool) error {
	body := map[string]any{"extra": map[string]any{"voiceprint": map[string]any{"chat_only_enabled": on}}}
	return c.do(ctx, http.MethodPut, "/xiaozhi/agent/"+url.PathEscape(agentID), body, nil)
}

func (c *HTTPClient) RecentVoiceSamples(ctx context.Context, agentID string) ([]VoiceSample, error) {
	var res struct {
		Records []struct {
			AudioURL   string `json:"audio_url"`
			MACAddress string `json:"mac_address"`
			MAC        string `json:"mac"`
			DeviceID   string `json:"device_id"`
			CreatedAt  string `json:"created_at"`
		} `json:"records"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/agents/"+url.PathEscape(agentID)+"/chat-history", nil, &res); err != nil {
		return nil, err
	}
	var out []VoiceSample
	for _, r := range res.Records {
		if r.AudioURL == "" {
			continue
		}
		mac := strings.ToLower(firstNonEmpty(r.MACAddress, r.MAC, r.DeviceID))
		out = append(out, VoiceSample{AudioURL: r.AudioURL, MAC: mac, At: parseTime(r.CreatedAt)})
	}
	return out, nil
}

func (c *HTTPClient) Sessions(ctx context.Context, agentID string, page, limit int) ([]Session, error) {
	var res struct {
		List []struct {
			SessionID string `json:"sessionId"`
			CreatedAt string `json:"createdAt"`
			ChatCount int    `json:"chatCount"`
		} `json:"list"`
	}
	q := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}}
	if err := c.do(ctx, http.MethodGet, "/xiaozhi/agent/"+url.PathEscape(agentID)+"/sessions?"+q.Encode(), nil, &res); err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(res.List))
	for _, s := range res.List {
		out = append(out, Session{ID: s.SessionID, CreatedAt: parseTime(s.CreatedAt), ChatCount: s.ChatCount})
	}
	return out, nil
}

func (c *HTTPClient) SessionMessages(ctx context.Context, agentID, sessionID string) ([]Message, error) {
	var res []struct {
		CreatedAt  string `json:"createdAt"`
		ChatType   int    `json:"chatType"`
		Content    string `json:"content"`
		AudioURL   string `json:"audioUrl"`
		MACAddress string `json:"macAddress"`
	}
	path := "/xiaozhi/agent/" + url.PathEscape(agentID) + "/chat-history/" + url.PathEscape(sessionID)
	if err := c.do(ctx, http.MethodGet, path, nil, &res); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(res))
	for _, m := range res {
		role := "tool"
		switch m.ChatType {
		case 1:
			role = "user"
		case 2:
			role = "assistant"
		}
		out = append(out, Message{Role: role, Text: m.Content, AudioURL: m.AudioURL, MAC: strings.ToLower(m.MACAddress), At: parseTime(m.CreatedAt)})
	}
	return out, nil
}

// 平台时间有 "2006-01-02 15:04:05"（北京时间）与 RFC 3339 两种写法。
var beijing = time.FixedZone("CST", 8*3600)

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, beijing); err == nil {
			return t
		}
	}
	return time.Time{}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
