# bridge/mcp

能力层 · `server/internal/bridge/mcp` · 依赖：`id`；端口 `ToLinx`、`ToDevice`、`HasMCP`；业务工具依赖 `device`、`zone`、`channel`、`relation`、`share` 的窄接口

## 边界

**职责**：代设备做 MCP。解析灵矽与设备之间的 JSON-RPC，把设备工具与业务工具合并成一份列表，按工具名路由调用。

**不负责**：业务规则本身（交给领域模块）；不解析设备工具的返回内容。

调用身份来自连接，工具参数里没有设备 ID。

## 接口

```go
func New(toLinx ToLinx, toDevice ToDevice, hasMCP HasMCP) *Proxy
func (p *Proxy) Register(t Tool)
func (p *Proxy) OnDeviceMCP(d, payload)     // 设备 → 灵矽
func (p *Proxy) OnLinxMCP(d, payload)       // 灵矽 → 设备（读循环里调用，不阻塞）
func (p *Proxy) Call(ctx, d, name, args) (ToolResult, error)   // 直接执行业务工具（演示遥控）
func (p *Proxy) Forget(d)                   // 断线清理

func BusinessTools(s Services) []Tool       // Services：device / zone / channel / relation / share 的窄接口

type Tool struct {
    Name        string          // self.<模块>.<功能>
    Description string          // 儿童友好，不引导说出个人信息
    InputSchema json.RawMessage
    Call        func(ctx, d, args) (ToolResult, error)
}
type ToolResult struct { Text string; IsError bool }
```

## 数据结构

```go
pending  map[reqID]chan rpc            // 代理自己发给设备的请求，等回复
devTools map[DeviceID]map[name]bool    // 设备自带的工具名；重名时以设备为准
```

代理发给设备的请求用字符串 ID `palpable-N`，与灵矽的数字 ID 不冲突。

## 关键流程

```text
OnLinxMCP(d, msg):                                  // 每个分支放到 goroutine
    initialize  → 设备支持 MCP：转给设备（params 里带视觉接口地址与令牌），回复原样回灵矽
                  失败或设备不支持：代理自己回 {protocolVersion, tools capability, serverInfo}
    tools/list  → tools = []
                  if hasMCP(d): 循环取设备的全部分页（最多 20 页，每次等 5s），按名字去重
                  devTools[d] = 设备工具名
                  for t in 业务工具: 与设备重名则跳过并告警；否则追加
                  回灵矽 {tools, nextCursor:""}
    tools/call  → name 是业务工具 且 设备没有同名工具：
                      执行 t.Call（20s 超时）；出错回「这件事暂时没做成，等一下再试试吧」
                  否则 hasMCP(d)：原样转给设备（回复经 OnDeviceMCP 原样回灵矽）
                  否则 回错误 -32601
    其他方法    → hasMCP ? 转给设备 : 回 -32601

OnDeviceMCP(d, msg):
    若是响应且 ID 在 pending 中 → 交给等待者（代理自己的请求）
    否则原样转给灵矽
```

## 业务工具

| 工具 | 参数 | 调用 | 返回给 AI |
| --- | --- | --- | --- |
| `self.profile.set_names` | `doll_name` `kid_name` | `device.SetNames` | 已记住 |
| `self.voiceprint.enroll` | — | `device.EnrollVoice` | 成功，或原因（未同意 / 没取到声音）；只有引导智能体会被提示使用 |
| `self.zone.join` | — | `zone.JoinCurrent` | 成功，或不在任何区域 |
| `self.group.create` | `ttl`: trip / today / forever | `channel.CreateGroup` | 4 位码，并让设备播报 |
| `self.group.join` | `code` | `channel.JoinGroup` | 成功或失败原因 |
| `self.group.switch` | `target`: 群名 / next / prev | `channel.Switch` | 当前群名 |
| `self.group.current` | — | `channel.Current` | 当前群与在线好友 |
| `self.friends.list` | — | `relation.Friends` | 好友名字 |
| `self.friends.nearby` | — | `relation.FilterFriends(channel.Members)` | 附近的好友 |
| `self.share.publish` | `description` | `share.Publish` | 已分享；未配置灵矽时不注册 |

设备端自带的工具（`self.light.set`、`self.haptic.pulse`、`self.camera.take_photo`）在设备侧实现，见 [web](web.md#device-sim)。

## 约束

- 等设备回复 ≤ 5s，业务工具 ≤ 20s，设备分页 ≤ 20 页。
- 设备不声明 `mcp` 能力时，代理仍提供业务工具。
- 工具描述与错误文案遵守儿童友好与「不诱导留下个人信息」。
- 处理放在 goroutine，不阻塞灵矽会话的读循环。
