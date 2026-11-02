// Package palpclient 是 factory.Platform 的 HTTP 实现：厂家后台经它调用 palpabled 的 /internal 接口。
// 接口约定见 docs/factory.md §5。
package palpclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiusCraft/palpable/server/internal/factory"
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

var _ factory.Platform = (*Client)(nil)

// New 的 baseURL 形如 http://palpabled:8080；token 是 palpabled 的 ops 令牌。
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), token: token, http: hc}
}

func (c *Client) do(ctx context.Context, method, mac, path string, body any, okStatus ...int) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/internal/devices/"+url.PathEscape(mac)+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	for _, s := range okStatus {
		if resp.StatusCode == s {
			return nil
		}
	}
	return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
}

func (c *Client) Enroll(ctx context.Context, mac string, pubkey []byte) error {
	return c.do(ctx, http.MethodPut, mac, "/key", map[string]string{"public_key": base64.StdEncoding.EncodeToString(pubkey)},
		http.StatusOK, http.StatusNoContent, http.StatusCreated)
}

// Revoke、Wipe 对 palpabled 不认识的设备（404）视为成功：目标状态已经达到。
func (c *Client) Revoke(ctx context.Context, mac string) error {
	return c.do(ctx, http.MethodDelete, mac, "/key", nil, http.StatusOK, http.StatusNoContent, http.StatusNotFound)
}

func (c *Client) Wipe(ctx context.Context, mac string) error {
	return c.do(ctx, http.MethodPost, mac, "/wipe", nil, http.StatusOK, http.StatusNoContent, http.StatusNotFound)
}

func (c *Client) SetLock(ctx context.Context, mac string, locked bool) error {
	return c.do(ctx, http.MethodPut, mac, "/lock", map[string]bool{"locked": locked}, http.StatusOK, http.StatusNoContent)
}
