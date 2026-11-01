# parentapi

接入层 · `server/internal/parentapi` · 依赖：`device`、`relation`、`channel`、`history`、`account`、`id`

## 边界

**职责**：家长端 REST。[微信小程序](miniprogram.md)与 Web 演示手机窗口调用同一套。做三件事：鉴权、孩子归属校验、调用领域模块并序列化。

**不负责**：任何业务规则。

## 接口

| 方法与路径 | 说明 | 调用 |
| --- | --- | --- |
| `POST /api/session` | 不需令牌。`{code}`（`wx.login` 的 code）换会话令牌 `{token, expires_at}`；code 无效 400，风控拦截 403，太频繁 429 | `Accounts.SignInWeChat` |
| `DELETE /api/session` | 退出登录，作废当前令牌 | `Accounts.SignOut` |
| `GET` / `PUT /api/me` | 家长称呼 `display_name`（1–20 字）；演示家长返回空称呼 | `Accounts.Profile` / `SetDisplayName` |
| `DELETE /api/me` | 注销：名下设备逐台解绑并恢复出厂，再删账号与全部会话 | `Accounts.DeleteAccount` |
| `GET /api/kids` | 名下的孩子与玩偶（含 `stage`、`online`、`waiting_you` 等你批准的请求数） | `device.ByGuardian`、`relation.Friends` |
| `GET /api/kids/{id}/friends` | 好友与等家长的请求；`waiting_for` 为 `you`（等你）或 `other`（等对方家长），`auto` 表示两家都设为自动 | `relation.Friends` |
| `POST /api/friend-requests/{id}/confirm` | 放行**自家孩子那一侧**；两个孩子都是自家的则两侧一起放行 | `relation.Request` + `Confirm(side)` |
| `DELETE /api/kids/{id}/friends/{fid}` | 解除好友或拒绝请求（拒绝后 24 小时不再询问） | `relation.Remove` |
| `GET` / `PUT /api/kids/{id}/settings` | `friend_policy`：`approve`（默认）或 `notify`；`agent_in_channel`：小伙伴能否在频道发言（默认 true，PUT 省略不改）；`voice_consent`：声音登记（PUT 省略不改；`false` 立即删除灵矽侧声纹，灵矽不可用时 503 且其余设置不改；`true` 只在起名前可开，否则 409）。GET 另有只读的 `voice_enrolled`、`voice_can_enable` | `device.Profile` / `SetFriendPolicy` / `SetAgentInChannel` / `GrantVoice` / `RevokeVoice` |
| `PUT /api/kids/{id}/friends/{fid}/agent` | `{"blocked": bool}`：屏蔽这位朋友的小伙伴；好友列表里有 `agent_blocked` | `device.BlockAgent` |
| `GET /api/kids/{id}/groups` | 所在频道与群 | `channel.List` |
| `GET /api/kids/{id}/digest?date=today` | 今日回顾；只支持 today | `history.Today` |
| `POST /api/bindings` | 输入设备播报的绑定码完成绑定；`ai_companion` 必须为 true，`voice_consent` 是声纹的单独同意。错码 400，猜太多 429，已被绑定 409 | `Bindings.Claim` |
| `DELETE /api/kids/{id}` | 解绑并恢复出厂（删好友、群、历史） | `Bindings.Unbind` |

端口（由 `app` 注入）：`Auth func(ctx, token) (GuardianID, bool)`、`Online func(DeviceID) bool`、`Bindings`（`EnableBinding` 注入；解绑要同时清理好友与频道，所以由 `app` 实现）、`Accounts`（`EnableAccounts` 注入；注销要先解绑名下设备，`DeleteAccount` 由 `app` 实现），以及上表对应的各模块窄接口。

## 数据结构

```go
type Kid    struct { ID; DollName; KidName; Skin; Stage string; Online bool; WaitingYou int }
type Friend struct { RequestID; DeviceID; Name; Status string /*pending|confirmed*/; Auto, Online bool; Since time.Time }
type Group  struct { ID; Kind string /*zone|group*/; Name, Code string; ExpiresAt *time.Time; Foreground bool; Members int }
```

## 鉴权与归属

```text
guard(h):   token = Authorization 去掉 "Bearer "
            g, ok = auth(token);  !ok → 401
kid(h):     p = devices.Profile(path.id)
            err 或 p.Guardian != g → 404          // 用 404 而非 403，避免探测设备是否存在
confirm:    f = relations.Request(id)
            sides = f.A、f.B 中属于 g 的
            f 不存在或 sides 为空 → 404
            for side in sides: Confirm(f, side, g)；ErrNotPending → 409
```

## 约束

- 鉴权由 `app` 组合：配置了 `wechat` 时用 [account](account.md) 的会话令牌；配置了 `demo.parent_token` + `demo.guardian` 时另接受演示固定令牌 `FixedToken`（常量时间比较）。两者都没有时 `app` 不注册 `/api/`。
- `POST /api/session` 的响应带 `Cache-Control: no-store`。
- 错误体统一 `{"error": "..."}`；内部错误只记日志，对外 `500 internal error`。
