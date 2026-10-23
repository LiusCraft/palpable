# 模块规格

> 每个模块只写五件事：**职责、不负责、接口、数据结构、依赖**。
> 灵矽对接的事实与约束见 [linx-integration.md](linx-integration.md)，任务排期见 [tasks.md](tasks.md)，设备协议见 [protocol/README.md](../protocol/README.md)。

状态：✅ 已完成　🚧 部分完成　⬜ 待做

## 总览

```
设备(Web 模拟器/固件)
        │ WebSocket：JSON 控制消息 + Opus 音频帧
     gateway ───────────────────────────────┐
        │ 上行：mic.state / wake / 音频帧    │ 下行：light / speaker.state / 音频帧
        ▼                                    │
      voice ──┬─ AI 路由 ──▶ bridge/linx ──▶ 灵矽（听说、记忆、声纹、视觉）
   （语音路由   │                 ▲
    与发言权）  │         bridge/mcp（工具：自带设备工具 + 业务工具）
              └─ 频道路由 ─▶ channel ──▶ relation（谁是好友）
                              ▲
                            zone（位置 → 区域频道）

 device（档案与自动开通）  linxapi（灵矽 REST）  history（今日回顾）  share（画面分享）
 parentapi（家长端 REST）  world（演示观察通道）  store（SQLite）
```

