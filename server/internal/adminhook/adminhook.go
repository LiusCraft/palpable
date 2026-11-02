// Package adminhook 把绑定、解绑事件推给厂家后台（palpable-admin）。
// 推送不能拖慢或阻塞绑定：事件进有界队列，由一个后台 goroutine 按顺序发送，失败按退避重试。
package adminhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	queueSize   = 256
	maxAttempts = 6
)

type Event struct {
	MAC      string `json:"mac"`
	Event    string `json:"event"` // activated | unbound
	OwnerRef string `json:"owner_ref,omitempty"`
}

type Client struct {
	url     string
	token   string
	http    *http.Client
	queue   chan Event
	backoff func(attempt int) time.Duration
}

func New(baseURL, token string) *Client {
	return &Client{
		url:   strings.TrimRight(baseURL, "/") + "/internal/platform-events",
		token: token,
		http:  &http.Client{Timeout: 10 * time.Second},
		queue: make(chan Event, queueSize),
		backoff: func(attempt int) time.Duration {
			return time.Duration(1<<attempt) * time.Second // 1s, 2s, 4s, 8s, 16s
		},
	}
}

// Notify 不阻塞；队列满了丢弃并记错误日志（后台与 palpabled 长时间不通时才会发生）。
func (c *Client) Notify(e Event) {
	select {
	case c.queue <- e:
	default:
		slog.Error("admin hook queue full, event dropped", "event", e.Event, "device_id", e.MAC)
	}
}

// Run 按顺序发送事件，直到 ctx 结束。顺序很重要（先绑定后解绑），所以重试期间后面的事件排队等待。
func (c *Client) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-c.queue:
			c.deliver(ctx, e)
		}
	}
}

func (c *Client) deliver(ctx context.Context, e Event) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		retry, err := c.post(ctx, e)
		if err == nil {
			return
		}
		if !retry {
			// 4xx：后台明确拒绝（例如它的台账里没有这台设备），重试没有意义。
			slog.Warn("admin hook rejected", "event", e.Event, "device_id", e.MAC, "error", err)
			return
		}
		slog.Warn("admin hook failed, will retry", "event", e.Event, "device_id", e.MAC, "attempt", attempt+1, "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.backoff(attempt)):
		}
	}
	slog.Error("admin hook gave up", "event", e.Event, "device_id", e.MAC)
}

// post 返回 (是否值得重试, 错误)。
func (c *Client) post(ctx context.Context, e Event) (bool, error) {
	body, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	switch {
	case resp.StatusCode < 300:
		return false, nil
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return true, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return false, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
}
