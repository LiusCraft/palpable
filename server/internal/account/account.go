// Package account 是家长账号：微信登录换取家长身份，签发与校验家长端会话令牌，注销账号。
// 令牌是随机串，库里只存 SHA-256；不保存 session_key、手机号、头像等用不到的微信信息。
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/wechat"
)

// SessionTTL 是令牌有效期。到期后小程序静默重新 wx.login，家长无感。
const SessionTTL = 30 * 24 * time.Hour

const maxDisplayName = 20 // 字符数

var (
	ErrBadCode     = errors.New("account: invalid login code")
	ErrRejected    = errors.New("account: login rejected by wechat risk control")
	ErrRateLimited = errors.New("account: too many login attempts")
	ErrBadName     = errors.New("account: display name must be 1-20 characters")
	ErrNotFound    = errors.New("account: not found")
)

type Guardian struct {
	ID          id.GuardianID
	DisplayName string
	CreatedAt   time.Time
}

// Token 是签发给小程序的会话令牌；Value 只在签发时出现一次。
type Token struct {
	Value     string
	Guardian  id.GuardianID
	ExpiresAt time.Time
}

type Accounts struct {
	guardians store.GuardianRepo
	sessions  store.SessionRepo
	wx        wechat.Client
	now       func() time.Time
}

func New(guardians store.GuardianRepo, sessions store.SessionRepo, wx wechat.Client) *Accounts {
	return &Accounts{guardians: guardians, sessions: sessions, wx: wx, now: time.Now}
}

// SignInWeChat 用 wx.login 的 code 登录：首次登录自动建家长账号，然后签发新令牌。
func (a *Accounts) SignInWeChat(ctx context.Context, code string) (Token, error) {
	if code == "" || len(code) > 128 {
		return Token{}, ErrBadCode
	}
	ident, err := a.wx.Code2Session(ctx, code)
	switch {
	case errors.Is(err, wechat.ErrInvalidCode):
		return Token{}, ErrBadCode
	case errors.Is(err, wechat.ErrRiskyUser):
		return Token{}, ErrRejected
	case errors.Is(err, wechat.ErrRateLimited):
		return Token{}, ErrRateLimited
	case err != nil:
		return Token{}, fmt.Errorf("sign in: %w", err)
	}
	g, err := a.guardianFor(ctx, ident.OpenID)
	if err != nil {
		return Token{}, err
	}
	return a.issue(ctx, g)
}

// guardianFor 找到或新建 openid 对应的家长；并发首次登录时以先建成的为准。
func (a *Accounts) guardianFor(ctx context.Context, openid string) (id.GuardianID, error) {
	g, err := a.guardians.ByWxOpenID(ctx, openid)
	if err == nil {
		return g.ID, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", fmt.Errorf("sign in: %w", err)
	}
	// 家长 ID 与 openid 无关，日志和接口里出现家长 ID 也不会泄露 openid。
	ng := store.Guardian{ID: id.GuardianID("g_" + id.New()), WxOpenID: openid, CreatedAt: a.now()}
	err = a.guardians.Create(ctx, ng)
	if errors.Is(err, store.ErrConflict) {
		if g, err := a.guardians.ByWxOpenID(ctx, openid); err == nil {
			return g.ID, nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("sign in: create guardian: %w", err)
	}
	return ng.ID, nil
}

func (a *Accounts) issue(ctx context.Context, g id.GuardianID) (Token, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, fmt.Errorf("issue token: %w", err)
	}
	now := a.now()
	t := Token{Value: base64.RawURLEncoding.EncodeToString(raw), Guardian: g, ExpiresAt: now.Add(SessionTTL)}
	if err := a.sessions.Create(ctx, store.Session{TokenHash: hash(t.Value), Guardian: g, CreatedAt: now, ExpiresAt: t.ExpiresAt}); err != nil {
		return Token{}, fmt.Errorf("issue token: %w", err)
	}
	return t, nil
}

// Verify 把令牌解析为家长；过期、不存在或格式不对都返回 false。
func (a *Accounts) Verify(ctx context.Context, token string) (id.GuardianID, bool) {
	if len(token) != 43 { // 32 字节的 base64url
		return "", false
	}
	s, err := a.sessions.Get(ctx, hash(token))
	if err != nil || !s.ExpiresAt.After(a.now()) {
		return "", false
	}
	return s.Guardian, true
}

// SignOut 作废这个令牌；令牌本来就无效时也算成功。
func (a *Accounts) SignOut(ctx context.Context, token string) error {
	if err := a.sessions.Delete(ctx, hash(token)); err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("sign out: %w", err)
	}
	return nil
}

func (a *Accounts) Profile(ctx context.Context, g id.GuardianID) (Guardian, error) {
	sg, err := a.guardians.Get(ctx, g)
	if errors.Is(err, store.ErrNotFound) {
		return Guardian{}, ErrNotFound
	}
	if err != nil {
		return Guardian{}, fmt.Errorf("guardian profile: %w", err)
	}
	return Guardian{ID: sg.ID, DisplayName: sg.DisplayName, CreatedAt: sg.CreatedAt}, nil
}

// SetDisplayName 改家长在家长端的称呼（如"朵朵妈妈"），只用于家长端展示。
func (a *Accounts) SetDisplayName(ctx context.Context, g id.GuardianID, name string) error {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n == 0 || n > maxDisplayName {
		return ErrBadName
	}
	sg, err := a.guardians.Get(ctx, g)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("set display name: %w", err)
	}
	sg.DisplayName = name
	if err := a.guardians.Put(ctx, sg); err != nil {
		return fmt.Errorf("set display name: %w", err)
	}
	return nil
}

// Delete 删除家长账号与全部会话。名下设备须由调用方先解绑（解绑要清理好友与频道，由 app 编排）。
func (a *Accounts) Delete(ctx context.Context, g id.GuardianID) error {
	err := a.guardians.Delete(ctx, g)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}

// Sweep 清理过期会话，返回清理条数。
func (a *Accounts) Sweep(ctx context.Context) (int, error) {
	return a.sessions.DeleteExpired(ctx, a.now())
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
