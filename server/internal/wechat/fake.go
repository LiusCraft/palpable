package wechat

import (
	"context"
	"strings"
)

// Fake 把 code 直接当作 openid：code 为 "bad" 时返回 ErrInvalidCode。供测试与本地开发用。
type Fake struct{}

func (Fake) Code2Session(_ context.Context, code string) (Identity, error) {
	if code == "" || code == "bad" {
		return Identity{}, ErrInvalidCode
	}
	return Identity{OpenID: "fake-" + strings.TrimSpace(code)}, nil
}
