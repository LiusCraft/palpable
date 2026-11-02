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
| 任何上行（设备未绑定时） | 丢弃：`serving(d)` 为假时不进 AI、频道、好友、位置（见 [trust.md §3](../trust.md#3-绑定家长先开通孩子再认主)） |
| `mic.state` / `wake` / 音频帧 | `voice.OnMicState / OnWake / OnMicFrame` |
| `proximity` | `relation.OnProximity` |
| `location` | `world.Publish(pos)` + `zone.OnLocation` |
| `mcp` | `mcp.OnDeviceMCP` |
| 断开 | 撤销绑定码、`voice.OnDisconnect`、`mcp.Forget`、`bridge.Close`、`channel.LeaveZone`、`zone.Forget` |

## 关键流程

### connect（连接建立，后台执行，不阻塞读循环）

```text
p = devices.Ensure(info)
    失败 → send error{internal}, light{muted}; return      // 设备仍可进频道对讲
p = applyDemo(p)                                           // 演示：绑定预置家长与声纹同意（demo.manual_bind 时跳过）
if p.Guardian == "":                                       // 未绑定：只播报绑定码
    loop: binder.Issue → send bind.code + 呼吸灯；到期前 30s 换码；直到绑定或掉线
    return
serve(p):                                                   // 绑定后（或家长刚绑定时由 device.Bound 事件触发）
active[d] = true
if p.Stage == named:  p = devices.Handoff(p)               // 上次交棒未完成，重试，不让孩子重新起名
if ai == nil: return
ai.Open(session, persona(p))
if p.Stage == fresh:  ai.Say(欢迎并请孩子起名)
```

`persona(p)` 把档案映射为 `linx.Persona`；`VoiceStep` 仅在「引导期且监护人同意」时非空。

### 绑定与解绑（实现 `parentapi.Bindings`）

```text
Claim(g, code, voiceConsent)  = binder.Claim → device.Bound 事件 → send bind.state{bound} → serve(p)
Unbind(g, d):
    p.Guardian != g → ErrNotYourDevice（parentapi 回 404）
    active[d] = false
    relation.RemoveAll(d)         // 每条发 Removed，voice 立即中断与好友的转发
    channel.Leave(全部频道)
    device.Unbind(d, g)           // 删档案，级联好友、群成员、历史
    在线 → send bind.state{unbound}，断开；设备重连后是新设备，重新播报绑定码
```

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
| `relation.Asked` | 双方 `friend.state{asking}` + `haptic double_tap` + `sound friend`；已就绪的设备由小伙伴问"要和 X 交朋友吗"（`sayFriendAsk`） |
| `relation.Dropped` | 双方 `friend.state{removed}`（清掉询问提示） |
| `relation.Proposed` | 双方 `friend.state{pending}`；小伙伴说"去告诉爸爸妈妈"（`sayFriendPending`） |
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
- 路由：`/ws`、`/healthz`；`/api/`（需 `wechat.app_id` + `wechat.secret`，或 `demo.parent_token` + `demo.guardian`，见 [parentapi](parentapi.md#约束)）；`/world` 与 `POST /world/tool`（需 `demo.world`）。
- 厂家后台对接：配置了 `ops.token` 才挂 `/internal/`（[opsapi](../factory.md#5-两个进程怎么协作)，`App` 实现 `opsapi.Ops`：`Enroll`/`Revoke`/`Wipe`/`SetLock`）；配置了 `ops.admin_url` 才启动 `adminhook` 推送。恢复出厂的流程 `reset` 由家长解绑与后台擦除共用，只有家长解绑会向后台推 `unbound`（后台自己下的擦除不回声）。
- 家长账号：配置了 `wechat` 时创建 [account](account.md)，每小时清过期会话；实现 `parentapi.Accounts.DeleteAccount`（逐台 `Unbind` 名下设备后删账号）。
- 未配置灵矽：`ai == nil`，`linxapi` 用内存 `Fake`；设备档案本地开通，频道对讲照常。
- 向离线设备下发消息直接丢弃；设备上线后从档案与频道状态重新得到所需信息。
