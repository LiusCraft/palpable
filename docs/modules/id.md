# id

基础层 · `server/internal/id` · 无依赖

## 边界

**职责**：跨模块共用的标识类型，避免模块之间为了一个类型互相 import；以及随机记录 ID。

## 接口与类型

```go
type DeviceID string        // 设备 MAC，小写冒号分隔，与 hello.device_id 一致
type ChannelID string       // "zone:<id>" | "group:<id>"
type GuardianID string
type ParticipantID string   // "dev:<mac>"；预留 "agt:<mac>"

type Kind int               // KindUnknown | KindDevice | KindAgent
func (p ParticipantID) Kind() Kind
func Device(d DeviceID) ParticipantID
func OwnerDevice(p ParticipantID) DeviceID   // 好友判断与家长控制都以它为准；无法识别返回 ""
func New() string                            // 16 位十六进制随机 ID（好友请求、群、历史事件）
```

## 约束

- `OwnerDevice` 把智能体参与者归到所属设备，所以好友规则与家长控制自动适用于将来的智能体。
- 参与者前缀是协议外的内部约定，不会出现在设备协议里。
