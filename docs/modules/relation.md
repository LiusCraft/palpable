# relation

领域层 · `server/internal/relation` · 依赖：`store`、`id`

## 边界

**职责**：好友关系的状态机与查询；把「两台设备靠近」变成「问两个孩子 → 两家监护人各自放行 → 好友」。`AreFriends` 是整个系统隐私规则的判定依据。同意规则见 [trust.md §4](../trust.md#4-交朋友)。

**不负责**：频道与音频；谁有权放行（parentapi 鉴权后才调 `Confirm`）；每家的交友方式存在哪（`AutoApprove` 端口由 `app` 注入，查 `device.Profile.FriendPolicy`）。

## 接口

```go
func New(ctx, repo store.FriendshipRepo) (*Relations, error)     // 启动时装载全部 confirmed
func (r *Relations) SetAutoApprove(fn AutoApprove)                // 须在开始上报前调用；默认每侧都要家长放行
type AutoApprove func(ctx, d DeviceID) bool                       // d 的家长是否设为"孩子自己交友"

func (r *Relations) AreFriends(ctx, a, b ParticipantID) (bool, error)        // 热路径，读无锁快照
func (r *Relations) FilterFriends(ctx, viewer, ps []ParticipantID) []ParticipantID
func (r *Relations) Friends(ctx, d DeviceID) ([]Friendship, error)           // pending + confirmed
func (r *Relations) Request(ctx, reqID) (Friendship, error)                  // parentapi 放行前校验归属
func (r *Relations) OnProximity(ctx, d, peers []Peer) error                  // 靠近满足 → ask
func (r *Relations) Assent(ctx, d) ([]DeviceID, error)                       // 孩子说"好"（self.friends.accept）
func (r *Relations) Decline(ctx, d) []DeviceID                               // 孩子说"不要"（self.friends.decline）
func (r *Relations) Confirm(ctx, reqID, side DeviceID, by GuardianID) error  // side 的家长放行这一侧；ErrNotFound / ErrNotPending
func (r *Relations) Remove(ctx, a, b DeviceID) error                         // 解除；对 pending 等同家长拒绝；幂等
func (r *Relations) RemoveAll(ctx, d DeviceID) error                         // 解绑时用
func (r *Relations) Sweep(ctx) error                                         // 询问超时作罢、pending 7 天过期；app 每 10 秒调
func (r *Relations) Subscribe() <-chan Event                                 // Asked | Dropped | Proposed | Confirmed | Removed
```

## 数据结构

```go
type Friendship struct {
    ID                   string
    A, B                 DeviceID     // 存储时 A < B，一对只有一条记录
    Status               Status       // Pending | Confirmed | Removed | Declined（Asking 只出现在事件里，不入库）
    ConfirmedBy          GuardianID   // 最后放行的家长；两侧都自动放行时为 "auto"
    ApprovedA, ApprovedB GuardianID   // 每侧由谁放行；"" = 还在等这一侧的家长
    CreatedAt            time.Time
}
func (f Friendship) WaitingFor() []DeviceID     // pending 时还在等哪一侧
type Peer struct { Device DeviceID; Strength float64 }   // 0..1，越大越近

friends atomic.Pointer[map[pair]struct{}]     // 已确认好友快照，写时整份替换
close   map[[2]DeviceID]near{since, last}     // 有向：from 对 to 的持续靠近计时
asks    map[pair]*asking{a, b bool, expires}  // 正在问两个孩子，只在内存
```

不变量：`friends` 快照 ⇔ 存储里 `confirmed` 的集合；`AreFriends(a, a)` 恒为假；参与者先经 `id.OwnerDevice` 解析成设备。

## 状态机

```text
            靠近满足                两个孩子都 Assent             两侧都放行
 (none) ──────────▶ asking ──────────────────────────▶ pending ──────────────▶ confirmed
   ▲                 │ 有人 Decline / 60s 超时            │  每侧：AutoApprove → "auto"  │
   │                 ▼                                    │        否则等 Confirm(side)  │
   │              Dropped（不通知家长）                      │                              │
   │                                         Remove（家长拒绝）                        Remove
   │                                                      ▼                              ▼
   │                                     declined（24h 内不再询问）                   removed
   └──────────── 再次贴近：复用原记录，放行重新来 ◀─────────── 7 天未放行 → removed ◀───┘
```

| 事件 | 时机 |
| --- | --- |
| `Asked` | 进入 asking：app 让两边的小伙伴问孩子 |
| `Dropped` | asking 作罢 |
| `Proposed` | 进入 pending：至少一侧要等家长 |
| `Confirmed` | 进入 confirmed：两侧都放行，或两家都设为自动 |
| `Removed` | 解除、家长拒绝、pending 过期 |

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
    for p in ready: ask(d, p)

ask(x, y):
    已在问且未超时 → 忽略
    f = repo.ByPair(x, y)
    pending / confirmed → 忽略
    declined 且 24h 内  → 忽略
    其他 → asks[pair] = {expires: now+60s}，发 Asked

Assent(d):  对 d 所在的每个未超时 ask 记下这一侧；两侧都说好 → promote
promote(p): 每侧 AutoApprove → Approved="auto"；两侧都有 → confirmed（ConfirmedBy=auto），否则 pending；发对应事件
```

## 约束

- `AreFriends` 不访问存储、不加锁；`Confirm / Remove / Assent / promote` 在 `mu` 内先写存储再整份替换快照，最后发事件。
- 存储写成功才更新快照；存储失败返回错误，快照不变。
- 事件缓冲 256，满了丢并记错误日志。
- 阈值：`nearStrength 0.8`、`nearHold 1.5s`、`reportGap 3s`、`askWindow 60s`、`declineCooldown 24h`、`pendingTTL 7d`。
- 将来智能体入频道：`AreFriends` 已接受 `ParticipantID`，智能体按所属设备判断。
