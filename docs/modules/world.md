# world

接入层 · `server/internal/world` · 依赖：`id` · **仅演示模式注册**

## 边界

**职责**：演示用观察通道，把设备位置、在线、频道成员、谁在说话、好友关系实时推给前端地图。

**不负责**：任何业务判断；不属于设备协议。它只是 `app` 推来的事件的聚合与广播。

## 接口

```go
func New(zones []Zone, checkOrigin func(*http.Request) bool) *World
func (w *World) Publish(e Event)         // app 调用；更新状态并广播，慢观察者丢事件
func (w *World) ServeHTTP(rw, r)         // GET /world：升级 WebSocket，先推 snapshot，再推增量
```

另有 `POST /world/tool {device, tool, arguments}`（由 `app.demoTool` 提供，不在本包）：代替孩子说出指令，直接调用 [bridge/mcp](bridge-mcp.md) 的业务工具，规则仍由领域模块裁决；只接受在线设备与本机前端来源。

## 数据结构

```go
type Event struct {
    Kind     string        // device | pos | member | burst | friend | snapshot
    Device   id.DeviceID
    Name, Skin string
    Online   *bool;  Pos *LatLng
    Channel  id.ChannelID; Joined, Speaking *bool
    Friend   *id.DeviceID; Status string      // pending | confirmed | removed
    Devices  []DeviceState; Friends []Friendship; Zones []Zone   // 仅 snapshot
}
type DeviceState struct { Device; Name, Skin string; Online bool; Pos *LatLng; Channels []id.ChannelID; Speaking bool }
```

| Kind | 触发（`app`） | 对内部状态的作用 |
| --- | --- | --- |
| `device` | 上线 / 下线 / 起名 / 换皮套 | 下线时清空 `Speaking` 与 `Channels` |
| `pos` | 设备上报 `location` | 更新 `Pos` |
| `member` | `channel` 的 Joined / Left | 增删 `Channels` |
| `burst` | `voice` 的 BurstStarted / Ended | `Speaking` |
| `friend` | `relation` 事件 | 更新好友表；`removed` 删除 |

## 约束

- 状态只在内存；`Publish` 在锁内 `apply` 后向各订阅 channel 非阻塞发送。
- **会暴露所有设备位置**：只在 `demo.world: true` 时注册路由，默认返回 404。
- 前端据「好友表」标注非好友为「已屏蔽」，这是展示，不是规则。
