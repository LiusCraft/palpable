# voice

能力层 · `server/internal/voice` · 依赖：`protocol`、`id`；端口 `Membership`、`Friends`、`AI`、`Sink`

**多人频道语音的核心，也是唯一决定声音去向的模块。** 隐私规则「非好友音频不转发」只在这里执行一次。

## 边界

**职责**
1. **上行路由**：每帧麦克风音频去 AI、去频道，还是丢弃。
2. **发言权**：同一频道同一时刻只有一个人说话。
3. **下行转发**：把发言人的 Opus 帧原样转给「同频道 ∩ 好友」；保证一台设备同一时刻只收一种来源。

**不负责**：谁是好友（relation）、谁在频道（channel）、AI 怎么回答（bridge/linx）。不解码、不混音、不转码。

## 接口

```go
type Router struct{ /* 一把锁保护全部状态 */ }
func New(m Membership, f Friends, ai AI, sink Sink, names Names) *Router

func (r *Router) OnMicState(d, on bool)             // mic.state start/stop
func (r *Router) OnWake(d)                          // 唤醒：开始一个 AI 回合
func (r *Router) OnMicFrame(d, frame)               // 热路径
func (r *Router) OnAIState(d, AIIdle|AISpeaking)    // bridge/linx 回报
func (r *Router) OnFriendRemoved(a, b)              // 立即把对方移出正在进行的转发
func (r *Router) OnLeave(d)                         // 离开或切换前台频道
func (r *Router) OnDisconnect(d)                    // 结束发言并释放下行 goroutine
func (r *Router) RouteOf(d) Route                   // RouteNone | RouteAI | RouteChannel
func (r *Router) Run(ctx)                           // 看门狗，100ms 一拍
func (r *Router) Subscribe() <-chan Event           // BurstStarted | BurstEnded

// 端口，由 app 注入
type Membership interface { ForegroundOf(p) (ChannelID, bool); Members(ch) []ParticipantID }
type Friends    interface { AreFriends(ctx, a, b) (bool, error) }
type AI         interface { Forward(d, frame); Interrupt(d); Available(d) bool; Listen(d, on bool) }
type Sink       interface { Send(p, msg) error; SendAudio(p, frame) error }
```

## 数据结构

```go
type deviceVoice struct {            // 每台设备一份
    micOn, aiListening, aiSpeaking bool
    aiTurn          bool             // wake 之后的 AI 回合
    aiTurnStart     time.Time
    aiTurnUntil     time.Time        // AI 说完后的回合截止；零值 = 还在等 AI 回应
    source          Source           // SourceNone | SourceAI | SourcePeer：当前正在下发的来源
    busyNotice      bool             // 本句已提示过占线
    out             *outbox          // 有序下行队列，唯一写 goroutine
}

type Burst struct {                  // 频道里的一次发言
    Channel   ChannelID
    Speaker   ParticipantID
    Listeners []ParticipantID        // 开始时计算一次
    StartedAt, LastFrameAt, FirstInAt, FirstOutAt time.Time
    stopAt    time.Time              // mic.state stop 后的拖尾截止；零值 = 还在说
}

state: devices map[DeviceID]*deviceVoice;  bursts map[ChannelID]*Burst   // 每个频道最多一个
```

## 关键流程

### 路由决策

```text
route(d):
    if v.aiTurn:
        过期 = (aiTurnUntil 非零且已过) 或 (now - aiTurnStart > 30s)
        if !过期: return AI
        v.aiTurn = false
    if members.ForegroundOf(d) 存在: return Channel
    if ai.Available(d):              return AI
    return None
```

| 设备状态 | 音频去向 |
| --- | --- |
| 不在频道，AI 可用 | AI（私密陪伴，回答只给自己听） |
| 在频道 | 频道 |
| 在频道，刚 `wake` | AI 一个回合；AI 说完（`AIIdle`）后 1.5s 内没有新语音则回到频道；回合最长 30s |
| 不在频道，AI 不可用 | 丢弃 |

