# channel

领域层 · `server/internal/channel` · 依赖：`store`、`id`

## 边界

**职责**：回答「谁在哪个频道」。区域频道与群的成员、4 位码入群与过期、每个参与者的**前台频道**。

**不负责**：音频与发言权（voice）；好友（relation）；下发设备消息（`app` 订阅事件后下发）。

## 接口

```go
func New(ctx, repo store.GroupRepo) (*Channels, error)       // 清理过期群，装载群与成员；区域成员启动时为空

func (c *Channels) DefineZone(ch ChannelID, name string)     // zone 启动时登记区域频道
func (c *Channels) JoinZone(ctx, d, ch) error                // 同时只在一个区域频道；加入即前台
func (c *Channels) LeaveZone(ctx, d) error
func (c *Channels) CreateGroup(ctx, owner, ttl GroupTTL) (code string, err error)
func (c *Channels) JoinGroup(ctx, d, code) error             // ErrBadCode / ErrTooManyAttempts
func (c *Channels) Switch(ctx, d, target string) error       // 群名 | 4 位码 | 频道 ID | "next" | "prev"
func (c *Channels) Leave(ctx, d, ch) error                   // 群成员身份同时从存储删除
func (c *Channels) Current(ctx, d) (ChannelInfo, error)      // 前台；ErrNotInChannel
func (c *Channels) List(ctx, d) []ChannelInfo                // 所在全部频道，前台在前
func (c *Channels) ForegroundOf(p) (ChannelID, bool)         // 热路径
func (c *Channels) Members(ch) []ParticipantID               // 热路径，按加入时间排序
func (c *Channels) Sweep(ctx) error                          // app 每分钟调用：清过期群与错码记录
func (c *Channels) Subscribe() <-chan Event                  // Joined | Left | Switched
```

## 数据结构

```go
type Channel struct {
    ID ChannelID          // "zone:<id>" | "group:<uuid>"
    Kind Kind             // KindZone | KindGroup
    Name string
    Code string           // 仅群：4 位数字，1000–9999
    Owner DeviceID        // 仅群
    ExpiresAt time.Time   // 仅群；零值 = 长期
}

chans   map[ChannelID]*chanState{ch, members map[ParticipantID]joinedAt}
parts   map[ParticipantID]*participant{channels []ChannelID /*按加入顺序*/, foreground ChannelID}
badCode map[DeviceID][]time.Time       // 错码时间，滑动窗口限流
```

不变量：`foreground` 要么为空，要么属于 `channels`；设备最多一个区域频道；一个参与者的 `channels` 为空时从 `parts` 删除。

## 关键流程

```text
joinLocked(p, cs):
    members[p] ??= now;  channels 追加（去重）
    if foreground == cs.id: return []             // 已是前台，不发事件
    foreground = cs.id;   return [Joined]

leaveLocked(p, ch):
    从 members 与 channels 删除;  事件 [Left]
    if foreground == ch:
        foreground = channels 的最后一个（最近加入）或空
        若有 → 追加 Switched

JoinZone(d, ch):   先离开设备已在的其他区域频道，再 joinLocked
JoinGroup(d, code):
    限流：近 1 分钟错码 ≥ 5 → ErrTooManyAttempts
    找 Kind=group 且 Code==code 且未过期的群；没有 → 记一次错码，ErrBadCode
    成功 → 清空错码；repo.AddMember；joinLocked
CreateGroup(owner, ttl):
    code = 随机 1000–9999，在「未过期的群」里唯一，最多试 50 次（ErrNoCodesAvailable）
    repo.Create + AddMember；joinLocked
Switch(d, target):
    next/prev: 在 channels 中按加入顺序环形移动
    其他: 在 d 已加入的频道里按 名称 / 码 / ID 匹配；没有 → ErrNoSuchChannel
    变化时发 Switched
```

过期：行程群 6 小时，今天群到次日 0 点，长期群无过期。`Sweep` 删库中过期群，让成员 `leaveLocked`，清理错码窗口外的记录。

## 约束

- `sync.RWMutex`：`ForegroundOf` / `Members` 用读锁；事件在解锁后 `emit`，不在锁内发送。
- 区域频道成员只在内存，设备断线（`app` 调 `LeaveZone`）即离开；群成员持久化，重启后仍在，前台在入群或切群后才恢复。
- 群名默认「群聊 + 码」。
- 事件缓冲 256，满了丢并记错误日志。
- 成员是 `ParticipantID`，将来智能体 `agt:<mac>` 无需改本模块。
