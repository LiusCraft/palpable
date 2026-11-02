# factory（厂家后台）

独立进程 `palpable-admin` · 领域 `server/internal/factory` · 存储 `factorydb` · 接入 `adminapi` · 装配 `adminapp` · 对外 `palpclient` · 页面 `admin/`

> 产品与流程见 [../factory.md](../factory.md)。本篇只写实现边界、接口和约束。

## 边界

**职责**：设备台账（序列号、MAC、公钥、批次、渠道、订单、保修、成色）、生命周期状态机、只追加的事件日志、运营人员与角色、批次导入、异常核对。

**不负责**：任何孩子或家长的信息（只收 `owner_ref` 的不可逆值）；设备协议与家长端接口（在 palpabled）；订单、物流、支付；固件 OTA。

与 `palpabled` **没有任何包级依赖**：不 import `device`、`store`，不共用数据库。唯一的联系是 `factory.Platform` 端口与 HTTP。

## 分层

```text
admin/ (React + shadcn)  ──HTTP──▶ adminapi ──▶ factory ──▶ Repo / StaffRepo ◀── factorydb (SQLite)
                                      ▲            │
          palpabled ─ platform-events ┘            └──▶ Platform ◀── palpclient ──HTTP──▶ palpabled /internal
```

依赖只能向下：`adminapi` 依赖 `factory`；`factorydb`、`palpclient` 实现 `factory` 声明的端口；`adminapp` 负责组装。

## 接口（`factory`）

```go
type Service struct{ /* Repo + Platform + Options */ }

func NewService(Repo, Platform, Options) *Service              // Platform 可为 nil：需要它的动作返回 ErrNoPlatform

func (s *Service) ImportBatch(ctx, Actor, BatchInput) ([]Unit, error)
func (s *Service) Apply(ctx, Actor, serial, Action, Params) (Unit, error)
func (s *Service) OnPlatformEvent(ctx, PlatformEvent) error    // activated | unbound，幂等
func (s *Service) List / Detail / Stats / Events / Batches / Channels
func (s *Service) ResolveAnomaly(ctx, Actor, eventID) error
func (s *Service) PutChannel(ctx, Actor, Channel) error

func Available(Unit, Role) []Action                            // 此刻此角色可做的动作，供界面画按钮

type Platform interface {                                      // 全部幂等
    Enroll(ctx, mac, pubkey) error
    Revoke(ctx, mac) error
    Wipe(ctx, mac) error
    SetLock(ctx, mac, locked bool) error
}

type Auth struct{}                                             // 登录、会话、账号
func (a *Auth) Login / Authenticate / Logout / Create / List / Update / ChangePassword / Bootstrap
```

状态、动作、角色权限的完整表见 [../factory.md §3.3、§6](../factory.md#33-动作表)，代码在 `lifecycle.go`（`rules`、`rolePerms`）。

## 数据（`factorydb`，独立库 `palpable-admin.db`）

| 表 | 内容 | 要点 |
| --- | --- | --- |
| `batch` | 批次号、型号、皮套、固件 | 批次号唯一 |
| `unit` | 序列号、MAC、公钥、状态、渠道、订单、保修、主人标记、锁定、`version` | `mac` 唯一；`version` 做乐观锁 |
| `unit_event` | 谁、何时、从哪到哪、备注、是否异常 | **触发器禁止删改**，只有"核对异常"两列可更新 |
| `channel` | 渠道名录 | |
| `staff`、`staff_session` | 账号（PBKDF2 哈希）、会话（只存令牌 SHA-256） | |

## 关键流程

```text
Apply(actor, serial, action, params):
    rule = rules[action];   !actor.Role.Can(action) → ErrForbidden
    校验输入（备注 ≤ 200 字；渠道必须存在；订单号 1–40 位字母数字 ._/-；成色 A/B；锁定原因）
    u = repo.Unit(serial);  状态不在 rule.from → ErrBadTransition;  已锁定且不是 unlock/scrap → ErrLocked
    副作用：rule.wipe → Platform.Wipe；rule.revoke → Platform.Revoke；lock/unlock → Platform.SetLock
            任一失败 → 整个动作失败（ErrPlatform），状态与事件都不变
    改 Unit（售出写 SoldAt 与保修；收退货清主人；翻新写成色与次数……）
    repo.SaveUnit(u, event)         // 版本号不符 → ErrConflict；设备与事件同一事务

OnPlatformEvent(activated, owner):
    Sold/Activated → Activated；换了人则 transfer_count++ 并记 transferred
    InStock/InTransit/OnShelf/Refurbished → Activated，事件标 anomaly（未登记售出）
    Returning/Repairing → 只记主人，不改状态
    Scrapped → 只记 anomaly，并再次吊销公钥
    产线与售后测试状态 → 只记事件，不算异常
OnPlatformEvent(unbound): 有主人才处理；Activated → Sold；PrevOwner = Owner
```

## 约束

- **擦除是硬门槛**：`qc_pass`、`receive_return`、`inspect_pass`、`refurbish_done` 必须先 `Wipe` 成功。`Platform` 为 nil 时返回 `ErrNoPlatform`，不会跳过。
- 副作用先于落库，落库失败时副作用已生效；因为都幂等，重试同一动作即可。
- 批次导入先 `Enroll` 全部公钥，再一个事务入库；入库失败时已登记的公钥留在 palpabled（同 MAC 同公钥重复登记返回成功，所以重试无害）。
- 登录：同一用户名 10 分钟内连续输错 5 次锁定 10 分钟；用户不存在时也做一次 PBKDF2，避免靠耗时枚举用户名。停用账号与改口令都会清掉其全部会话。
- HTTP：请求体限长、拒绝未知字段与多余内容；错误响应只给通用文字（`classify`），细节进日志；API 响应带 `Cache-Control: no-store`。
- 单元视图 **不返回公钥与 `owner_ref`**，只给指纹和"有没有主人"。
- 批量动作逐台执行、逐台返回结果，不做"全成功或全失败"，界面上列出失败的几台。

## 配置（`server/admin.config.yaml`，不入库）

见 [admin.config.example.yaml](../../server/admin.config.example.yaml)。`platform.mode`：`http`（生产）、`none`（默认，需要 palpabled 的动作被拒绝）、`noop`（仅开发，擦除与挂失"假成功"，启动时打警告）。

## 运行

```bash
cp server/admin.config.example.yaml server/admin.config.yaml   # 填 bootstrap_admin，开发可用 platform.mode: noop
make dev-admin        # 后端 :8090
make dev-admin-web    # 页面 :5174，/api 代理到 :8090
# 构建后由后端直接提供页面：cd admin && pnpm build，static_dir 指向 admin/dist
```

## 页面（`admin/`）

React 19 + TypeScript strict + Vite + Tailwind 4 + **shadcn/ui**（`src/components/ui/` 由 `shadcn` CLI 生成，不手改，要换样式改 `index.css` 的主题变量）。

| 页面 | 用途 |
| --- | --- |
| 概览 | 各环节设备数（点击进入台账）、待核对异常、最近动态 |
| 设备台账 | 按状态/批次/锁定筛选，搜序列号、MAC、订单号；多选后做批量动作（只列所选设备共有的动作） |
| 设备详情 | 信息、销售与归属、可做的动作按钮、完整履历 |
| 生产批次 | 批次列表；导入批次（粘贴"MAC 公钥"清单） |
| 渠道、人员 | 仅 admin 可改 |

页面只负责展示与提交；"能做什么"来自接口返回的 `actions`，规则一律在服务端。会话令牌放 `sessionStorage`，关闭标签页即退出。
