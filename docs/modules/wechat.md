# wechat

基础层 · `server/internal/wechat` · 无依赖

## 边界

**职责**：微信开放接口的薄客户端，只做协议转换。目前只有登录凭证校验（`code2Session`）。

**不负责**：账号与会话（[account](account.md)）。

## 接口

```go
type Client interface {
    Code2Session(ctx, code) (Identity, error)   // GET /sns/jscode2session
}
type Identity struct { OpenID, UnionID string }   // UnionID 仅在绑定开放平台时有

func New(appID, secret, base string) *HTTPClient  // base 为空用 https://api.weixin.qq.com
type Fake struct{}                                // code 直接当 openid（"fake-"+code）；"bad" 返回 ErrInvalidCode
```

| 微信 errcode | 错误 |
| --- | --- |
| 40029、40163（code 无效 / 已用过） | `ErrInvalidCode` |
| 40226（高风险用户） | `ErrRiskyUser` |
| 45011（频率限制） | `ErrRateLimited` |
| 其他非 0 | `*APIError` |

## 约束

- AppSecret 只来自 `server/config.yaml` 的 `wechat.secret`；传输错误剥掉 `url.Error`，**错误信息里不出现带 secret 的 URL**。
- `session_key` 解析后丢弃，不返回、不记日志。
- 微信的 `Content-Type` 有时是 `text/plain`，一律按 JSON 解析；响应体最多读 64 KiB。
- 订阅消息（交友请求提醒）需要 `access_token` 与发送接口，计划中，见 [tasks.md](../tasks.md)。
