# bridge/linx

能力层 · `server/internal/bridge/linx` · 依赖：`protocol`、`id`；端口 `DeviceLink`

## 边界

**职责**：为每台已开通的设备维护一条灵矽 WebSocket 会话：握手、转发 `voice` 交来的音频、把灵矽的 `stt / tts / mcp` 翻译成设备侧的灯光与 `speaker.state`、把 TTS 音频送回设备、断线重连。

**不负责**：音频要不要给 AI（voice）；业务工具（bridge/mcp）；智能体管理（device、linxapi）。

## 接口

```go
// 设备侧连接，由 gateway.Session 满足
type DeviceLink interface { DeviceID(); Send(msg) error; SendAudio(frame) error; Context() context.Context }

func New(cfg Config) *Bridge                          // Config{URL, Token}，来自 YAML
func (b *Bridge) Open(ctx, dev DeviceLink, p Persona) error   // 后台连接；已存在则 ErrAlreadyOpen
func (b *Bridge) Close(d)                             // 返回时该设备所有 goroutine 已退出
func (b *Bridge) Forward(d, frame)                    // voice.AI：热路径，未连接/未收音/队列满即丢
func (b *Bridge) Listen(d, on bool)                   // 开关灵矽收音并切灯
func (b *Bridge) Interrupt(d)                         // 中止 AI 播报并丢弃本轮剩余 TTS
func (b *Bridge) Available(d) bool                    // 当前是否已连上
func (b *Bridge) Say(ctx, d, text) error              // 不经 LLM 直接播报；未连上则等待
func (b *Bridge) SendMCP(d, payload) error            // bridge/mcp 的回复发回灵矽
func (b *Bridge) OnAIState(fn func(d, AIState))       // AIIdle | AISpeaking
func (b *Bridge) OnMCP(fn func(d, payload))           // 灵矽发来的 mcp
```

## 数据结构

```go
// 由 app 从 device.Profile 映射，bridge 因此不依赖 device
type Persona struct {
    AgentID   string   // X-Agent-ID；空则用平台上该 MAC 已绑定的智能体
    DollName, KidName string
    VoiceStep string   // 仅引导智能体：声音登记这一步的指令，注入 {{voice_step}}
}

type link struct {               // 一台设备的全部状态，跨越多次重连
    persona; ctx, cancel, done
    cur       atomic.Pointer[conn]   // 当前连接，nil = 未连上
    listening, speaking, muted, platformErr atomic.Bool
    ready     chan struct{}          // 连上时关闭；断线后换新的（Say 等待用）
}
type conn struct { ws; session string; out chan outFrame(32); closed chan struct{} }
```

`links` 用 `sync.Map`：`Forward` 是热路径，读不加锁。

## 关键流程

### 会话与重连

```text
link.run():
    delay = 1s
    loop:
        connected, err = session()
        if ctx 结束: return
        if !connected 且本轮尚未报告: dev.Send(error{internal, "ai service unavailable"})   // 只报一次
        if 本次连接稳定 ≥ 30s: delay = 1s
        sleep(delay);  delay = min(delay*2, 30s)

session():
    ws = dial(headers)           // Device-Id=MAC, Client-Id=UUID, Protocol-Version=1, Authorization, X-Agent-ID
    send hello{ features:{mcp:true}, agent_params.custom_replace_prompt{doll_name, kid_name, voice_step} }
    读 welcome（8s 超时），不是 hello 则失败
    启动唯一写 goroutine；cur = conn
    dev.Send(audio.downlink{sample_rate, frame_duration})       // 必须早于任何下行音频
    若设备此前已开麦：补发 listen start
    readLoop()                    // 阻塞到断线
    退出时：cur = nil；ready 换新；清 muted / platformErr；若正在说话则通知设备 stop
```

### 灵矽下行 → 设备

| 灵矽消息 | 动作 |
| --- | --- |
| 二进制帧 | `muted` 为真则丢；否则 `dev.SendAudio` |
| `stt` 且 `error_code != 0` | 平台错误（如设备未绑定智能体）：`platformErr = muted = true`，向设备发 `error`，不播给孩子 |
| `stt` 有文本 | 灯 `thinking`（文本本身不转发，设备没有屏幕） |
| `tts start` | 清 `muted`；`speaking` 置真则：灯 `speaking`、`speaker.state{start, source:ai}`、`OnAIState(Speaking)`；`platformErr` 时忽略 |
| `tts stop` | `stopSpeaking()` |
| `mcp` | `onMCP(payload)`（交给 bridge/mcp，在读循环里调用，不得阻塞） |

`stopSpeaking`：`speaker.state{stop, source:ai}`；灯回 `listening` 或 `idle`；`OnAIState(Idle)`。

### 打断与说话

```text
Interrupt(d):  若 !speaking: return
               speaking=false; muted=true                     // 丢弃本轮剩余 TTS，直到下一次 tts start
               send abort{reason: interrupted}; stopSpeaking()

Say(d, text):  c = waitReady(ctx)                              // 未连上则等到连上或 ctx 结束
               send listen{state: detect, mode: realtime, text_response: text}
```

## 约束

- 所有对灵矽的写经**唯一写 goroutine** 与 32 项有界队列；`Forward` 用非阻塞 `trySend`，满了丢音频帧。
- `listen.mode = realtime`（全双工，可打断）。
- `hello.features.mcp` 总为真：设备不支持 MCP 时，代理仍向灵矽提供业务工具。
- 名字每次会话经 `agent_params` 注入（空值也要发，否则提示词留下 `{{占位符}}`）。交棒时 `Close` 再 `Open`，`Open` 取档案里最新的 `agent_id`。
- 设备断开（`dev.Context()` 取消）即停止重连并释放全部 goroutine。
- 超时：拨号 8s，写 5s，读上限 1MB。
- 日志只记元数据，不记音频。
