# 模块规格

> 每个模块按同一模板写：**职责、不负责、接口、数据结构、规则、事件、依赖**。没有内容的项省略。
> 分层与依赖规则见 [architecture.md §3](architecture.md#3-分层与依赖)，完成状态见 [tasks.md](tasks.md)，灵矽的事实见 [linx-integration.md](linx-integration.md)。

## 总览

| 层 | 模块 | 一句话 |
| --- | --- | --- |
| 接入层 | [gateway](#gateway) | 设备 WebSocket 接入与会话 |
| | [parentapi](#parentapi) | 家长端 REST |
| | [world](#world) | 演示地图的只读观察通道 |
| 编排层 | [app](#app) | 分发设备消息、把领域事件翻译成下行消息、组装端口 |
| 能力层 | [voice](#voice) | **语音核心**：音频去 AI 还是频道、发言权、转发给好友 |
| | [bridge/linx](#bridgelinx) | 每台设备一条灵矽会话 |
| | [bridge/mcp](#bridgemcp) | 设备端 MCP 代理与业务工具 |
| 领域层 | [device](#device) | 设备档案、自动开通、起名 |
| | [relation](#relation) | 好友关系 |
| | [channel](#channel) | 区域频道与群、前台频道 |
| | [zone](#zone) | 位置到区域 |
| | [history](#history) | 今日回顾 |
| | [share](#share) | 画面分享 |
| 基础层 | [store](#store) | 持久化 |
| | [linxapi](#linxapi) | 灵矽 REST 客户端 |
| | [protocol](#protocol) | 设备协议的 Go 类型、编解码、计划中的增量 |
| | [id](#id) | 公共标识类型 |

**全局约定**

- 接口方法首参为 `context.Context`，热路径方法（音频帧、`AreFriends`、`Members`）除外。
- 热路径不加长锁、不打日志、不阻塞；错误带上下文返回，不吞。
- 领域模块用 `Subscribe() <-chan Event` 发事件：有界 channel，只有 `app` 订阅，随进程 context 关闭。
- 每个模块的存储都经 `store` 的仓储接口，测试用假实现。

---

## 接入层

### gateway

**职责** 接受设备 WebSocket；10 秒内完成 `hello` 握手并回 `hello.ack`；经 `protocol` 校验每条上行消息，非法消息回 `error` 并保持连接；维护会话表；把消息与音频帧交给唯一的 `Handler`（即 `app`）。

**不负责** 好友、频道、AI、开通；它只知道"这是哪台设备"。

**接口**
```go
type Handler interface {
    OnConnect(s *Session)
    OnMessage(s *Session, msg protocol.Message)
    OnAudio(s *Session, frame []byte)       // 热路径
    OnDisconnect(s *Session)
}
func (g *Gateway) Session(d id.DeviceID) (*Session, bool)
func (s *Session) DeviceID() id.DeviceID
func (s *Session) Capabilities() []string
func (s *Session) Send(msg protocol.Message) error  // 写串行化
func (s *Session) SendAudio(frame []byte) error
func (s *Session) Context() context.Context         // 连接断开即取消
```
`Session` 的方法集恰好满足 `bridge/linx.DeviceLink`，`app` 直接把会话传过去。

**规则** 同一设备同时只允许一条连接，后来的连接被拒绝（`unauthorized`）。连接断开时先调 `OnDisconnect`，再取消会话 context。

**依赖** protocol、id。

---

### parentapi

**职责** 家长端 REST；Web 手机窗口与将来的小程序调用同一套。

**不负责** 业务规则；只做鉴权、归属校验、编排与序列化。

**接口**

| 方法与路径 | 说明 | 调用 |
| --- | --- | --- |
| `GET /api/kids` | 家长名下的孩子与玩偶 | `device` |
| `GET /api/kids/{id}/friends` | 好友列表，含待确认 | `relation.Friends` |
| `POST /api/friend-requests/{id}/confirm` | 确认好友 | `relation.Confirm` |
| `DELETE /api/kids/{id}/friends/{fid}` | 解除好友 | `relation.Remove` |
| `GET /api/kids/{id}/groups` | 所在群聊 | `channel.Current` |
| `GET /api/kids/{id}/digest?date=today` | 今日回顾 | `history.Today` |

**规则** 演示用固定令牌，将来用微信登录。每个接口都校验"该孩子属于此家长"，不属于时返回 404 而不是 403，避免探测设备是否存在。

**依赖** device、relation、channel、history。

---

### world

**职责** 演示用观察通道：把设备位置、谁在说话、频道成员实时推给前端地图。

**不负责** 任何业务判断；不属于设备协议。

**接口** `GET /world` 升级为 WebSocket，推送：
```go
type WorldEvent struct {
    Kind     string        // "device" | "burst" | "member"
    Device   id.DeviceID
    Pos      *LatLng
    Channel  id.ChannelID
    Speaking bool
    Friend   *id.DeviceID  // 与视角设备的好友关系，前端据此标注"已屏蔽"
}
func (w *World) Publish(e WorldEvent) // 由 app 调用
```

**规则** 它会暴露所有设备的位置，**只在演示模式注册路由**，默认启动不开启。

**依赖** id。

---

## 编排层

### app

**职责**
1. 实现 `gateway.Handler`，把设备消息分发到对应模块；
2. 订阅领域事件，翻译成下行设备消息，并转给 `world` 与 `history`；
3. 启动时创建各模块、组装端口（见 [端口一览](architecture.md#3-分层与依赖)）。

**不负责** 业务判断。它只做"收到 X 就调用 Y"，不持有业务状态；出现 `if 是好友` 之类的判断，说明规则放错了地方。

**上行分发**

| 设备事件 | 调用 |
| --- | --- |
| 连接建立（`hello` 已通过） | `device.Ensure` → `bridge/linx.Open`；`stage=fresh` 时 `Say` 由引导智能体欢迎并请孩子起名；`stage=named` 时直接 `Handoff`。失败时下发 `error` |
| `mic.state`、`wake`、音频帧 | `voice` |
| `proximity` | `relation.OnProximity` |
| `location` | `zone.OnLocation`；同时 `world.Publish` |
| `mcp` | `bridge/mcp.OnDeviceMCP` |
| 断开 | `voice`、`bridge/linx`、`channel` 清理该设备 |

**事件翻译**

| 领域事件 | 下行 / 去向 |
| --- | --- |
| `channel` Joined / Left / Switched | 本设备 `channel.state` + `sound`；同频道好友 `presence`（成员先经 `relation.FilterFriends`）；`world`、`history` |
| `relation` Proposed / Confirmed / Removed | 双方 `friend.state` + `haptic`；Removed 同时通知 `voice` |
| `zone` Entered | `Say` 询问是否加入区域频道；`stage` 未到 `ready` 的设备忽略 |
| `device` Named | 等 `OnAIState` 回到 idle → `device.Handoff` → `bridge/linx.Close` + `Open`（新 `agent_id`）→ `Say` 第一句问候 |
| `voice` BurstStarted / BurstEnded | `world` |

**依赖** 下层所有模块。代码放 `server/internal/app`，`cmd/palpabled/main.go` 只读配置并调用它。

---

## 能力层

### voice

**这是多人频道语音对话的核心。** 回答三件事：设备的话说给谁听、频道里同时有人说话怎么办、怎么把声音送到好友设备。

**职责**
1. **上行路由**：每帧麦克风音频去 AI、去频道，还是丢弃。
2. **发言权**：同一频道同一时刻只有一个人在说话（对讲机式）。
3. **下行转发**：把发言人的 Opus 帧原样转给同频道且是好友的参与者；保证一台设备同一时刻只收一种来源的音频。

**不负责** 谁是好友（relation）、谁在频道（channel）、AI 怎么回答（bridge/linx）；不解码、不混音、不转码，只透传 Opus。

**路由规则**

| 设备状态 | 麦克风音频去向 | 说明 |
| --- | --- | --- |
| 不在任何频道 | AI | 私密陪伴，免按键，回答只给自己听 |
| 在频道里 | 频道 | 默认说给好友听 |
| 在频道里，刚发出 `wake` | AI（一个回合） | AI 说完（`tts stop`）后 1.5 秒内没有新语音，回到频道 |
| 不在频道，且 AI 不可用 | 丢弃 | 开通失败或灵矽断线时 |

`wake` 由设备本地唤醒词引擎产生（Web 用点击玩偶或语音唤醒）。服务端不靠识别文本判断"是否在叫小伙伴"，这样频道里的闲聊不会被送给 AI。

**发言权**
- 设备用 `mic.state start/stop` 圈出一句话（设备端 VAD 决定）。服务端另有看门狗：路由到频道的音频 600ms 没有新帧，也视为说完。
- 频道空闲时，第一个 `start` 的人拿到发言权，成为 `Burst` 的发言人；说完后保留 600ms 拖尾，避免句中停顿被抢。
- 有人占着发言权时，其他人的音频帧丢弃，并向其发 `sound{busy}` 与 `haptic{tap}`。
- 发言人断线或离开频道，立刻结束该 `Burst`。

**下行转发**
- `Burst` 开始向每个听者发 `speaker.state{start, source:peer, from}`，结束发 `stop`。
- 听者 = 发言人前台频道的成员 ∩ `AreFriends(发言人, 听者)`，**每个 `Burst` 开始时算一次**；中途加入频道的人从下一个 `Burst` 开始收听；好友被解除时立即把对方移出。
- 每个听者一条有界队列（8 帧 ≈ 480ms），满了丢最旧帧；一个慢听者不能拖慢其他人。
- 听者正在播 AI 语音时，好友发言**优先**：先 `AI.Interrupt`，再开始好友音频。同一设备同一时刻只有一种来源，所以设备靠 `speaker.state.source` 就能区分，音频帧不需要加头。

**延迟目标** 服务端内部 < 100ms（不含网络），端到端 < 300ms。每个 `Burst` 记录首帧收到与首帧发出的时间戳。

**接口**
```go
type Router interface {
    OnMicState(d id.DeviceID, on bool)
    OnWake(d id.DeviceID)
    OnMicFrame(d id.DeviceID, frame []byte)       // 热路径
    OnAIState(d id.DeviceID, s AIState)           // bridge/linx 回报：idle / speaking
    OnFriendRemoved(a, b id.DeviceID)             // app 转发 relation 事件
    OnLeave(d id.DeviceID)                        // 断线或离开频道
    RouteOf(d id.DeviceID) Route
    Subscribe() <-chan Event
}

// 端口：由 app 注入
type Membership interface {
    ForegroundOf(p id.ParticipantID) (id.ChannelID, bool)
    Members(ch id.ChannelID) []id.ParticipantID
}
type Friends interface { AreFriends(ctx context.Context, a, b id.ParticipantID) (bool, error) }
type AI      interface { Forward(d id.DeviceID, frame []byte); Interrupt(d id.DeviceID); Available(d id.DeviceID) bool }
type Sink    interface {
    Send(p id.ParticipantID, m protocol.Message) error
    SendAudio(p id.ParticipantID, f []byte) error
}
```

**数据结构**
```go
type Route int  // RouteAI | RouteChannel | RouteNone
type Source int // SourceNone | SourceAI | SourcePeer

type deviceVoice struct {
    route       Route
    aiTurnUntil time.Time   // wake 之后 AI 回合的截止时间
    micOn       bool
    down        chan []byte // 有界下行队列
    source      Source      // 当前正在下发的来源
}

type Burst struct {
    Channel     id.ChannelID
    Speaker     id.ParticipantID
    Listeners   []id.ParticipantID
    StartedAt   time.Time
    LastFrameAt time.Time
    FirstOutAt  time.Time   // 首帧发出时间，用于延迟统计
}
```

**事件** `BurstStarted{channel, speaker, listeners}`、`BurstEnded{channel, speaker, firstInAt, firstOutAt}`。

**依赖** protocol、id；端口 `Membership`（channel）、`Friends`（relation）、`AI`（bridge/linx）、`Sink`（gateway 会话）。

---

### bridge/linx

**职责** 为每台已开通的设备维护一条灵矽会话：握手、转发 `voice` 交来的音频、把灵矽的 `stt/tts` 翻译成灯光与 `speaker.state`、把 TTS 音频送回设备；断线重连。

**不负责** 决定音频要不要给 AI（voice）；业务工具（bridge/mcp）；智能体管理（device、linxapi）。

**接口**
```go
// 端口：设备侧连接，由 gateway.Session 满足
type DeviceLink interface {
    DeviceID() id.DeviceID
    Send(msg protocol.Message) error
    SendAudio(frame []byte) error
    Context() context.Context
}

func (b *Bridge) Open(ctx context.Context, dev DeviceLink, p device.Profile) error // 开通成功后由 app 调用
func (b *Bridge) Close(d id.DeviceID)
func (b *Bridge) Forward(d id.DeviceID, frame []byte)   // voice.AI，热路径
func (b *Bridge) Interrupt(d id.DeviceID)               // 中止 AI 播报并停止下发
func (b *Bridge) Available(d id.DeviceID) bool
func (b *Bridge) Say(ctx context.Context, d id.DeviceID, text string) error // 不经 LLM，直接合成
func (b *Bridge) SendMCP(d id.DeviceID, payload json.RawMessage) error     // bridge/mcp 回给灵矽
func (b *Bridge) OnAIState(fn func(d id.DeviceID, s AIState))
func (b *Bridge) OnMCP(fn func(d id.DeviceID, payload json.RawMessage))    // 灵矽发来的 mcp
```

**数据结构**
```go
type Config struct { URL, Token string } // 来自 YAML 配置；智能体 ID 取自设备档案
type link struct {
    dev   DeviceLink
    ws    *websocket.Conn
    input <-chan []byte // 上行音频来源，现在是本设备麦克风；预留替换为频道发言
    ready, listening, speaking bool
}
type AIState int // AIIdle | AISpeaking
```

**规则**
- 握手头：`Authorization`、`Protocol-Version: 1`、`Device-Id`（设备自身 MAC）、`Client-Id`（UUID）、`X-Agent-ID`（档案中的智能体）。`listen.mode=realtime`。
- `hello.agent_params.custom_replace_prompt` 注入 `doll_name`、`kid_name`，引导智能体另注入 `voice_step`（是否做声音登记）；起名后在下次会话生效，当前会话里 AI 已从对话中得知名字。交棒时 `Close` 再 `Open` 即换智能体，`Open` 总是读档案里最新的 `agent_id`。
- 灵矽断线后指数退避重连，设备断开时停止重连并释放所有 goroutine。

**依赖** protocol、id、device（只读 `Profile` 类型）；端口 `DeviceLink`。

---

### bridge/mcp

**职责** 代设备做 MCP：解析灵矽与设备之间的 `mcp` JSON-RPC，把设备工具与业务工具合并成一份列表，按工具名路由调用。

**不负责** 业务规则本身（交给领域模块）；不解析设备工具返回的内容。

**接口**
```go
type Tool struct {
    Name        string          // self.<模块>.<功能>
    Description string          // 儿童友好、不引导说出个人信息
    InputSchema json.RawMessage
    Call        func(ctx context.Context, d id.DeviceID, args json.RawMessage) (ToolResult, error)
}
type ToolResult struct { Text string; IsError bool }

func (p *Proxy) Register(t Tool)
func (p *Proxy) OnDeviceMCP(d id.DeviceID, payload json.RawMessage) // 设备 → 灵矽
func (p *Proxy) OnLinxMCP(d id.DeviceID, payload json.RawMessage)   // 灵矽 → 设备
```

**规则**
- `initialize`、`tools/list`：先取完设备的全部分页，再合并业务工具回复灵矽。
- `tools/call`：名字属于业务工具 → 服务端执行；否则转给设备，结果原样回灵矽。
- 调用身份来自连接，参数里没有设备 ID。设备工具与业务工具重名时以设备为准并记告警。
- 设备不支持 MCP（`capabilities` 无 `mcp`）时，代理仍向灵矽提供业务工具。

**业务工具**

| 工具 | 参数 | 调用 | 返回给 AI |
| --- | --- | --- | --- |
| `self.profile.set_names` | `doll_name` `kid_name` | `device.SetNames` | 已记住 |
| `self.voiceprint.enroll` | — | `device.EnrollVoice` | 登记成功，或原因（未同意 / 没取到声音）；仅引导智能体会被提示使用 |
| `self.zone.join` | — | `zone.JoinCurrent` | 成功，或不在任何区域 |
| `self.group.create` | `ttl`：trip / today / forever | `channel.CreateGroup` | 4 位码，并让设备播报 |
| `self.group.join` | `code` | `channel.JoinGroup` | 成功或失败原因 |
| `self.group.switch` | `target`：群名 / next / prev | `channel.Switch` | 当前群名 |
| `self.group.current` | — | `channel.Current` | 当前群与在线好友 |
| `self.friends.list` | — | `relation.Friends` | 好友名字 |
| `self.friends.nearby` | — | `relation.FilterFriends(channel.Members)` | 附近的好友 |
| `self.share.publish` | `description` | `share.Publish` | 已分享 |

**依赖** id、bridge/linx（`SendMCP`）、device、zone、channel、relation、share。下发给设备的 `mcp` 经 gateway 会话，由 `app` 注入。

---

## 领域层

### device

**职责** 设备档案与自动开通：新设备先绑定引导智能体；起名后交棒，按皮套选风格智能体并切换；换皮套时切换智能体；记录小伙伴与孩子的名字；登记孩子的声纹。流程见 [architecture.md §4](architecture.md#4-设备如何组织)。

**不负责** 灵矽 REST 细节（linxapi）；打招呼与语音交互（由 app 调 bridge/linx）；重连灵矽会话（交棒后由 app 通知 bridge/linx）；好友与频道。

**接口**
```go
type Devices interface {
    Ensure(ctx context.Context, info DeviceInfo) (Profile, error) // 幂等
    Profile(ctx context.Context, d id.DeviceID) (Profile, error)
    SetNames(ctx context.Context, d id.DeviceID, doll, kid string) error // 写档案，stage=named，发 Named 事件
    Handoff(ctx context.Context, d id.DeviceID) (Profile, error)         // 幂等：切到风格智能体，写记忆焦点，stage=ready
    EnrollVoice(ctx context.Context, d id.DeviceID) error                // 无监护人同意返回 ErrNoConsent
    Bind(ctx context.Context, d id.DeviceID, g id.GuardianID) error
    Subscribe() <-chan Event // Named
}

type Catalog interface {
    EnsureAll(ctx context.Context) error // 启动时确保引导智能体与每个风格的智能体存在
    Onboarding() Style                   // 引导智能体，不随 skin 变化
    ForSkin(skin string) Style           // 未知 skin 返回默认风格
}
```

**数据结构**
```go
type DeviceInfo struct { MAC, Skin, Firmware string } // 来自 hello

type Style struct {
    ID           string // "dragon" ...；引导智能体为 "welcome"
    Skin         string // 引导智能体为空
    AgentName    string // 引导智能体暂定"小可"
    SystemPrompt string // 儿童友好，带 {{doll_name}} {{kid_name}} 占位；引导智能体另带 {{voice_step}}
    VoiceID      string
    AgentID      string // EnsureAll 填充
}

type Stage int // StageFresh → StageNamed → StageReady

type Profile struct {
    ID           id.DeviceID // = MAC
    Skin         string
    StyleID      string      // fresh / named 时为 "welcome"，ready 后为风格
    AgentID      string      // 当前绑定的智能体
    DollName     string
    KidName      string
    Guardian     id.GuardianID
    VoiceConsent bool   // 监护人是否同意声纹登记
    SpeakerID    string // 声纹说话人，未登记为空；不绑定到智能体
    Stage        Stage
}
```

**规则**
- 灵矽预注册对已存在的设备返回冲突时，按"已开通"处理并核对绑定的智能体。
- 名字只接受小伙伴名与孩子的称呼，不存其他个人信息。
- `SetNames` 在 `fresh` 时进入 `named`；`Handoff` 在 `named` 时进入 `ready`，已是 `ready` 直接返回。名字写入记忆焦点放在 `Handoff` 里，因为记忆容器属于风格智能体。
- `EnrollVoice` 只在 `VoiceConsent` 为真时调用 linxapi 登记，失败不改变 `Stage`；`Profile` 只存 `SpeakerID`，不存音频与声纹特征。

**依赖** store、linxapi、id。

---

### relation

**职责** 好友关系的状态机与查询；把"两台设备靠近"转成待确认的好友请求。

**不负责** 频道与音频；谁有权确认（由 parentapi 鉴权后调用 `Confirm`）。

**接口**
```go
type Relations interface {
    AreFriends(ctx context.Context, a, b id.ParticipantID) (bool, error)  // 热路径，内存缓存
    FilterFriends(ctx context.Context, viewer id.ParticipantID, ps []id.ParticipantID) []id.ParticipantID
    Friends(ctx context.Context, d id.DeviceID) ([]Friendship, error)
    OnProximity(ctx context.Context, d id.DeviceID, peers []Peer) error
    Confirm(ctx context.Context, reqID string, by id.GuardianID) error
    Remove(ctx context.Context, a, b id.DeviceID) error
    Subscribe() <-chan Event // Proposed / Confirmed / Removed
}
```

**数据结构**
```go
type Status int // StatusPending | StatusConfirmed | StatusRemoved

type Friendship struct {
    ID          string
    A, B        id.DeviceID // 存储时 A < B，一对只有一条
    Status      Status
    ConfirmedBy id.GuardianID
    CreatedAt   time.Time
}

type Peer struct { Device id.DeviceID; Strength float64 } // 0..1，越大越近
```

**规则**
- 参与者先经 `id.OwnerDevice` 解析为设备再判断。
- 某个 `Strength ≥ 0.8` 持续 1.5 秒，且**双方都上报了对方**，才生成 `Pending`；已是好友或已有未处理请求则忽略。
- 只有 `Confirm` 能让 `AreFriends` 为真；`Remove` 立即生效并发事件。

**依赖** store、id。

---

### channel

**职责** 频道与群的成员；区域频道的加入与离开；群的创建、4 位码入群、过期；每台设备的前台频道。

**不负责** 音频与发言权（voice）；好友（relation）；下发设备消息（app 订阅事件后下发）。`channel` 只回答"谁在哪个频道"。

**接口**
```go
type Channels interface {
    JoinZone(ctx context.Context, d id.DeviceID, ch id.ChannelID) error
    LeaveZone(ctx context.Context, d id.DeviceID) error
    CreateGroup(ctx context.Context, owner id.DeviceID, ttl GroupTTL) (code string, err error)
    JoinGroup(ctx context.Context, d id.DeviceID, code string) error
    Switch(ctx context.Context, d id.DeviceID, target string) error // 群名、"next"、"prev"
    Leave(ctx context.Context, d id.DeviceID, ch id.ChannelID) error
    Current(ctx context.Context, d id.DeviceID) (ChannelInfo, error)
    ForegroundOf(p id.ParticipantID) (id.ChannelID, bool) // 热路径
    Members(ch id.ChannelID) []id.ParticipantID          // 热路径
    Subscribe() <-chan Event                              // Joined / Left / Switched
}
```

**数据结构**
```go
type Kind int     // KindZone | KindGroup
type GroupTTL int // TTLTrip | TTLToday | TTLForever

type Channel struct {
    ID        id.ChannelID
    Kind      Kind
    Name      string
    Code      string      // 仅群：4 位数字
    Owner     id.DeviceID // 仅群
    ExpiresAt time.Time   // 仅群；零值表示长期
}

type ChannelInfo struct { Channel Channel; Members []id.ParticipantID; Foreground bool }
```

**规则**
- 成员是参与者；设备加入时是 `dev:<mac>`。
- 一台设备可在多个频道，**前台只有一个**；说话和收听只针对前台频道。
- 4 位码在有效期内唯一，过期失效；同一设备 1 分钟内错码超过 5 次则暂时拒绝。
- 区域频道成员只在内存；群与码持久化。

**依赖** store、id。

---

### zone

**职责** 把设备坐标映射到区域，处理边界抖动，在孩子同意后把设备加入区域频道。

**不负责** 频道成员管理（channel）；询问孩子（app 收到 `Entered` 后让小伙伴问）。

**接口**
```go
type Zones interface {
    OnLocation(ctx context.Context, d id.DeviceID, lat, lng float64) error
    JoinCurrent(ctx context.Context, d id.DeviceID) error // 不在任何区域时返回 ErrNotInZone
    ZoneAt(lat, lng float64) (ZoneID, bool)
    Subscribe() <-chan Event // Entered / Left
}
```

**数据结构**
```go
type ZoneID string                                            // 对应频道 "zone:<id>"
type Zone struct { ID ZoneID; Name string; Polygon []LatLng } // 演示只有一个：公园
```

**规则** 进出各设 20 米滞回。进入只发 `Entered` 事件，等孩子同意；离开直接 `channel.LeaveZone`。

**依赖** channel、id。

---

### history

**职责** 只追加的事件日志，生成今日回顾：遇到哪些小朋友、分享了什么、聊了什么。

**不负责** 生成摘要（先展示原话）；谁能看（parentapi）。

**接口**
```go
type History interface {
    Record(ctx context.Context, e Event) error
    Today(ctx context.Context, d id.DeviceID) (Digest, error)
}
```

**数据结构**
```go
type EventKind int // EventMetFriend | EventSharedScene | EventChat

type Event struct {
    ID      string
    Device  id.DeviceID
    Kind    EventKind
    Payload json.RawMessage // MetFriend{Friend, Channel} / SharedScene{Text, Channel} / Chat{Lines}
    At      time.Time
}

type Digest struct { Met []MetFriend; Shared []SharedScene; Chats []ChatLine }
```

**规则** 遇到好友、分享由 `app` 转来的事件记录。聊天从 `linxapi.Sessions` 拉取，必须能按设备筛选才展示，否则会混入共用同一风格智能体的其他孩子的对话（见 [开放问题](architecture.md#11-开放问题)）。

**依赖** store、linxapi、id。

---

### share

**职责** 画面分享：孩子让小伙伴描述眼前的画面，描述播给前台频道里的好友，也播给孩子自己，并记入历史。

**不负责** 拍照与看图（设备工具与灵矽视觉）；语音转发（用 `Announcer`）。

**接口**
```go
type Announcer interface { Say(ctx context.Context, d id.DeviceID, text string) error } // 端口

func (s *Share) Publish(ctx context.Context, from id.DeviceID, description string) error
```

**规则** AI 用设备工具 `self.camera.take_photo` 得到描述 → 调 `self.share.publish` → 取发送者前台频道成员并经 `relation.FilterFriends` → 对每台设备（含发送者）`Say("<小伙伴名>看到：…")` → `history.Record`。

**依赖** channel、relation、device、history、id；端口 `Announcer`（bridge/linx）。

---

## 基础层

### store

**职责** 持久化接口与 SQLite 实现；迁移；每个领域模块一个仓储接口，并提供假实现。

| 表 | 关键字段 |
| --- | --- |
| `device` | `device_id`（MAC） `skin` `style_id` `agent_id` `doll_name` `kid_name` `stage` `guardian_id` `voice_consent` `speaker_id` `created_at` |
| `style` | `style_id` `skin` `agent_id`（含 `welcome` 引导智能体） |
| `guardian` | `guardian_id` `display_name` |
| `friendship` | `id` `a` `b` `status` `confirmed_by` `created_at` |
| `channel_group` | `channel_id` `name` `owner` `code` `expires_at` |
| `channel_member` | `channel_id` `participant_id` `joined_at`（只存群成员；区域频道成员在内存） |
| `history_event` | `id` `device_id` `kind` `payload` `at` |

**规则** 声纹与原始音频不入库；记忆本体在灵矽，不重复保存。删除设备时同一事务删除其好友、群成员与历史。只存取记录、不含业务规则：阶段、好友状态、历史类型以整数存放，含义由所属领域模块定义；好友对由仓储规范为 `A < B`。`NewMemory()` 是假实现，与 SQLite 跑同一套契约测试。

**依赖** id。

---

### linxapi

**职责** 灵矽 REST 的薄客户端，只做协议转换，不含业务。

**接口**
```go
type Client interface {
    CreateAgent(ctx context.Context, a AgentSpec) (agentID string, err error)
    FindAgentByName(ctx context.Context, name string) (agentID string, found bool, err error)
    PreregisterDevice(ctx context.Context, agentID, mac string) error
    SwitchAgent(ctx context.Context, mac, agentID string, migrateHistory bool) error
    EnableMemory(ctx context.Context, agentID string) (memoryID string, err error)
    UpsertAttention(ctx context.Context, memoryID, name, description string) (attentionID string, err error)
    SetAttentionValue(ctx context.Context, memoryID, mac, attentionID, value string) error
    AddMemoryNode(ctx context.Context, memoryID, mac, content string) error
    CreateSpeaker(ctx context.Context, name string) (speakerID string, err error)
    EnrollVoiceprint(ctx context.Context, speakerID, audioURL string) error
    BindSpeakers(ctx context.Context, agentID string, speakerIDs []string) error
    SetOwnerOnly(ctx context.Context, agentID string, on bool) error
    RecentVoiceSamples(ctx context.Context, agentID string) ([]VoiceSample, error)
    Sessions(ctx context.Context, agentID string, page, limit int) ([]Session, error)
    SessionMessages(ctx context.Context, agentID, sessionID string) ([]Message, error)
}
```
`CreateSpeaker`、`EnrollVoiceprint`、`RecentVoiceSamples` 服务于引导期的声音登记（S12）；`BindSpeakers`、`SetOwnerOnly` 服务于可选的声纹认主，黑客松版不调用。引导智能体被所有新设备共用，`RecentVoiceSamples` 必须能确认样本属于当前设备，否则不登记。

**数据结构**
```go
type AgentSpec   struct { Name, AssistantName, SystemPrompt, MemoryModel string }
type VoiceSample struct { AudioURL string; At time.Time }
type Message     struct { Role, Text, AudioURL string; At time.Time }
```

**规则** HTTP 恒为 200，以响应体 `code` 判断；`401` 视为令牌失效；所有调用设超时并重试一次。令牌只来自 YAML 配置文件。

**依赖** 无。

---

### protocol

**职责** 设备协议的 Go 类型；解码并校验上行消息，编码下行消息。以 `protocol/palpable.schema.json` 为准，Go 类型手写，测试对照 schema（见 [protocol/README.md](../protocol/README.md)）。

**不负责** 任何路由与业务判断。

**接口**
```go
func DecodeUpstream(data []byte) (Message, error) // 未知类型 / 未知字段 / 越界值 → ErrBadMessage
func Encode(msg Message) ([]byte, error)
```

**计划中的协议增量**（落地时先改 schema，再从本表删除）

| 方向 | 消息 | 变更 | 用途 | 任务 |
| --- | --- | --- | --- | --- |
| 上行 | `hello` | 增加 `skin`；`device_id` 限定为 MAC（小写冒号分隔） | 自动开通 | S2 |
| 上行 / 下行 | `mcp` | 新增，`payload` 为 JSON-RPC | 设备端 MCP | S7 |
| 下行 | `channel.state` | 新增：`name` `color` `event`（joined / switched / left） | 入群、切群的灯色与提示 | S4 |
| 下行 | `friend.state` | 新增：`event`（pending / confirmed / removed） `device_id` `name` | 成为好友的振动与提示 | S3 |
| 下行 | `sound` | `name` 增加 `busy` | 占线提示 | S5 |
| 下行 | `speaker.state` | 可选 `from_kind`（device / agent） | 预留：智能体入频道时才加 | — |

**依赖** 无。

---

### id

**职责** 跨模块共用的标识类型，避免模块之间为了一个类型互相 import。

```go
type DeviceID string      // 设备 MAC，与 hello.device_id 一致
type ChannelID string     // "zone:park"、"group:<id>"
type GuardianID string
type ParticipantID string // "dev:<mac>"；预留 "agt:<mac>"

type Kind int // KindDevice | KindAgent（预留）
func (p ParticipantID) Kind() Kind
func Device(d DeviceID) ParticipantID
func OwnerDevice(p ParticipantID) DeviceID // 好友判断与家长控制都以它为准
```

**依赖** 无。

---

## 前端模块

`web/src/` 下的模块。设备类实现协议，只由服务端下行消息驱动；Web 展示类只存在于演示，不进协议。

| 模块 | 类别 | 职责 |
| --- | --- | --- |
| `shared` | — | 生成的协议类型、WS 客户端、zustand store |
| `device-sim` | 设备 | 虚拟设备：麦克风与扬声器、设备端 VAD 与 `mic.state`、`wake`、`proximity`、`location`、按 `audio.downlink` 解码、设备端 MCP 服务 |
| `plush` | 设备表现 | 玩偶与灯带，响应 `light`、`speaker.state`、`haptic` |
| `scenes/unbox` | Web 展示 | 开箱 |
| `scenes/desk` | Web 展示 | 桌面场景、起名引导的画面 |
| `scenes/park` | Web 展示 | 公园，多个小朋友走动、靠近，提供场景截图 |
| `map` | Web 展示 | 地图，显示位置、谁在说话、"已屏蔽"（订阅 `/world`） |
| `parent-sim` | Web 展示 | 手机窗口，调用 `/api` |
| `ui` | Web 展示 | 儿童友好的统一文案表与提示音 |

`device-sim` 不得读取其他虚拟设备的状态；同一页面里的多个小朋友窗口也只通过服务端互通。
