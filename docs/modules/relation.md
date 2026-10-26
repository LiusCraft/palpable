# relation

领域层 · `server/internal/relation` · 依赖：`store`、`id`

## 边界

**职责**：好友关系的状态机与查询；把「两台设备靠近」转成好友请求。`AreFriends` 是整个系统隐私规则的判定依据。

**不负责**：频道与音频；谁有权确认（parentapi 鉴权后才调 `Confirm`）。

## 接口

```go
func New(ctx, repo store.FriendshipRepo) (*Relations, error)     // 启动时装载全部 confirmed
func (r *Relations) SetAutoConfirm(on bool)                       // 须在开始上报前调用

func (r *Relations) AreFriends(ctx, a, b ParticipantID) (bool, error)        // 热路径，读无锁快照
func (r *Relations) FilterFriends(ctx, viewer, ps []ParticipantID) []ParticipantID
func (r *Relations) Friends(ctx, d DeviceID) ([]Friendship, error)           // 含 pending，不含 removed
func (r *Relations) Request(ctx, reqID) (Friendship, error)                  // parentapi 确认前校验归属
func (r *Relations) OnProximity(ctx, d, peers []Peer) error
func (r *Relations) Confirm(ctx, reqID, by GuardianID) error                 // ErrNotFound / ErrNotPending
func (r *Relations) Remove(ctx, a, b DeviceID) error                         // 解除或拒绝；幂等
func (r *Relations) Subscribe() <-chan Event                                 // Proposed | Confirmed | Removed
```

## 数据结构

```go
type Friendship struct {
    ID          string
    A, B        DeviceID        // 存储时 A < B，一对只有一条记录
    Status      Status          // StatusPending | StatusConfirmed | StatusRemoved
    ConfirmedBy GuardianID      // "auto" 表示靠近后自动生效（AutoGuardian）
    CreatedAt   time.Time
}
type Peer struct { Device DeviceID; Strength float64 }   // 0..1，越大越近

friends atomic.Pointer[map[pair]struct{}]     // 已确认好友快照，写时整份替换
close   map[[2]DeviceID]near{since, last}     // 有向：from 对 to 的持续靠近计时
```

不变量：`friends` 快照 ⇔ 存储里 `confirmed` 的集合；`AreFriends(a, a)` 恒为假；参与者先经 `id.OwnerDevice` 解析成设备。

## 状态机

```text
                 propose（靠近满足）
 (none) ───────────────────────────────▶ pending ──Confirm(by)──▶ confirmed
   ▲                                       │                         │
   │                      auto 开启 ───────┼─────直接进入───────────▶│
   │                                       │                         │
   └───────── removed ◀──────Remove────────┴─────────Remove──────────┘
                  │  再次贴近：复用原记录重新进入 pending / confirmed
```

事件：进入 pending → `Proposed`；进入 confirmed → `Confirmed`；Remove → `Removed`。`SetAutoConfirm(true)`（默认，`demo.friend_needs_approval` 关闭）时贴近直接 `confirmed`、`ConfirmedBy=auto`，家长靠通知得知并可随时解除。

## 靠近判定

每次 `OnProximity(d, peers)` 是 `d` 的**完整快照**：

```text
OnProximity(d, peers):
    seen = {}
    for p in peers where p != d 且 p.Strength >= 0.8:
        seen += p
        n = close[d→p]
        if n 不存在 或 now - n.last > 3s: n.since = now      // 上报间隔过长视为中断，重新计时
        n.last = now
        back = close[p→d]                                    // 对方也必须上报了自己
        if back 存在 且 now - back.last <= 3s
           且 now - n.since >= 1.5s 且 now - back.since >= 1.5s:
            ready += p
    删除 close[d→*] 中本次没出现（或不够近）的条目          // 计时中断
    for p in ready: propose(d, p)

propose(x, y):
    f = repo.ByPair(x, y)
    不存在    → 创建（pending 或 confirmed）
    removed   → 复用原记录重新进入
    其他      → 忽略（已是好友或已有未处理请求）
```

## 约束

- `AreFriends` 不访问存储、不加锁；`Confirm / Remove / propose` 在 `mu` 内先写存储再整份替换快照，最后发事件。
- 存储写成功才更新快照；存储失败返回错误，快照不变。
- 事件缓冲 256，满了丢并记错误日志。
- 阈值：`nearStrength 0.8`、`nearHold 1.5s`、`reportGap 3s`。
- 将来智能体入频道：`AreFriends` 已接受 `ParticipantID`，智能体按所属设备判断。
