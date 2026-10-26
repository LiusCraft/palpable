# gateway

接入层 · `server/internal/gateway` · 依赖：`protocol`、`id`

## 边界

**职责**：接受 `/ws` 连接；完成 `hello` 握手；校验每条上行消息；维护「设备 → 会话」表；把消息与音频交给唯一的 `Handler`（`app`）。

**不负责**：好友、频道、AI、开通。它只知道「这是哪台设备」。

## 接口

```go
// 由 app 实现
type Handler interface {
    OnConnect(s *Session)
    OnMessage(s *Session, msg protocol.Message)
    OnAudio(s *Session, frame []byte)           // 热路径
    OnDisconnect(s *Session)
}

func New(h Handler, checkOrigin func(*http.Request) bool) *Gateway
func (g *Gateway) ServeHTTP(w, r)                // 升级并运行一个会话
func (g *Gateway) Session(d id.DeviceID) (*Session, bool)

type Session struct{ /* 只读：ID, DeviceID, Skin, Firmware, Capabilities */ }
func (s *Session) Send(msg protocol.Message) error   // 写串行化
func (s *Session) SendAudio(frame []byte) error
func (s *Session) Context() context.Context          // 连接断开即取消
```

`Session` 的方法集恰好满足 [bridge/linx](bridge-linx.md) 的 `DeviceLink`，`app` 直接把会话传过去。

## 连接生命周期

```text
upgrade (CheckOrigin)
hello = readHello(deadline 10s)            // 必须是 text 帧、合法 hello
    失败 → error{bad_message}，关闭
register(deviceID)                         // 同一 MAC 已在线 → error{unauthorized}，关闭
send hello.ack{session_id, audio_params}
handler.OnConnect(s)
loop ReadMessage:
    binary → handler.OnAudio(s, data)
    text:
        len > 4KB          → error{bad_message}，继续
        decode 失败        → error{bad_message}，继续
        是 hello           → error{bad_message}，继续
        否则               → handler.OnMessage(s, msg)
退出时（defer 顺序）：handler.OnDisconnect(s) → unregister → cancel ctx → 关闭 ws
```

## 约束

| 项 | 值 |
| --- | --- |
| 握手超时 | 10s |
| 单条文本 / 单帧读上限 | 4KB |
| 写超时 | 5s |
| 同一设备并发连接 | 1；后来者被拒绝 |
| 写并发 | `Session.write` 加互斥，可从多个 goroutine 调用 |

- 非法消息只回 `error`，不断开连接。
- `OnDisconnect` 先于 context 取消执行，让 `app` 能在会话仍有效时清理。
- `checkOrigin` 为 nil 时只允许同源；开发期由 `app` 注入「只放行本机前端」。
