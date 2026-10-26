# 模块设计文档

> 每个模块一篇，写给开发者：**边界、接口、数据结构、关键流程**。分层与依赖规则见 [architecture.md §3](../architecture.md#3-分层与依赖)；
> 完成状态见 [tasks.md](../tasks.md)；灵矽的事实见 [linx-integration.md](../linx-integration.md)；测试见 [testing.md](../testing.md)。

## 模块一览

| 层 | 模块 | 路径 | 一句话 |
| --- | --- | --- | --- |
| 接入 | [gateway](gateway.md) | `internal/gateway` | 设备 WebSocket：握手、会话表、消息校验 |
| | [parentapi](parentapi.md) | `internal/parentapi` | 家长端 REST：鉴权、归属校验、编排 |
| | [world](world.md) | `internal/world` | 演示地图的只读观察通道（仅演示模式） |
| 编排 | [app](app.md) | `internal/app` | 组装模块、分发设备消息、领域事件 → 下行消息 |
| 能力 | [voice](voice.md) | `internal/voice` | **声音去向**：路由、发言权、好友转发 |
| | [bridge/linx](bridge-linx.md) | `internal/bridge/linx` | 每台设备一条灵矽会话 |
| | [bridge/mcp](bridge-mcp.md) | `internal/bridge/mcp` | 设备端 MCP 代理与业务工具 |
| 领域 | [device](device.md) | `internal/device` | 档案、开通、起名、交棒、声纹登记 |
| | [relation](relation.md) | `internal/relation` | 好友状态机与靠近判定 |
| | [channel](channel.md) | `internal/channel` | 区域频道与群、前台频道 |
| | [zone](zone.md) | `internal/zone` | 坐标 → 区域，边界滞回 |
| | [history](history.md) | `internal/history` | 事件日志与今日回顾 |
| | [share](share.md) | `internal/share` | 画面分享 |
| 基础 | [store](store.md) | `internal/store` | 仓储接口、SQLite 与内存实现 |
| | [linxapi](linxapi.md) | `internal/linxapi` | 灵矽 REST 薄客户端 |
| | [protocol](protocol.md) | `internal/protocol` | 设备协议 Go 类型与编解码 |
| | [id](id.md) | `internal/id` | 跨模块标识类型 |
| | [config](config.md) | `internal/config` | YAML 配置 |
| 前端 | [web](web.md) | `web/src` | 设备模拟器与演示展示 |

## 文档模板

每篇按需包含下列小节，没有内容的省略：

1. **边界**：职责、不负责。
2. **接口**：对外暴露的方法与它声明的端口。
3. **数据结构**：关键类型与不变量。
4. **关键流程**：状态机、算法；复杂的用伪代码，不贴实现。
5. **约束**：并发、容量、时限、错误语义。
6. **事件与依赖**。

## 全局约定

- 接口方法首参为 `context.Context`；热路径（音频帧、`AreFriends`、`Members`、`ForegroundOf`）除外。
- 热路径不加长锁、不打日志、不阻塞；错误带上下文返回，不吞。
- 领域模块用 `Subscribe() <-chan Event` 发事件：有界 channel，满了丢弃并记错误日志；只有 `app` 订阅。
- 锁内只改内存状态，不做 I/O；需要对外执行的动作先收集，解锁后执行（`voice` 的 `effects`、`channel` 的 `emit`）。
- 每个模块的存储都经 [store](store.md) 的仓储接口；测试用 `store.NewMemory()`。
- 外部输入（WS 消息、HTTP 请求）一律校验，非法输入返回协议错误，不 panic。
