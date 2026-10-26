# linxapi

基础层 · `server/internal/linxapi` · 无依赖

## 边界

**职责**：灵矽 REST 的薄客户端，只做协议转换，不含业务。WebSocket 对话不在这里（见 [bridge/linx](bridge-linx.md)）。

**不负责**：决定调用什么、何时调用（device、history 决定）。

## 接口

```go
type Client interface {
    // 智能体
    CreateAgent(ctx, AgentSpec) (agentID, error)
    FindAgentByName(ctx, name) (agentID string, found bool, err error)
    UpdateAgent(ctx, agentID, AgentSpec) error             // 空字段保留原值
    // 设备
    PreregisterDevice(ctx, agentID, mac) error             // 已存在 → ErrAlreadyRegistered
    ActivateDevice(ctx, mac) error                         // 以设备身份访问 OTA 完成绑定；返回激活码则 ErrActivationRequired
    SwitchAgent(ctx, mac, agentID, migrateHistory bool) error
    // 记忆
    EnableMemory(ctx, agentID) (memoryID, error)           // 614（已存在）时取现有容器
    UpsertAttention(ctx, memoryID, name, description) (attentionID, error)   // 按名字查找，没有才创建
    SetAttentionValue(ctx, memoryID, mac, attentionID, value) error
    AddMemoryNode(ctx, memoryID, mac, content) error
    // 声纹
    CreateSpeaker(ctx, name) (speakerID, error)
    EnrollVoiceprint(ctx, speakerID, audioURL) error
    BindSpeakers(ctx, agentID, speakerIDs []string) error  // 可选的声纹认主，黑客松版不调用
    SetOwnerOnly(ctx, agentID, on bool) error              // 同上
    RecentVoiceSamples(ctx, agentID) ([]VoiceSample, error)
    // 聊天记录
    Sessions(ctx, agentID, page, limit) ([]Session, error)
    SessionMessages(ctx, agentID, sessionID) ([]Message, error)
}
func New(base, token string) *HTTPClient    // .WithOTA(url) 覆盖 OTA 地址
func NewFake() *Fake                        // 内存实现，供上层与未配置灵矽时使用
```

## 数据结构

```go
type AgentSpec   struct { Name, AssistantName, SystemPrompt, MemoryModel, VoiceID string; ChatHistory int }
//  MemoryModel: MemoryLongTerm | MemoryNone；ChatHistory: 0 不改 | 1 只存文字 | 2 文字与音频
type VoiceSample struct { AudioURL, MAC string; At time.Time }    // MAC 为空 = 平台没给样本来源
type Session     struct { ID, MAC string; CreatedAt time.Time; ChatCount int }
type Message     struct { Role, Text, AudioURL, MAC string; At time.Time }
```

## 关键流程

```text
do(method, path, body):                 // 最多 2 次
    once():
        HTTP 5xx 或网络错误（ctx 未取消） → retry
        HTTP 200，解析 envelope{code, msg, data}
            code 0        → 解 data
            401 / 403     → ErrUnauthorized
            404           → ErrNotFound
            其他          → APIError{path, code, msg}      // 业务错误不重试
    retry 之间等 300ms
```

## 约束

- 灵矽 HTTP 恒为 200，以响应体 `code` 判断。
- 所有调用 10s 超时。
- 令牌只来自 YAML（`linx.token`），REST 地址 `linx.api_base`，OTA 地址 `linx.ota_url`。
- `RecentVoiceSamples` 返回的样本必须带可核对的 MAC：引导智能体被所有新设备共用，调用方（device）只使用标着本机 MAC 的样本，没有则不登记。
- 声纹登记、`BindSpeakers`、`SetOwnerOnly` 的产品边界见 [linx-integration](../linx-integration.md)。