`wake` 由设备本地唤醒词产生。服务端不靠识别文本判断是否在叫小伙伴，频道里的闲聊不会送给 AI。

### 帧处理（热路径）

```text
OnMicFrame(d, frame):
    lock
    switch route(d):
      AI:      unlock; ai.Forward(d, frame); return
      Channel:
        b = bursts[ch] ?? takeFloor(d)
        if b == nil 或 b.Speaker != d: 丢弃（takeFloor 已安排占线提示）
        else:
            b.LastFrameAt = now
            if 说话中恢复(micOn 且 stopAt 非零): stopAt = 零
            for l in b.Listeners: dev(l).out.pushFrame(frame)       // 入队，不做 I/O
            首次有听者时记 FirstOutAt
      None:    丢弃
    unlock; apply(effects)
```

### 发言权

```text
takeFloor(d):                                   // 在锁内
    ch = 前台频道
    if b = bursts[ch]:
        if b.Speaker == d: stopAt = 零; return b       // 同一人继续说
        若 !busyNotice: 排队「sound busy + haptic tap」给 d; return b   // 占线
    若 d 正在听 AI: 排队 Interrupt(d)                    // 孩子开口优先于播报
    b = new Burst(speaker=d)
    for m in members(ch), m != d:
        if friends.AreFriends(d, m):                     // ← 隐私规则唯一执行点
            b.Listeners += m
    for l in b.Listeners:
        若 l 正在听 AI: 排队 Interrupt(l)                // 好友发言优先
        l.source = Peer
        排队 speaker.state{start, source:peer, from:d, name}
    bursts[ch] = b;  事件 BurstStarted
```

结束条件（任一）：`mic.state stop` 后 600ms 拖尾过去；600ms 没有新帧（看门狗）；发言人 `OnLeave` / `OnDisconnect`；`OnWake`（发言人转去叫 AI）。
结束时对每个听者排队 `speaker.state{stop}`，`source` 归零，事件 `BurstEnded`，记录 `internal_latency_ms = FirstOutAt - FirstInAt`。

### 下行队列（outbox）

```text
pushFrame(f): 若音频已满 8 帧 → 丢最旧的音频帧；追加 f
pushMsg(m):   追加 m（控制消息从不丢弃，与音频保持先后）
run():        唯一写 goroutine：按序取出，经 Sink 发送
```

8 帧 ≈ 480ms；一个慢听者只会丢自己的帧，不影响其他人。

### AI 与好友的优先级

```text
OnAIState(Speaking): 若 source == Peer → Interrupt(d)（好友优先，AI 让路）；否则 source = AI
OnAIState(Idle):     source = None（若是 AI）；若处于 aiTurn 且在等回应 → aiTurnUntil = now + 1.5s
```

### 收音开关

```text
syncListen(d): want = micOn && route(d) == AI
               want != aiListening 时 ai.Listen(d, want)
```

灵矽只在「路由到 AI 且麦克风开着」时收音。

## 约束

| 项 | 值 |
| --- | --- |
| 拖尾 / 断流超时 | 600ms / 600ms |
| AI 回合宽限 / 最长 | 1.5s / 30s |
| 听者队列 | 8 帧 |
| 看门狗间隔 | 100ms |
| 事件缓冲 | 256，满了丢并记错误日志 |

- 一把 `sync.Mutex` 保护全部状态；临界区内只改内存、入队，不做 I/O。需要打断 AI、发占线提示、调 `Listen`、发事件的动作收集到 `effects`，**解锁后**执行，且先 `Interrupt` 再开始好友音频。
- 听者在 Burst 开始时一次性确定：中途加入频道的人从下一个 Burst 起收听；好友被解除时 `OnFriendRemoved` 立即移出。
- 热路径无日志；每个 Burst 结束打一条 debug 元数据日志。
- 延迟目标：服务端内部 p95 < 100ms（不含网络）。
- 将来智能体入频道：`Sink` 与 `ParticipantID` 已为此预留，`voice` 不需要知道对方是设备还是智能体（见 [架构 §6](../architecture.md#6-预留小伙伴智能体作为频道成员)）。