| 模块 | 一句话 | 状态 |
| --- | --- | --- |
| [protocol](#1-protocol) | 设备协议类型与校验 | ✅ |
| [gateway](#2-gateway) | 设备 WebSocket 接入与会话 | ✅ |
| [voice](#3-voice) | **语音对话核心**：上行音频去 AI 还是去频道、频道内谁发言、把发言转给好友 | ⬜ |
| [channel](#4-channel) | 频道与群：成员、4 位码、前台群 | ⬜ |
| [relation](#5-relation) | 好友关系 | ⬜ |
| [zone](#6-zone) | 位置到区域频道 | ⬜ |
| [bridge/linx](#7-bridgelinx) | 设备与灵矽的会话桥接 | 🚧 |
| [bridge/mcp](#8-bridgemcp) | 设备端 MCP 代理与业务工具 | ⬜ |
| [device](#9-device) | 设备档案、按外观自动开通智能体、开箱引导 | ⬜ |
| [linxapi](#10-linxapi) | 灵矽 REST 客户端 | ⬜ |
| [history](#11-history) | 今日回顾的事件记录 | ⬜ |
| [share](#12-share) | 画面分享 | ⬜ |
| [parentapi](#13-parentapi) | 家长端 REST | ⬜ |
| [world](#14-world) | 演示用观察通道 | ⬜ |
| [store](#15-store) | 持久化 | ⬜ |

## 设备如何组织

**设备身份就是设备 MAC。** 真实设备用自己的 MAC；Web 模拟器首次打开时生成一个并存在浏览器里。设备连上后端时自报家门，后端据此**自动开通**，不需要人工配置。

**风格智能体。** 我们预置几种风格的智能体，与皮套外观一一对应，设备的外观（`skin`）默认决定它用哪个：

| skin | 风格智能体 | 说明 |
| --- | --- | --- |
| `dragon` | 小龙 | 每种风格有自己的系统提示词与音色 |
| `rabbit` | 兔兔 | |
| `bear` | 小熊 | |
| `cat` | 小猫 | |

服务端启动时确保每个风格的智能体在灵矽侧存在（按名字查找，缺失则创建），记下 `agent_id`。`skin` 未知或缺失时用默认风格。

**上线与自动开通流程**

```
设备 hello（device_id=MAC, skin, firmware）
   │
   ▼
device.Ensure(info)  ── 幂等
   ├─ 没有档案：按 skin 选风格智能体 → linxapi.PreregisterDevice(agent, mac) → 建档案（stage=fresh）
   ├─ 有档案但 skin 对应的智能体变了（换皮套）：linxapi.SwitchAgent → 更新档案
   └─ 有档案且一致：直接返回
   │
   ▼
bridge/linx 以该 MAC 连灵矽 → 正常对话；stage=fresh 时进入开箱引导
```

- **开通失败**：设备收到 `error`，灯变 `muted`，稍后自动重试。频道对讲不依赖 AI，所以开通失败时设备仍可进入频道与好友说话。
- **一个风格智能体对应很多设备**：记忆按设备 MAC 隔离（灵矽的记忆值与片段按 `identify_id` 区分）。
- **取舍一：声纹。** 声纹说话人绑定和"只回应已绑定说话人"都是智能体级配置，共用风格智能体时做不到"只有主人能和这个玩偶说话"。默认不做；需要时升级为"每台设备从风格模板克隆一个智能体"，其他流程不变。
- **取舍二：换皮套。** 切换智能体后，记忆容器是新的。可通过 `linxapi` 读写记忆片段把旧记忆搬过去，演示里换皮套很少，先不做。

公共类型（`server/internal/id`）：

```go
type DeviceID string  // 与 hello.device_id 一致
type ChannelID string // "zone:park"、"group:7326"
type GuardianID string

// 频道里的"参与者"。现在只有设备；将来设备专属的小伙伴智能体也可以作为参与者加入。
type ParticipantID string // "dev:<mac>"；预留 "agt:<mac>"
func (p ParticipantID) Kind() Kind     // KindDevice | KindAgent（预留）
func OwnerDevice(p ParticipantID) DeviceID // 参与者归属哪台设备，好友判断与家长控制都以它为准
```

全局约定：接口方法首参为 `context.Context`；热路径（音频帧）不加长锁、不打日志、不阻塞；错误带上下文返回，不吞。

---

## 预留：小伙伴智能体作为频道成员（暂不做）

将来每台设备的专属小伙伴可以**作为一个实体加入频道**，听大家说话、也参与对话。现在不做，但模块边界要为它留好接缝。

**怎么看智能体与频道的关系：** 智能体不是频道的一部分，而是和设备平级的**参与者**。频道只管"有哪些参与者"，不关心对方是孩子的设备还是小伙伴。

| 现在 | 将来加入智能体时 |
| --- | --- |
| 参与者只有设备：`dev:<mac>` | 多一种 `agt:<mac>`，由同一台设备的小伙伴扮演 |
| 听者收到的是设备的 Opus 帧，下发给设备 | 智能体听者收到同样的帧，喂给它的灵矽会话（ASR） |
| 发言人是设备，说话靠 `mic.state` | 智能体发言人是 TTS 音频，同样要先抢发言权 |
| `AreFriends(a, b)` 看设备 | 智能体按所属设备判断：`OwnerDevice(p)` |

**现在就要做对的五个接缝**

1. **频道成员用 `ParticipantID`，不用 `DeviceID`。** `channel.Members`、`voice.Burst` 的发言人与听者都是参与者。现在只有 `KindDevice`，改动成本最低。
2. **voice 面向 `Sink` 而不是 gateway 的会话。** 给参与者发消息与音频只走 `Sink`；设备的实现是 gateway 会话，将来智能体的实现是它的灵矽会话。voice 不知道对方是谁。
3. **好友关系经 `OwnerDevice` 解析。** `relation.AreFriends` 接受参与者，智能体视同它所属的设备，家长控制与屏蔽自动适用。
4. **灵矽会话的音频输入可替换。** `bridge/linx` 的会话把"喂给灵矽的音频来源"抽象成一个输入：现在只有"所属设备的麦克风"，将来可以是"频道里的发言"。灵矽的会话和身份不变，不会因为多开会话和同一个 MAC 冲突。
5. **发言权天然给智能体提供了干净输入。** 同一频道同一时刻只有一个人说话，所以智能体的 ASR 拿到的是一句一句串行的话，不会有混音。

**将来才需要新增的东西（现在不做）**

- 智能体何时开口的策略（被点名、停顿后、被问到）：`Policy.ShouldSpeak(ctx, agent, utterance) bool`，接口现在不写。
- 告诉智能体"是谁说的"：把发言人的名字随音频带给它。
- 协议：`speaker.state` 增加可选的 `from_kind`，设备据此用不同灯色表示"这是某个小伙伴在说话"。
- 隐私与家长控制：智能体听到什么、说了什么都要进历史，家长可关闭；这条必须在上线前定。

**不会变的**：设备协议、`gateway`、`relation` 的状态机、`zone`、家长端接口。

---

## 1. protocol

**职责** 定义设备与服务端之间的所有消息类型，解码并校验上行消息，编码下行消息。

**不负责** 任何路由与业务判断。

**接口**
```go
func DecodeUpstream(data []byte) (Message, error) // 未知类型/未知字段/越界值 → ErrBadMessage
func Encode(msg Message) ([]byte, error)
```

**数据结构** 以 `protocol/palpable.schema.json` 为准，Go 类型手写，测试对照 schema。

`hello` 是设备自报家门的消息，开通就靠它：
```go
type Hello struct {
    DeviceID     string      // 设备 MAC，规范化为小写冒号分隔
    Skin         string      // 外观：dragon / rabbit / bear / cat，缺省用默认风格
    Firmware     string
    AudioParams  AudioParams
    Capabilities []string
}
```

| 方向 | 已有 | 待增（见各模块与文末附录） |
| --- | --- | --- |
| 上行 | `hello` `wake` `mic.state` `proximity` `location` | `mcp`；`hello` 增加 `skin`，`device_id` 改为 MAC 格式 |
| 下行 | `hello.ack` `audio.downlink` `light` `haptic` `sound` `speaker.state` `presence` `error` | `channel.state` `friend.state` `mcp`；`sound.name` 增加 `busy` |

**依赖** 无。

---

## 2. gateway

**职责** 接受设备 WebSocket，完成 `hello` 握手，维护在线会话，把消息与音频帧交给 `Handler`，提供向设备发送的能力。

**不负责** 好友、频道、AI；它只知道"这是哪台设备"。

**接口**
```go
type Handler interface {            // 由各模块组合实现
    OnConnect(s *Session)
    OnMessage(s *Session, msg protocol.Message)
    OnAudio(s *Session, frame []byte)
    OnDisconnect(s *Session)
}
func (g *Gateway) Session(d DeviceID) (*Session, bool)
func (s *Session) Send(msg protocol.Message) error   // 写串行化
func (s *Session) SendAudio(frame []byte) error
func (s *Session) Context() context.Context          // 连接断开即取消
```

**数据结构**
```go
type Session struct { id string; deviceID DeviceID; capabilities []string; ctx context.Context }
```
会话表 `device_id → *Session`，同一设备只允许一条连接。

**依赖** protocol。

> 待改：现在只有一个 `Handler`，多个模块要订阅事件时，需要一个把事件分发给多个订阅者的组合器。

---

## 3. voice

**这是"多人频道语音对话"的核心。** 解决三件事：设备的话说给谁听、频道里同时有人说话怎么办、怎么把声音送到好友设备。

**职责**
1. **上行路由**：每帧麦克风音频去 AI 还是去频道。
2. **发言权**：同一频道同一时刻只有一个人在说话（对讲机式）。
3. **下行转发**：把发言人的 Opus 帧原样转给同频道、且是好友的设备；管理一台设备同时只收一种来源的音频（AI 或好友）。

**不负责** 谁是好友（relation）、谁在频道（channel）、AI 如何回答（bridge/linx）；不解码、不混音、不转码，只透传 Opus。发言人与听者都是**参与者**（`ParticipantID`），现在只有设备，将来可以有智能体，见"预留"一节。

**路由规则**

| 设备状态 | 麦克风音频去向 | 说明 |
| --- | --- | --- |
| 不在任何频道 | AI | 私密陪伴，免按键，AI 的回答只放给自己听 |
| 在频道里 | 频道 | 默认说给好友听 |
| 在频道里，且刚发出 `wake` | AI（一个回合） | 孩子叫小伙伴名字时才是和 AI 说话；AI 说完（`tts stop`）后 1.5 秒内没有新语音，回到频道 |

`wake` 由设备本地唤醒词引擎产生（Web 模拟器用点击玩偶或语音唤醒），服务端不靠识别文本来判断"是否在叫小伙伴"，这样频道里的闲聊不会被送给 AI。

**发言权（floor control）**
- 设备用 `mic.state start/stop` 圈出一句话（设备端 VAD 决定）。服务端另有看门狗：路由到频道的音频 600ms 没有新帧，也视为一句话结束。
- 频道空闲时，第一个 `start` 的人拿到发言权，成为 `Burst` 的发言人；说完后保留 600ms 拖尾，避免句中停顿被抢。
- 有人占着发言权时，其他人的音频帧丢弃，并向该设备发 `sound{busy}` 与 `haptic{tap}`，孩子知道"别人在说话"。
- 发言人断线或离开频道，立刻结束该 `Burst`。

**下行转发**
- `Burst` 开始：向每个听者发 `speaker.state{start, source:peer, from}`；结束：发 `stop`。
- 听者集合 = 发言人所在频道的成员 ∩ `relation.AreFriends(发言人, 听者)`，**每个 `Burst` 开始时计算一次**；期间收到好友被解除的事件，立即把对方移出。
- 每个听者一条有界队列（8 帧 ≈ 480ms），满了丢最旧帧；一个慢听者不能拖慢其他人。
- 听者正在播 AI 语音时，好友的发言**优先**：先让 `bridge/linx` 停止向该设备转发 AI 音频并中止 AI 播报，再开始好友音频。同一设备同一时刻只有一种来源的音频在发送，所以设备靠 `speaker.state` 的 `source` 就能区分，不必给音频帧加头。

**延迟目标** 发言人到听者的服务端内部延迟 < 100ms（不含网络），端到端 < 300ms。每个 `Burst` 记录首帧收到与首帧发出的时间戳，用于验证。

**接口**
```go
type Router interface {
    OnMicState(d DeviceID, on bool)
    OnWake(d DeviceID)
    OnMicFrame(d DeviceID, frame []byte)       // 热路径
    OnAIState(d DeviceID, s AIState)           // bridge 回报：idle / speaking
    RouteOf(d DeviceID) Route
}

// voice 依赖的外部能力，由对应模块实现
type Membership interface { ForegroundOf(p ParticipantID) (ChannelID, bool); Members(ch ChannelID) []ParticipantID }
type Friends    interface { AreFriends(ctx context.Context, a, b ParticipantID) (bool, error) }
type AI         interface { Forward(d DeviceID, frame []byte); Interrupt(d DeviceID) }
type Sink       interface { Send(p ParticipantID, m protocol.Message) error; SendAudio(p ParticipantID, f []byte) error }
// Sink 的实现：设备 = gateway 会话；将来的智能体 = 它的灵矽会话。voice 不关心对方是谁。
```

**数据结构**
```go
type Route int // RouteAI | RouteChannel | RouteNone

type deviceVoice struct {
    route       Route
    aiTurnUntil time.Time   // wake 之后 AI 回合的截止时间
    micOn       bool
    down        chan []byte // 有界下行队列
    source      Source      // 当前正在下发的来源：none / ai / peer
}

type Burst struct {
    Channel     ChannelID
    Speaker     ParticipantID
    Listeners   []ParticipantID
    StartedAt   time.Time
    LastFrameAt time.Time
    FirstOutAt  time.Time   // 首帧发出时间，用于延迟统计
}
```

**产生的事件**（给 world 与 history）：`BurstStarted{channel, speaker, listeners}`、`BurstEnded{...}`。

**依赖** channel、relation、bridge/linx、gateway。

---

## 4. channel

**职责** 管理频道与群的成员；区域频道的自动加入与离开；群的创建、4 位码入群、过期；每台设备的"前台频道"。

**不负责** 音频转发与发言权（voice）、好友关系（relation）。`channel` 只回答"谁在哪个频道"，**能不能听见**由 voice 结合 relation 决定。

**接口**
```go
type Channels interface {
    JoinZone(ctx, d DeviceID, ch ChannelID) error            // 由 zone 触发
    LeaveZone(ctx, d DeviceID) error
    CreateGroup(ctx, owner DeviceID, ttl GroupTTL) (code string, err error)
    JoinGroup(ctx, d DeviceID, code string) error
    Switch(ctx, d DeviceID, target string) error             // 群名、"下一个"、"上一个"
    Leave(ctx, d DeviceID, ch ChannelID) error
    Current(ctx, d DeviceID) (ChannelInfo, error)
    ForegroundOf(p ParticipantID) (ChannelID, bool)          // voice 使用，热路径
    Members(ch ChannelID) []ParticipantID                    // voice 使用，热路径
    Subscribe() <-chan Event                                 // Joined / Left / Switched
}
```

**数据结构**
```go
type ChannelKind int // KindZone | KindGroup

type Channel struct {
    ID        ChannelID
    Kind      ChannelKind
    Name      string
    Code      string     // 仅 Group：4 位数字
    Owner     DeviceID   // 仅 Group
    ExpiresAt time.Time  // 仅 Group；零值表示长期
}

type GroupTTL int // TTLTrip | TTLToday | TTLForever

type ChannelInfo struct { Channel Channel; Members []MemberInfo; Foreground bool }
type MemberInfo  struct { Who ParticipantID; Kind Kind; Name string; Friend bool } // Friend 相对于查询者
```

**规则**
- 成员是**参与者**（`ParticipantID`）。设备加入频道时，加入的是 `dev:<mac>`；将来智能体加入时是 `agt:<mac>`，频道逻辑不变。
- 一台设备可以加入多个频道，**前台只有一个**；说话和收听都只针对前台频道，后台频道只收到 `presence`。
- 4 位码在有效期内唯一；过期后失效；同一设备 1 分钟内错码超过 5 次则暂时拒绝。
- 区域频道成员关系只在内存，群与码持久化到 store。
- 事件：加入、离开、切换，驱动 `presence`、`channel.state` 下行消息，以及 world、history。

**依赖** store、gateway（发 `presence`、`channel.state`）。

---

## 5. relation

**职责** 好友关系的状态机与查询；把"两台设备靠近"转成待确认的好友请求。

**不负责** 频道与音频；不决定家长是否确认（由 parentapi/device 调用 `Confirm`）。

**接口**
```go
type Relations interface {
    AreFriends(ctx, a, b ParticipantID) (bool, error)        // 热路径，内存缓存；智能体按 OwnerDevice 解析为所属设备
    Friends(ctx, d DeviceID) ([]Friend, error)
    OnProximity(ctx, d DeviceID, peers []Peer) error         // 来自设备的 proximity 上报
    Confirm(ctx, reqID string, by GuardianID) error
    Remove(ctx, a, b DeviceID) error
    Subscribe() <-chan Event                                 // Proposed / Confirmed / Removed
}
```

**数据结构**
```go
type Status int // StatusPending | StatusConfirmed | StatusRemoved

type Friendship struct {
    ID          string
    A, B        DeviceID   // 存储时 A < B，保证一对只有一条
    Status      Status
    ConfirmedBy GuardianID
    CreatedAt   time.Time
}

type Peer struct { Device DeviceID; Strength float64 } // 0..1，越大越近
```

**规则**
- `proximity` 中某个 `Strength ≥ 0.8` 持续 1.5 秒才生成 `Pending`，避免擦肩而过误配；已是好友或已有未处理请求则忽略。
- 双方都要在场并上报靠近，请求才成立。
- `Confirm` 之后 `AreFriends` 才为真；`Remove` 立即生效并发事件，voice 据此中断正在进行的转发。
- 演示中确认者可以是孩子本人，但状态机保留 `Pending` 一步。

**依赖** store。

---

## 6. zone

**职责** 把设备上报的坐标映射到区域频道，并处理进出的抖动。

**不负责** 频道成员管理（交给 channel）。

**接口**
```go
type Zones interface {
    OnLocation(ctx, d DeviceID, lat, lng float64) error // 来自 location 上报
    ZoneAt(lat, lng float64) (ZoneID, bool)
}
```

**数据结构**
```go
type Zone struct { ID ZoneID; Name string; Polygon []LatLng } // 演示只有一个：公园
type ZoneID string                                            // 对应频道 "zone:<id>"
```

**规则** 进入区块 → 触发"要加入公园频道吗"的询问，孩子确认后 `channel.JoinZone`；离开 → `LeaveZone`。进出各设 20 米滞回，避免在边界反复切换。询问由 AI 完成，确认通过业务工具 `self.zone.join` 回到服务端。

**依赖** channel、bridge/linx。

---

## 7. bridge/linx

**职责** 为每台设备维护一条到灵矽的会话：握手、转发麦克风音频、把灵矽的 `stt/tts` 翻译成设备的灯光与 `speaker.state`、把 TTS 音频送回设备。

**不负责** 决定音频要不要送给 AI（voice）；不处理业务工具（bridge/mcp）；不管理智能体（linxapi/device）。

**接口**
```go
// 供 voice 使用
func (b *Bridge) Forward(d DeviceID, frame []byte)   // 上行音频帧 → 灵矽
func (b *Bridge) Interrupt(d DeviceID)               // 中止 AI 播报并停止下发音频
// 供 device/share/zone 使用
func (b *Bridge) Say(ctx, d DeviceID, text string) error // 不经 LLM，直接合成
// 回报
func (b *Bridge) OnAIState(fn func(d DeviceID, s AIState))
```

**数据结构**
```go
type Config struct { URL, Token, AgentID string }       // 来自环境变量
type link struct { dev *gateway.Session; ws *websocket.Conn; ready, listening, speaking bool }
type AIState int // AIIdle | AISpeaking
```

**现状（M1 已有）** 握手头（`Protocol-Version`、UUID、`X-Agent-ID`）、`realtime`、`audio.downlink`、灯光映射。

**待做** `Forward/Interrupt/Say` 对外接口（现在由 `OnAudio` 直接转发，需改为由 voice 调用）；**只在 `device.Ensure` 成功后才拨号**，`Device-Id` 直接用设备自己的 MAC（现在的 `PseudoMAC` 要删除）；`hello.agent_params` 注入孩子与小伙伴的名字；重连。

**预留** 灵矽会话的"上行音频来源"要能替换：现在只有所属设备的麦克风，将来智能体入频道时可以是频道里的发言。会话与身份不变（同一个 MAC、同一条连接），只换输入。

**依赖** gateway。

---

## 8. bridge/mcp

**职责** 代设备做 MCP：解析灵矽与设备之间的 `mcp` JSON-RPC，把设备自带工具和业务工具合并成一份列表，并按工具名路由调用。

**不负责** 业务规则本身（调用 channel、relation 等）；不解析工具返回的内容。

**接口**
```go
type Tool struct {
    Name        string          // self.<模块>.<功能>
    Description string
    InputSchema json.RawMessage
    Call        func(ctx context.Context, d DeviceID, args json.RawMessage) (ToolResult, error)
}
func (p *Proxy) Register(t Tool)
func (p *Proxy) OnDeviceMCP(d DeviceID, payload json.RawMessage)  // 设备 → 灵矽
func (p *Proxy) OnLinxMCP(d DeviceID, payload json.RawMessage)    // 灵矽 → 设备
```

**数据结构**
```go
type ToolResult struct { Text string; IsError bool }
```

**规则**
- `initialize`、`tools/list`：网关合并业务工具后回复，设备分页要处理完再合并。
- `tools/call`：名字属于业务工具 → 网关执行；否则转给设备，结果原样回灵矽。
- 业务工具调用身份来自连接，参数里没有设备 ID。设备自带工具与业务工具重名时以设备为准并记告警。

**业务工具**

| 工具 | 参数 | 调用 | 返回给 AI |
| --- | --- | --- | --- |
| `self.group.create` | `ttl`：trip / today / forever | `Channels.CreateGroup` | 4 位码，并让设备播报 |
| `self.group.join` | `code` | `Channels.JoinGroup` | 成功或失败原因 |
| `self.group.switch` | `target`：群名 / next / prev | `Channels.Switch` | 当前群名 |
| `self.group.current` | — | `Channels.Current` | 当前群与在线好友 |
| `self.zone.join` | — | `Channels.JoinZone`（仅在设备处于某区域时） | 成功或不在区域 |
| `self.friends.list` | — | `Relations.Friends` | 好友名字 |
| `self.friends.nearby` | — | `Relations.Friends` ∩ `Channels.Members` | 附近的好友 |
| `self.share.publish` | `description` | `Share.Publish` | 已分享 |

**依赖** bridge/linx、channel、relation、share。

---

## 9. device

**职责** 设备档案与自动开通：设备上线时按外观选定风格智能体，在灵矽侧注册设备；皮套变了就切换智能体；首次上线时引导孩子给小伙伴起名。

**不负责** 灵矽 REST 细节（linxapi）；不决定好友或频道。

**接口**
```go
type Devices interface {
    Ensure(ctx, info DeviceInfo) (Profile, error)      // 幂等：无档案则开通，换皮套则切换智能体
    Profile(ctx, d DeviceID) (Profile, error)
    SetNames(ctx, d DeviceID, doll, kid string) error  // 同时写入灵矽记忆焦点
}

type Catalog interface {                                // 风格目录
    EnsureAll(ctx) error                                // 启动时确保每个风格的智能体存在
    ForSkin(skin string) Style                          // skin 未知返回默认风格
}
```

**数据结构**
```go
type DeviceInfo struct { MAC, Skin, Firmware string }  // 来自 hello

type Style struct {
    ID           string  // "dragon" ...
    Skin         string
    AgentName    string
    SystemPrompt string  // 儿童友好、带 {{doll_name}} {{kid_name}} 占位
    VoiceID      string
    AgentID      string  // 启动时由 EnsureAll 填充
}

type Profile struct {
    ID       DeviceID    // = MAC
    Skin     string
    StyleID  string
    AgentID  string
    DollName string
    KidName  string
    Guardian GuardianID
    Stage    Stage       // StageFresh → StageNamed
}
```

**开通规则** 见"设备如何组织"。`Ensure` 要幂等：灵矽预注册对已存在的设备返回冲突时，按"已开通"处理并核对绑定的智能体。

**开箱引导** `Fresh`：`Say` 问候并请孩子给小伙伴起名 → 孩子说出名字 → `SetNames` 写入档案与记忆焦点，并更新会话变量 → `Named`。认主（声纹）是可选的后续能力，见"设备如何组织"的取舍一。

**依赖** linxapi、bridge/linx、store。

---

## 10. linxapi

**职责** 灵矽 REST 的薄客户端，只做协议转换，不含业务。

**接口**
```go
type Client interface {
    CreateAgent(ctx, a AgentSpec) (agentID string, err error)
    FindAgentByName(ctx, name string) (agentID string, found bool, err error)
    PreregisterDevice(ctx, agentID, mac string) error
    SwitchAgent(ctx, mac, agentID string, migrateHistory bool) error
    EnableMemory(ctx, agentID string) (memoryID string, err error)
    UpsertAttention(ctx, memoryID, name, description string) (attentionID string, err error)
    SetAttentionValue(ctx, memoryID, mac, attentionID, value string) error
    AddMemoryNode(ctx, memoryID, mac, content string) error
    CreateSpeaker(ctx, name string) (speakerID string, err error)
    EnrollVoiceprint(ctx, speakerID, audioURL string) error
    BindSpeakers(ctx, agentID string, speakerIDs []string) error
    SetOwnerOnly(ctx, agentID string, on bool) error
    RecentVoiceSamples(ctx, agentID string) ([]VoiceSample, error)
    Sessions(ctx, agentID string, page, limit int) ([]Session, error)
    SessionMessages(ctx, agentID, sessionID string) ([]Message, error)
}
```

**数据结构**
```go
type AgentSpec   struct { Name, AssistantName, SystemPrompt string; MemoryModel string }
type VoiceSample struct { AudioURL string; At time.Time }
type Message     struct { Role, Text, AudioURL string; At time.Time }
```

**规则** 响应里 HTTP 恒为 200，以 `code` 判断；`401` 视为令牌失效；所有调用设超时并重试一次。令牌只来自环境变量。

**依赖** 无。

---

## 11. history

**职责** 只追加的事件日志，生成"今日回顾"：遇到哪些小朋友、分享了什么画面、聊了什么。

**不负责** 生成摘要（先展示原话）；不决定谁能看（parentapi 控制）。

**接口**
```go
type History interface {
    Record(ctx, e Event) error
    Today(ctx, d DeviceID) (Digest, error)
}
```

**数据结构**
```go
type EventKind int // EventMetFriend | EventSharedScene | EventChat

type Event struct {
    ID      string
    Device  DeviceID
    Kind    EventKind
    Payload json.RawMessage // MetFriend{Friend, Channel} / SharedScene{Text, Channel} / Chat{Lines}
    At      time.Time
}

type Digest struct { Met []MetFriend; Shared []SharedScene; Chats []ChatLine }
```

**数据来源** 遇到好友：`relation` 与 `channel` 的事件；分享：`share`；聊天：定期从 `linxapi.Sessions` 拉取并按创建时间筛选当日（灵矽没有日期过滤）。

**依赖** store、linxapi。

---

## 12. share

**职责** 画面分享：孩子让小伙伴描述眼前的画面，描述播给频道里的好友，也播给孩子自己，并记入历史。

**不负责** 拍照与看图（设备端工具和灵矽的视觉模型）；不转发语音（用 `Say`）。

**接口**
```go
func (s *Share) Publish(ctx, from DeviceID, description string) error
```

**流程** AI 通过设备端 `self.camera.take_photo` 得到画面描述 → 调用业务工具 `self.share.publish` → `Publish` 取发送者前台频道里的好友 → 对每台设备（含发送者）`Say("<小伙伴名>看到：…")` → `history.Record`。

**依赖** channel、relation、bridge/linx、history。

---

## 13. parentapi

**职责** 家长端 REST；小程序与 Web 手机窗口调用同一套。

**不负责** 业务规则；只做鉴权、编排与序列化。

**接口**

| 方法与路径 | 说明 |
| --- | --- |
| `GET /api/kids` | 家长名下的孩子与玩偶 |
| `GET /api/kids/{id}/friends` | 好友列表，含待确认 |
| `POST /api/friend-requests/{id}/confirm` | 确认好友 |
| `DELETE /api/kids/{id}/friends/{fid}` | 解除好友 |
| `GET /api/kids/{id}/groups` | 所在群聊 |
| `GET /api/kids/{id}/digest?date=today` | 今日回顾 |

**鉴权** 演示用固定令牌；真实小程序用微信登录。所有接口都校验"该孩子属于此家长"。

**依赖** relation、channel、history、device。

---

## 14. world

**职责** 演示用观察通道：把设备位置、谁在说话、频道成员实时推给前端地图。不属于设备协议，走单独的 WebSocket。

**接口** `GET /world` 升级为 WebSocket，推送：
```go
type WorldEvent struct {
    Kind     string // "device" | "burst" | "member"
    Device   DeviceID
    Pos      *LatLng
    Channel  ChannelID
    Speaking bool
    Friend   *DeviceID // 与视角设备的好友关系，前端据此标注"已屏蔽"
}
```

**依赖** gateway、channel、voice（订阅 `BurstStarted/Ended`）。

---

## 15. store

**职责** 持久化接口与 SQLite 实现；迁移；仓储接口隔离，业务模块用假实现可单测。

| 表 | 关键字段 |
| --- | --- |
| `device` | `device_id`（MAC） `skin` `style_id` `agent_id` `doll_name` `kid_name` `stage` `guardian_id` `created_at` |
| `style` | `style_id` `skin` `agent_id`（风格智能体在灵矽的 ID） |
| `guardian` | `guardian_id` `display_name` |
| `friendship` | `id` `a` `b` `status` `confirmed_by` `created_at` |
| `channel_group` | `channel_id` `owner` `code` `expires_at` |
| `history_event` | `id` `device_id` `kind` `payload` `at` |

声纹原始音频不入库；记忆本体在灵矽，不重复保存。

---

## 前端模块（`web/src/`）

| 模块 | 类别 | 职责 | 状态 |
| --- | --- | --- | --- |
| `shared` | — | 生成的协议类型、WS 客户端、store | 🚧 |
| `device-sim` | 设备 | 虚拟设备：麦克风与扬声器已有；待补设备端 VAD 与 `mic.state`、`wake`、`proximity`、`location`、多来源下行音频（AI 与好友分别按采样率解码）、设备端 MCP 服务 | 🚧 |
| `plush` | 设备表现 | 玩偶与灯带，响应 `light`、`speaker.state`、`haptic` | 🚧 |
| `scenes/unbox` | Web 展示 | 开箱 | ⬜ |
| `scenes/desk` | Web 展示 | 桌面场景、命名与认主引导 | ⬜ |
| `scenes/park` | Web 展示 | 公园，多小朋友走动，靠近，提供场景截图 | ⬜ |
| `map` | Web 展示 | 地图，显示位置、谁在说话、"已屏蔽"（订阅 world） | ⬜ |
| `parent-sim` | Web 展示 | 手机窗口，调用 parentapi | ⬜ |
| `ui` | Web 展示 | 儿童友好文案与提示音 | ⬜ |

设备行为要能在固件复现；Web 展示行为只存在于演示，不进协议。

## 附：协议增量

| 方向 | 消息 | 字段 | 用途 |
| --- | --- | --- | --- |
| 下行 | `channel.state` | `name` `color` `event`（joined/switched/left） | 切群、入群的灯色与提示 |
| 下行 | `friend.state` | `event`（pending/confirmed/removed） `device_id` `name` | 贴近成为好友的震动与提示 |
| 双向 | `mcp` | `payload`（JSON-RPC） | 设备端 MCP，由 bridge/mcp 处理 |
| 下行 | `sound` | `name` 增加 `busy` | 占线提示 |
| 下行 | `speaker.state` | 预留可选 `from_kind`（device / agent） | 将来智能体入频道时，区分"小伙伴在说话"；现在不加 |
