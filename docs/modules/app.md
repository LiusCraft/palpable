# app

编排层 · `server/internal/app` · 依赖：下层所有模块

## 边界

**职责**
1. 实现 `gateway.Handler`，把设备消息分发到对应模块。
2. 订阅领域事件，翻译成下行设备消息，并转给 `world` 与 `history`。
3. 启动时创建各模块、组装端口。`cmd/palpabled/main.go` 只读配置并调用 `app.Run`。

**不负责**：业务判断。`app` 不持有业务状态；出现「是好友 / 在频道」这类 `if`，说明规则放错了地方。

## 端口组装

| 端口 | 声明方 | `app` 注入的实现 |
| --- | --- | --- |
| `voice.Membership` | voice | `channel.Channels` |
| `voice.Friends` | voice | `relation.Relations` |
| `voice.AI` | voice | `bridge/linx.Bridge`；未配置灵矽时为 `noAI`（`Available=false`） |
| `voice.Sink` | voice | `App.Send / SendAudio`，经 `gateway.Session` |
| `linx.DeviceLink` | bridge/linx | `gateway.Session` |
| `mcp.ToLinx / ToDevice / HasMCP` | bridge/mcp | `Bridge.SendMCP` / 会话发 `mcp` 消息 / `capabilities` 含 `mcp` |
| `share.Announcer` | share | `Bridge.Say` |

回调：`Bridge.OnAIState → voice.OnAIState`（并记 `speaking` 供交棒等待）；`Bridge.OnMCP → mcp.OnLinxMCP`。

## 上行分发

| 设备事件 | 调用 |
| --- | --- |
| 连接建立 | 后台 `connect`（见下） |
| `mic.state` / `wake` / 音频帧 | `voice.OnMicState / OnWake / OnMicFrame` |
| `proximity` | `relation.OnProximity` |
| `location` | `world.Publish(pos)` + `zone.OnLocation` |
| `mcp` | `mcp.OnDeviceMCP` |
| 断开 | `voice.OnDisconnect`、`mcp.Forget`、`bridge.Close`、`channel.LeaveZone`、`zone.Forget` |

## 关键流程

### connect（连接建立，后台执行，不阻塞读循环）

```text
p = devices.Ensure(info)
    失败 → send error{internal}, light{muted}; return      // 设备仍可进频道对讲
p = applyDemo(p)                                           // 演示：绑定预置家长与声纹同意
if p.Stage == named:  p = devices.Handoff(p)               // 上次交棒未完成，重试，不让孩子重新起名
if ai == nil: return
ai.Open(session, persona(p))
if p.Stage == fresh:  ai.Say(欢迎并请孩子起名)
```

`persona(p)` 把档案映射为 `linx.Persona`；`VoiceStep` 仅在「引导期且监护人同意」时非空。

### handoff（订阅 `device.Named`，每台设备一个 goroutine）

```text
waitEnroll(d)       // 有同意且未登记：等 EnrollAttempts 通知，最长 45s（成败都算）
waitAISilent(d)     // 等引导智能体说完告别：先给 4s 开口，之后等 speaking=false，最长 20s
p = devices.Handoff(d)          失败 → 记日志，保持 named，下次上线重试
ai.Close(d); ai.Open(session, persona(p))               // 换 agent_id 重连
ai.Say(d, 「<孩子名>，我是<小伙伴名>」)
```

### 领域事件 → 下行

| 事件 | 动作 |
| --- | --- |
| `relation.Proposed` | 双方 `friend.state{pending}` + `haptic double_tap` + `sound friend` |
| `relation.Confirmed` | 双方 `friend.state{confirmed}` + 同上提示；双方各记 `RecordMet` |
| `relation.Removed` | 双方 `friend.state{removed}`；`voice.OnFriendRemoved` |
| `channel.Joined` | 本机 `channel.state{joined}` + `sound`；对**同频道好友**（经 `FilterFriends`）互发 `presence{join}`，互记 `RecordMet`；`world` |
| `channel.Left` | `voice.OnLeave`；本机 `channel.state{left}`；通知同频道好友 `presence{leave}` |
| `channel.Switched` | `voice.OnLeave`；本机 `channel.state{switched}` |
| `zone.Entered` | 设备已 `ready` 才 `Say` 询问是否加入；引导期忽略 |
| `voice` Burst 起止 | 仅转 `world` |

灯色：区域频道固定绿色，群按频道 ID 哈希取色（`channelColor`）。

## 其他职责

- 启动后台任务：`ensureCatalog`（指数退避，5s 起、上限 5min，直到灵矽侧智能体就绪）、`sweepGroups`（每分钟）、各事件循环、`voice.Run`。
- 路由：`/ws`、`/healthz`；`/api/`（需 `demo.parent_token` + `demo.guardian`）；`/world` 与 `POST /world/tool`（需 `demo.world`）。
- 未配置灵矽：`ai == nil`，`linxapi` 用内存 `Fake`；设备档案本地开通，频道对讲照常。
- 向离线设备下发消息直接丢弃；设备上线后从档案与频道状态重新得到所需信息。
