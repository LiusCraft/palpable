# gateway

接入层 · `server/internal/gateway` · 依赖：`protocol`、`id`

## 边界

**职责**：接受 `/ws` 连接；完成 `hello` 握手与设备认证挑战；校验每条上行消息；维护「设备 → 会话」表；把消息与音频交给唯一的 `Handler`（`app`）。

**不负责**：好友、频道、AI、开通、绑定。它只知道「这是哪台设备、它证明了自己」。公钥怎么登记由 `device.Authenticator` 决定（端口 `Authenticator`，`app` 注入），规则见 [trust.md §2](../trust.md#2-设备身份与认证)。

## 接口

```go
// 由 app 实现
type Handler interface {
    OnConnect(s *Session)
    OnMessage(s *Session, msg protocol.Message)
    OnAudio(s *Session, frame []byte)           // 热路径
    OnDisconnect(s *Session)
}

// 由 device.Registry 实现：厂家后台挂失或吊销的设备在这里被拒绝
type Admission interface {
    Admit(ctx, d DeviceID) error
}

// 由 device.Authenticator 实现
type Authenticator interface {
    Required() bool
    Verify(ctx, d DeviceID, pubkey []byte, nonce string, sig []byte) error
}

func New(h Handler, checkOrigin func(*http.Request) bool) *Gateway
func (g *Gateway) SetAuth(a Authenticator)       // nil 或 Required()==false 时不认证
func (g *Gateway) SetAdmission(a Admission)      // 握手后、认证前决定能否连接；nil 不设准入
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
deadline = now + 10s                       // 覆盖整个握手
hello = readHello(deadline)                // 必须是 text 帧、合法 hello
    失败 → error{bad_message}，关闭
if 设了准入 && Admit(mac) 失败:
    error{unauthorized, "device not allowed"}，关闭        // 在认证之前：被封锁的设备不必再做签名
if 认证开启:
    send hello.challenge{nonce}            // 32 字节随机数，每条连接一个
    proof = readText(deadline)             // 必须是 hello.proof
    auth.Verify(mac, hello.pubkey, nonce, sig)
        失败 → error{unauthorized}，关闭
register(deviceID, takeover=认证开启)
    已在线且 takeover → 旧会话 kick（error{unauthorized,"replaced"} + 关闭）→ 等旧会话 done（≤5s）→ 登记
    已在线且不 takeover → error{unauthorized}，关闭
send hello.ack{session_id, audio_params}
handler.OnConnect(s)
loop ReadMessage:
    binary → handler.OnAudio(s, data)
    text:
        len > 4KB          → error{bad_message}，继续
        decode 失败        → error{bad_message}，继续
        是 hello / hello.proof → error{bad_message}，继续
        否则               → handler.OnMessage(s, msg)
退出时（defer 顺序）：handler.OnDisconnect(s) → unregister → close(done) → cancel ctx → 关闭 ws
```

## 约束

| 项 | 值 |
| --- | --- |
| 握手超时 | 10s |
| 单条文本 / 单帧读上限 | 4KB |
| 写超时 | 5s |
| 同一设备并发连接 | 1；认证开启时新连接顶掉旧连接，否则后来者被拒绝 |
| 顶号等待旧会话清理 | 5s，超时则拒绝新连接 |
| 写并发 | `Session.write` 加互斥，可从多个 goroutine 调用 |

- 非法消息只回 `error`，不断开连接。
- `OnDisconnect` 先于 context 取消执行，让 `app` 能在会话仍有效时清理。
- `checkOrigin` 为 nil 时只允许同源；开发期由 `app` 注入「只放行本机前端」。
