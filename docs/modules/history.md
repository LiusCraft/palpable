# history

领域层 · `server/internal/history` · 依赖：`store`、`linxapi`、`id`

## 边界

**职责**：只追加的事件日志，生成「今日回顾」：遇到哪些小朋友、分享了什么、聊了什么。

**不负责**：生成摘要（先展示原话）；谁能看（parentapi 校验归属）。

## 接口

```go
func New(repo store.HistoryRepo, linx linxapi.Client, agentOf AgentOf) *History   // AgentOf: 设备 → 当前智能体

func (h *History) Record(ctx, Event) error
func (h *History) RecordMet(ctx, d, MetFriend) error
func (h *History) RecordShared(ctx, d, SharedScene) error
func (h *History) Today(ctx, d) (Digest, error)
```

## 数据结构

```go
type Event struct { ID string; Device DeviceID; Kind EventKind; Payload json.RawMessage; At time.Time }
// EventKind: EventMetFriend | EventSharedScene | EventChat（预留，聊天不入库）

type MetFriend   struct { Friend DeviceID; Name, Channel string; At time.Time }
type SharedScene struct { From DeviceID; Name, Text, Channel string; At time.Time }
type ChatLine    struct { Role string /*user|assistant*/; Text string; At time.Time }
type Digest      struct { Met []MetFriend; Shared []SharedScene; Chats []ChatLine }   // 空列表为 []，不为 null
```

存储：`history_event(id, device_id, kind, payload JSON, at)`，索引 `(device_id, at)`。

## 关键流程

```text
Today(d):
    from = 北京时间今天 0 点；to = from + 1 天
    evs = repo.ListByDevice(d, from, to)
    for e in evs:
        MetFriend    → 同一好友只保留第一次
        SharedScene  → 全部保留
    chats = chats(d, from)            // 失败只记日志，其余部分照常返回

chats(d, from):
    agent = agentOf(d);  空则无聊天
    for page in 1..10:
        sessions = linx.Sessions(agent, page)
        for s in sessions:
            s.CreatedAt < from        → 标记「已翻到更早」，跳过
            s.MAC 非空且 != d         → 跳过（别的孩子的会话，不拉消息）
            for m in linx.SessionMessages(agent, s.ID):
                只保留 m.MAC == d 且文本非空、role ∈ {user, assistant}、m.At >= from
        翻到更早的会话，或本页不满，停止
    按时间升序
```

## 约束

- **MAC 过滤是隐私规则**：风格智能体被多台设备共用，消息没有 MAC 一律不展示，避免把别的孩子的对话给家长看。
- 「今天」按北京时间（`Asia/Shanghai`，加载失败回退 UTC+8）。
- 谁写入：遇到好友由 `app` 在好友确认、设备加入频道时记录（加入频道时与在场每位好友互记）；分享由 [share](share.md) 记录。
- 聊天不入库，实时从灵矽拉取；灵矽音频约 60 天后清理。
