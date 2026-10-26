# store

基础层 · `server/internal/store` · 依赖：`id`

## 边界

**职责**：持久化接口与实现。每个领域模块一个仓储接口；SQLite 实现（`modernc.org/sqlite`，免 CGO）与内存假实现（`NewMemory()`）；迁移。

**不负责**：业务规则。只存取记录；枚举（阶段、好友状态、历史类型）以整数存放，含义由所属领域模块定义。

## 接口

```go
type Store interface {
    Devices() DeviceRepo;   Styles() StyleRepo;   Guardians() GuardianRepo
    Friendships() FriendshipRepo;   Groups() GroupRepo;   History() HistoryRepo
    Close() error
}
func Open(ctx, path string) (*SQLite, error)    // 打开并迁移
func NewMemory() *Memory                        // 假实现，与 SQLite 满足同一套契约
```

| 仓储 | 方法 | 语义要点 |
| --- | --- | --- |
| `DeviceRepo` | `Get / Create / Update / Delete / ListByGuardian` | `Create` 已存在 `ErrConflict`（并发开通判重）；`Update` 不存在 `ErrNotFound`、不改 `CreatedAt`；`Delete` 同一事务删设备、好友、群成员、历史 |
| `StyleRepo` | `Get / Put / List` | `Put` 覆盖 |
| `GuardianRepo` | `Get / Put` | `Put` 覆盖 |
| `FriendshipRepo` | `Get / ByPair / Create / Update / ListByDevice / ListByStatus` | `ByPair` 参数顺序无关；`Update` 只改 `Status`、`ConfirmedBy`；`ListByStatus` 供启动装载 |
| `GroupRepo` | `Create / Get / ByCode / List / Delete / DeleteExpired / AddMember / RemoveMember / Members / ListByMember` | 只存群；区域频道成员不入库；`AddMember` 已是成员不改 `JoinedAt` |
| `HistoryRepo` | `Append / ListByDevice(from, to)` | 区间 `[from, to)`，按时间升序 |

错误：`ErrNotFound`、`ErrConflict`。

## 数据结构

| 表 | 字段 | 约束与索引 |
| --- | --- | --- |
| `device` | `device_id` PK, `skin`, `style_id`, `agent_id`, `doll_name`, `kid_name`, `stage`, `guardian_id`, `voice_consent`, `speaker_id`, `created_at` | — |
| `style` | `style_id` PK, `skin`, `agent_id` | 引导智能体 `style_id = welcome` |
| `guardian` | `guardian_id` PK, `display_name` | — |
| `friendship` | `id` PK, `a`, `b`, `status`, `confirmed_by`, `created_at` | `CHECK (a < b)`；`UNIQUE (a, b)`；索引 `b` |
| `channel_group` | `channel_id` PK, `name`, `owner`, `code`, `expires_at` | 索引 `code` |
| `channel_member` | `(channel_id, participant_id)` PK, `joined_at` | 外键 `ON DELETE CASCADE`；索引 `participant_id` |
| `history_event` | `id` PK, `device_id`, `kind`, `payload`, `at` | 索引 `(device_id, at)` |

## 约束

- 时间统一存 Unix 毫秒（UTC），0 表示无；两种实现都经 `toMillis / fromMillis` 换算，读出值一致。
- 好友对由仓储规范为 `A < B`；设备与自己结为好友被拒绝。
- 迁移：`migrations/*.sql` 按文件名顺序执行，进度记在 `PRAGMA user_version`。
- **声纹与原始音频不入库**；记忆本体在灵矽，不重复保存。
- 业务模块只依赖自己那一个仓储接口。
