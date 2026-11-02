# voice

能力层 · `server/internal/voice` · 依赖：`protocol`、`id`；端口 `Membership`、`Friends`、`AI`、`Sink`

**多人频道语音的核心，也是唯一决定声音去向的模块。** 隐私规则「非好友音频不转发」只在这里执行一次；例外：群是凭邀请码手动加入的，群成员不要求是好友。

## 边界

**职责**
1. **上行路由**：每帧麦克风音频去 AI、去频道，还是丢弃。
2. **同时说话**：频道里谁都可以随时开口，不占线。一人说话时透传，多人同时说话时混音。
3. **下行转发**：发给「同频道 ∩ 好友」（群频道为全部成员）；只有一个发言人时原样透传 Opus，多人同时说话时把各路解码、混成一路再编码，每个听者听到除自己外所有人的混音。一台设备同一时刻只收一种来源。

**不负责**：谁是好友（relation）、谁在频道（channel）、AI 怎么回答（bridge/linx）。仅在多人同时说话时解码与混音（`mixer.go`，纯 Go 的 `kazzmir/opus-go`，无 CGO）；统一按 16kHz 单声道 60ms 重新编码，解码不要求来源是 16kHz，灵矽 24kHz 的小伙伴语音同样能混。

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
func (r *Router) ArmAgent(owner, blocked map[DeviceID]bool) error  // self.channel.speak：下一段 AI 语音作为 agt:<owner> 播给频道；不在频道 → ErrNotInChannel
func (r *Router) OnAgentFrame(owner, frame)         // 热路径：主人的 AI 下行帧，小伙伴发言时转给听者
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
    hearing         int              // 正在给它送声音的发言数；0↔1 时发 speaker.state start/stop
    out             *outbox          // 有序下行队列，唯一写 goroutine
}

type Burst struct {                  // 频道里的一次发言
    Channel   ChannelID
    Speaker   ParticipantID
    Listeners []ParticipantID        // 开始时计算一次
    StartedAt, LastFrameAt, FirstInAt, FirstOutAt time.Time
    stopAt    time.Time              // mic.state stop 后的拖尾截止；零值 = 还在说
}

state: devices map[DeviceID]*deviceVoice;  bursts map[(ChannelID, ParticipantID)]*Burst;  mixers map[ChannelID]*chanMix   // 每个发言人一段；≥2 人同时说话才有 mixer
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
        b = bursts[(ch, d)] ?? takeFloor(d)
        if b == nil: 丢弃
        else:
            b.LastFrameAt = now
            if 说话中恢复(micOn 且 stopAt 非零): stopAt = 零
            mixers[ch] 为空: for l in b.Listeners: dev(l).out.pushFrame(frame)   // 透传，入队不做 I/O
            否则: 锁外 mixers[ch].push(speaker, frame)                            // 解码进该发言人的队列
            首次有听者时记 FirstOutAt
      None:    丢弃
    unlock; apply(effects)
```

### 开始发言

```text
takeFloor(d):                                   // 在锁内；名字沿用，实际不再有"占线"
    ch = 前台频道
    if b = bursts[(ch, d)]: stopAt = 零; return b         // 同一人继续说
    若 d 正在听 AI: 排队 Interrupt(d)                      // 孩子开口优先于播报
    b = new Burst(speaker=d)
    for m in members(ch), m != d, m 的小伙伴没在对频道说话:
        if 群 或 friends.AreFriends(d, m):                  // ← 隐私规则唯一执行点
            b.Listeners += m
    for l in b.Listeners:
        l.hearing++
        if l.hearing == 1:                                  // 之前没人在对它说话
            若 l 正在听 AI: 排队 Interrupt(l)               // 好友发言优先
            l.source = Peer;  排队 speaker.state{start, source, from:d, name}
    bursts[(ch, d)] = b;  事件 BurstStarted
    频道发言人数 ≥ 2 且没有混音 → 建 chanMix
```

一个频道里每个发言人各有一段 Burst，可以同时进行。设备上只看到"有人在说话"：`speaker.state start` 在 `hearing` 0→1 时发，`stop` 在回到 0 时发。

### 混音

```text
每 60ms mixTick():                       // 锁内取听者名单快照，锁外做编解码
    for 每个有 chanMix 的频道:
        每个发言人取一帧（队列不足 960 采样则这一拍不出声）
        每个听者 = 它能听到的发言人之和（clip 到 int16）→ 该听者自己的编码器 → pushFrame
```

- 发言人队列最多 3 帧，超出丢最旧的，防止设备与服务端时钟漂移让延迟越积越大。
- 频道发言人数回到 1 就拆掉 chanMix，剩下的人回到透传。
- 来自设备的包不可信：解码、编码的 panic 一律兜住，坏包丢弃。

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
OnAIState(Speaking): 若 source == Peer 且小伙伴没在对频道说话 → Interrupt(d)（好友优先，AI 让路）；否则 source = AI
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

- 一把 `sync.Mutex` 保护全部状态；临界区内只改内存、入队，不做 I/O，也不做 Opus 编解码（在 `chanMix` 自己的锁里）。需要打断 AI、调 `Listen`、发事件的动作收集到 `effects`，**解锁后**执行，且先 `Interrupt` 再开始好友音频。
- 听者在 Burst 开始时一次性确定：中途加入频道的人从下一个 Burst 起收听；好友被解除时 `OnFriendRemoved` 立即移出。
- 热路径无日志；每个 Burst 结束打一条 debug 元数据日志。
- 延迟目标：服务端内部 p95 < 100ms（不含网络）。
- 小伙伴发言（见 [架构 §6](../architecture.md#6-小伙伴智能体作为频道成员)）：`ArmAgent` 后 20 秒内 AI 开口即以 `agt:<mac>` 抢发言权，AI 说完或主人开口即结束；听者另按 `blocked` 排除，屏蔽名单由调用方在锁外算好。断流看门狗对小伙伴放宽到 3 秒（TTS 句间停顿较长）。
- 同时说话时，小伙伴发言不会被好友开口打断，只有它自己的主人开口才结束它。小伙伴发言期间主人直接听自己的小伙伴，不是频道里其他人的听者（主人只有一路下行）。
- 测试不要加 `-race`：转译库的指针运算会触发 checkptr 报错，不是我们的代码问题。
