# parentapi

接入层 · `server/internal/parentapi` · 依赖：`device`、`relation`、`channel`、`history`、`id`

## 边界

**职责**：家长端 REST。Web 手机窗口与将来的小程序调用同一套。做三件事：鉴权、孩子归属校验、调用领域模块并序列化。

**不负责**：任何业务规则。

## 接口

| 方法与路径 | 说明 | 调用 |
| --- | --- | --- |
| `GET /api/kids` | 名下的孩子与玩偶（含 `stage`、`online`） | `device.ByGuardian` |
| `GET /api/kids/{id}/friends` | 好友与待确认，含 `auto`（靠近自动生效） | `relation.Friends` |
| `POST /api/friend-requests/{id}/confirm` | 确认好友请求 | `relation.Request` + `Confirm` |
| `DELETE /api/kids/{id}/friends/{fid}` | 解除好友或拒绝请求 | `relation.Remove` |
| `GET /api/kids/{id}/groups` | 所在频道与群 | `channel.List` |
| `GET /api/kids/{id}/digest?date=today` | 今日回顾；只支持 today | `history.Today` |

端口（由 `app` 注入）：`Auth func(token) (GuardianID, bool)`、`Online func(DeviceID) bool`，以及上表对应的各模块窄接口。

## 数据结构

```go
type Kid    struct { ID; DollName; KidName; Skin; Stage string; Online bool }
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
            f 不存在，或 g 既不拥有 f.A 也不拥有 f.B → 404
            Confirm 返回 ErrNotPending → 409
```

## 约束

- 演示鉴权 `FixedToken`：常量时间比较单个令牌，代表 `demo.guardian`；将来换微信登录，只换 `Auth` 实现。
- 未配置 `demo.parent_token` 或 `demo.guardian` 时 `app` 不注册 `/api/`。
- 错误体统一 `{"error": "..."}`；内部错误只记日志，对外 `500 internal error`。
