# account

领域层 · `server/internal/account` · 依赖：`store`、`wechat`、`id`

## 边界

**职责**：家长账号。微信登录换取家长身份（首次登录自动建账号）、签发与校验家长端会话令牌、改称呼、注销。

**不负责**：注销时解绑名下设备（要清好友、离频道、通知设备，由 `app` 编排后再调 `Delete`）；家长端鉴权中间件（`parentapi`）。

## 接口

```go
func New(guardians store.GuardianRepo, sessions store.SessionRepo, wx wechat.Client) *Accounts

SignInWeChat(ctx, code) (Token, error)          // ErrBadCode / ErrRejected / ErrRateLimited
Verify(ctx, token) (id.GuardianID, bool)        // 即 parentapi.Auth
SignOut(ctx, token) error                       // 令牌本来无效也算成功
Profile(ctx, g) (Guardian, error)               // ErrNotFound
SetDisplayName(ctx, g, name) error              // 去首尾空白后 1–20 字，否则 ErrBadName
Delete(ctx, g) error                            // 删账号与全部会话；ErrNotFound
Sweep(ctx) (int, error)                         // 清过期会话，app 每小时调一次
```

## 数据结构

```go
type Token    struct { Value string; Guardian id.GuardianID; ExpiresAt time.Time }   // Value 只在签发时出现一次
type Guardian struct { ID id.GuardianID; DisplayName string; CreatedAt time.Time }
```

- 家长 ID 为 `g_` + 16 位随机十六进制，**与 openid 无关**：日志与接口里出现家长 ID 不泄露 openid。
- 令牌是 32 字节随机数的 base64url（43 字符），库里只存 SHA-256（`guardian_session.token_hash`）。
- 有效期 30 天，不续期；过期后小程序静默 `wx.login` 重新换一个，家长无感。
- 只保存 openid，不保存 `session_key`、unionid、手机号、头像、昵称。

## 关键流程

```text
SignInWeChat(code):
    ident = wx.Code2Session(code)              // 微信错误码映射为 ErrBadCode / ErrRejected / ErrRateLimited
    g = guardians.ByWxOpenID(ident.OpenID)
    不存在 → Create(新 ID)；ErrConflict（并发首次登录）→ 再查一次，用先建成的
    issue(g)：随机令牌 → sessions.Create(hash, g, now+30d)

Verify(token):  长度不是 43 → false；sessions.Get(hash) 不存在或已过期 → false
```

## 约束

- 注销后同一微信再登录是**全新账号**（新家长 ID），旧账号的设备已在注销时解绑。
- 演示家长（`demo.guardian`）没有账号记录：`Profile` 返回 `ErrNotFound`，`parentapi` 按空称呼展示，不能注销。
