// Package wechat 是微信开放接口的薄客户端，只做协议转换，不含业务。
// AppSecret 只来自服务端 YAML 配置；session_key 用不到，解析后立即丢弃，不返回、不记日志。
package wechat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	DefaultBase    = "https://api.weixin.qq.com"
	requestTimeout = 10 * time.Second
)

var (
	// ErrInvalidCode：登录 code 无效、过期或已用过（40029、40163）。小程序重新 wx.login 即可。
	ErrInvalidCode = errors.New("wechat: invalid login code")
	// ErrRiskyUser：微信判定为高风险用户，拒绝登录（40226）。
	ErrRiskyUser = errors.New("wechat: user blocked by risk control")
	// ErrRateLimited：同一用户登录太频繁（45011）。
	ErrRateLimited = errors.New("wechat: rate limited")
)

// APIError 是微信返回的其他非 0 errcode。
type APIError struct {
	Code int
	Msg  string
}

func (e *APIError) Error() string { return fmt.Sprintf("wechat: errcode %d: %s", e.Code, e.Msg) }

// Identity 是登录得到的用户标识；UnionID 只在小程序绑定了开放平台时才有。
type Identity struct {
	OpenID  string
	UnionID string
}

type Client interface {
	// Code2Session 用小程序 wx.login 拿到的 code 换取用户标识。
	Code2Session(ctx context.Context, code string) (Identity, error)
}

type HTTPClient struct {
	appID, secret, base string
	http                *http.Client
}

// New 创建客户端；base 为空时用微信正式地址，测试时指向假服务器。
func New(appID, secret, base string) *HTTPClient {
	if base == "" {
		base = DefaultBase
	}
	return &HTTPClient{appID: appID, secret: secret, base: base, http: &http.Client{Timeout: requestTimeout}}
}

func (c *HTTPClient) Code2Session(ctx context.Context, code string) (Identity, error) {
	q := url.Values{"appid": {c.appID}, "secret": {c.secret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/sns/jscode2session?"+q.Encode(), nil)
	if err != nil {
		return Identity{}, fmt.Errorf("code2session: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// 不把带 secret 的 URL 带进错误：url.Error 会打印完整地址。
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return Identity{}, fmt.Errorf("code2session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("code2session: http %d", resp.StatusCode)
	}
	// 微信的 Content-Type 有时是 text/plain，按 JSON 解析即可。
	var body struct {
		OpenID  string `json:"openid"`
		UnionID string `json:"unionid"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return Identity{}, fmt.Errorf("code2session: decode: %w", err)
	}
	switch body.ErrCode {
	case 0:
	case 40029, 40163:
		return Identity{}, ErrInvalidCode
	case 40226:
		return Identity{}, ErrRiskyUser
	case 45011:
		return Identity{}, ErrRateLimited
	default:
		return Identity{}, fmt.Errorf("code2session: %w", &APIError{Code: body.ErrCode, Msg: body.ErrMsg})
	}
	if body.OpenID == "" {
		return Identity{}, errors.New("code2session: empty openid")
	}
	return Identity{OpenID: body.OpenID, UnionID: body.UnionID}, nil
}
