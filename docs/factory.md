# 厂家后台（palpable-admin）：设备从出厂到报废的全生命周期

> 面向厂家运营、仓库、销售、售后的内部后台。**独立进程、独立数据库**，与 `palpabled` 只通过一组窄接口交互。
> 模块规格见 [modules/factory.md](modules/factory.md)；完成状态见 [tasks.md](tasks.md)；设备认证与激活锁规则见 [trust.md](trust.md)。

## 1. 为什么需要它

现在系统只认"设备上线了没有"：MAC + 公钥 + 家长绑定。这回答不了硬件产品必须回答的问题：

| 问题 | 谁会问 |
| --- | --- |
| 这批货做了多少、质检过了多少、卡在哪一步 | 生产、仓库 |
| 这台设备现在在仓库、在途、在哪家店的货架上 | 仓库、销售 |
| 这台设备是谁卖的、什么时候卖的、还在不在保修期 | 售后、客服 |
| 收到退货后怎么保证孩子的数据已清、才能再卖 | 合规、售后 |
| 用户把设备二手卖了，新主人是谁接手的 | 客服（防纠纷、防盗） |
| 设备丢了或被偷了，怎么让它不能被别人用 | 客服、家长 |
| 设备没卖出去就激活了，是不是串货或内部流出 | 销售、运营 |

## 2. 调研：同类硬件的做法

> 以下是对公开做法的归纳，用来取舍设计，不是逐家核实的结论；上线前需要与实际代工厂和渠道再对一遍流程。

| 环节 | 通行做法 | 我们的取舍 |
| --- | --- | --- |
| 唯一标识 | 序列号（SN）+ MAC，生产线烧录并扫码绑定；SN 进出厂清单 | 序列号 `PP<批次>-<序号>`，与 MAC、公钥一起入库；公钥即现有的 `device_key` |
| 出厂清单 | 烧录工位导出（MAC、密钥材料），导入云端后设备才能上线（类似 IoT 平台的"预置/一机一密"） | 对应 `auth.enrollment: manifest`：后台导入批次即向 `palpabled` 下发公钥；私钥永远不出设备 |
| 质检 | 老化、音频、传感器、按键，不合格进返修或报废 | 质检通过/不通过/返修/报废四个动作 |
| 入库—发货—上架 | 入库 → 调拨 → 渠道收货 → 上架；全程扫码，以序列号为粒度 | 以序列号为粒度，渠道是名录项；支持批量发货 |
| 售出与保修 | 以销售订单为准，保修自售出日起算；翻新机另算较短保修 | `sold_at` 起算，新机 12 个月、翻新机 6 个月（可配置） |
| 激活 | 手机端激活后云端回写"已激活"，与售出记录交叉校验 | 家长端绑定成功由 `palpabled` 推送事件；**未售出先激活**记为异常，不拒绝服务，等人工核对 |
| 退货 | 7 天无理由 / 质量问题；退货入库后检测、翻新、再销售或报废 | 退货 → 收货 → 检测 → 翻新 → 翻新入库 → 再上架；**收货与翻新入库都要擦除成功** |
| 二手转让 | 苹果式激活锁：只有原主解绑，新主人才能激活；转让不经过厂家 | 沿用 trust.md §3：家长解绑后新家长用新绑定码接手；后台只记"换主人"，**看不到任何一方是谁** |
| 丢失/被盗 | 挂失后设备被云端拒绝服务，找回后解锁 | 挂失 = 在 `palpabled` 锁定该设备，所有连接被拒；解锁需客服操作 |
| 报废 | 回收处理，吊销设备凭证 | 报废即吊销公钥，设备永远连不上 |

儿童产品的额外约束：**厂家后台是成人运营系统，不接触孩子数据**。这决定了第 4 节的数据边界。

## 3. 生命周期

### 3.1 状态

| 状态 | 含义 | 设备在哪 |
| --- | --- | --- |
| `produced` | 已生产，公钥已登记，待质检 | 工厂 |
| `qc_failed` | 质检不合格 | 工厂返修区 |
| `in_stock` | 质检通过，新品入库 | 仓库 |
| `in_transit` | 发往渠道的途中 | 物流 |
| `on_shelf` | 渠道已收货并上架 | 店铺、电商仓 |
| `sold` | 已卖给消费者，尚未激活（或已解绑） | 消费者手里 |
| `activated` | 家长已绑定，正在使用 | 消费者手里 |
| `returning` | 用户申请退货，设备在寄回途中 | 物流 |
| `returned` | 已收到退货，**已擦除服务端数据** | 售后仓 |
| `inspecting` | 检测中 | 售后仓 |
| `refurbishing` | 翻新中（换外壳、电池、清洁） | 售后仓 |
| `refurbished` | 翻新入库，可作为翻新品再发货 | 仓库 |
| `repairing` | 售后维修，修好寄回原主 | 售后仓 |
| `scrapped` | 报废，公钥已吊销 | 回收 |

另有一个**正交的锁定标记** `locked`（原因：`lost` 丢失 / `stolen` 被盗 / `other`）：任何状态下都可能被挂失，所以不是一个状态。锁定期间只允许解锁和报废。

### 3.2 状态图

```text
                    质检不过          返修
  produced ───────▶ qc_failed ──────▶ produced
     │ 质检通过           │ 报废
     ▼                   ▼
  in_stock ◀──下架── on_shelf ◀─收货── in_transit ◀─发货── in_stock / refurbished
     │                   │售出
     │                   ▼
     │                 sold ◀───解绑────── activated ◀──绑定（palpabled 事件）── sold
     │                   │  │                  │
     │         ┌─────────┘  └─────维修─────▶ repairing ──修好─▶ 回到进入前状态
     │         │ 申请退货（sold/activated）
     │         ▼
     │     returning ──取消──▶ 回到进入前状态
     │         │ 收货（擦除）
     │         ▼
     │     returned ─▶ inspecting ─┬─▶ refurbished（无需翻新，擦除）
     │                             └─▶ refurbishing ─▶ refurbished（擦除）
     │
     └── 退货侧状态 / qc_failed / repairing ──▶ scrapped（吊销公钥）
```

### 3.3 动作表

| 动作 | 从 | 到 | 必填 | 谁能做 | 副作用（经 `Platform` 端口） |
| --- | --- | --- | --- | --- | --- |
| `qc_pass` | produced | in_stock | — | 仓库 | 擦除（清掉产线测试遗留的绑定） |
| `qc_fail` | produced | qc_failed | 备注 | 仓库 | — |
| `rework` | qc_failed | produced | — | 仓库 | — |
| `ship` | in_stock、refurbished | in_transit | 渠道 | 仓库 | — |
| `receive` | in_transit | on_shelf | — | 仓库、销售 | — |
| `unshelf` | on_shelf | in_stock（新品）/ refurbished（翻新品） | — | 仓库、销售 | — |
| `sell` | on_shelf | sold | 订单号 | 销售 | — |
| `return_request` | sold、activated | returning | 退货原因 | 客服 | — |
| `return_cancel` | returning | 进入前状态 | — | 客服 | — |
| `receive_return` | returning | returned | — | 仓库 | **擦除；清空上一任主人标记** |
| `inspect` | returned | inspecting | — | 仓库 | — |
| `inspect_pass` | inspecting | refurbished | 成色 A/B | 仓库 | 擦除 |
| `refurbish` | inspecting | refurbishing | — | 仓库 | — |
| `refurbish_done` | refurbishing | refurbished | 成色 A/B | 仓库 | 擦除 |
| `repair` | sold、activated | repairing | 故障说明 | 客服 | — |
| `repair_done` | repairing | 进入前状态 | — | 客服 | — |
| `scrap` | qc_failed、returned、inspecting、refurbishing、repairing | scrapped | 备注 | 仓库 | **吊销公钥** |
| `lock` | 除 scrapped 外的任意状态 | 不变 | 原因 | 客服 | 在 `palpabled` 锁定 |
| `unlock` | 任意（已锁定） | 不变 | — | 客服 | 在 `palpabled` 解锁 |
| `activated`（系统） | sold、activated 等 | activated | `owner_ref` | `palpabled` | — |
| `unbound`（系统） | activated | sold | — | `palpabled` | — |

规则：

- **进入可再销售状态前必须擦除成功**（`qc_pass`、`inspect_pass`、`refurbish_done`）；`receive_return` 同理。擦除失败则整个动作失败、状态不变。擦除是幂等的，重试安全。
- 每个动作写一条**只追加**的事件，记录谁、何时、从哪到哪、备注。事件不可改不可删。
- 并发用乐观版本号：两个人同时操作同一台设备，后到的得到冲突错误，重新加载再试。

### 3.4 二手转让与"换主人"

设备在使用期间发生"原主解绑 → 新家长绑定"：

```text
palpabled: unbound                 → 后台：activated → sold，记下上一任主人标记 prev_owner_ref
palpabled: activated(owner_ref=B)  → 后台：sold → activated；B ≠ prev_owner_ref → 事件 transferred，transfer_count + 1
```

- 厂家**不审批**转让（激活锁已由原主解绑保证）；后台只记账，用于客服核对"这台是不是二手"。
- 退货翻新再销售时，`receive_return` 清空 `prev_owner_ref`，下一个买家视为首任主人，不算转让。
- 转让不改变保修起算日；翻新再销售重新起算，期限按翻新品。

### 3.5 异常

| 异常 | 后台处理 |
| --- | --- |
| 设备在 `in_stock`、`in_transit`、`on_shelf`、`refurbished` 被家长激活（未登记售出） | 状态直接转 `activated`，事件标 `anomaly`，概览的异常列表待人工核对 |
| 退货、维修中的设备被家长绑定（比如解绑后又绑回来） | 记下主人，不算异常；结束后回到的状态相应调整 |
| 产线、质检、检测、翻新中的设备被绑定（多为测试） | 只记一条普通事件，状态不变；下一次擦除会清掉 |
| 已报废设备被激活 | 只记 `anomaly`，并再次吊销公钥 |
| `palpabled` 报告了后台没有的 MAC | 拒绝并记日志（说明清单与线上不一致） |

## 4. 数据边界：厂家看不到孩子

| 厂家后台**有** | 厂家后台**没有** |
| --- | --- |
| 序列号、MAC、公钥、批次、固件版本、成色 | 孩子名字、玩偶名字、声纹、好友、聊天与历史 |
| 状态、渠道、订单号、售出/保修日期 | 家长的微信信息、手机号 |
| 事件日志、运营人员账号 | 设备的位置 |
| `owner_ref`：`palpabled` 对家长标识做 **HMAC-SHA256** 后的不可逆值，只用来判断"是不是同一个人" | 能还原 `owner_ref` 的密钥（只在 `palpabled`） |

订单号属于商务信息，不得填写买家姓名、电话、地址；录入界面有提示，服务端对订单号只做长度和字符校验。

## 5. 两个进程怎么协作

```text
 运营人员 ──浏览器──▶ palpable-admin ───────────────▶ palpabled
                     │ 独立 SQLite                     │ 设备 WebSocket、家长端 API、出厂清单、激活锁
                     │                                 │
   批次入库   ── Platform 端口 ──▶  PUT    /internal/devices/{mac}/key     导入公钥
   擦除       ────────────────▶  POST   /internal/devices/{mac}/wipe    恢复出厂：删档案、好友、历史、声纹
   吊销       ────────────────▶  DELETE /internal/devices/{mac}/key     报废：公钥作废
   挂失/解锁  ────────────────▶  PUT    /internal/devices/{mac}/lock    {locked: bool}
                     ▲
   绑定/解绑事件 ── POST /internal/platform-events ── palpabled          {mac, event, owner_ref}
```

- 两个方向各用一个 Bearer 令牌，只放配置文件。`palpabled` 一侧的 `/internal/*` 不对公网开放。
- `Platform` 是后台声明的端口。**擦除是硬门槛**：没有可用的 `Platform` 时，需要擦除的动作直接返回错误，而不是跳过；开发环境用 `platform.mode: noop` 显式放行。
- palpabled 一侧：`opsapi`（`/internal/devices/*`，配置 `ops.token` 才开放）、`device.Registry`（公钥与封锁）、`adminhook`（事件推送，有界队列 + 退避重试，按顺序发送）。**锁定与吊销在网关握手时拦截**，在线连接立即断开；锁定保留数据，吊销永久封锁且不能再登记。
- `/internal` 与 `/ws`、`/api` 在同一个监听端口上，**必须在反向代理处屏蔽 `/internal/`**，只允许后台所在的内网访问；令牌至少 16 位，弱令牌启动即失败。
- 推送是尽力而为：palpabled 重启或后台长时间不通时事件可能丢失。台账与线上不一致时，以 palpabled 为准，由人在异常列表里核对；没有自动对账（见 tasks）。
- 副作用先于落库：先调 `Platform`，成功后再写状态和事件。落库失败时副作用已生效，但它们都是幂等的，重试同一动作即可。

## 6. 权限

| 角色 | 能做什么 |
| --- | --- |
| `admin` | 全部，含管理人员账号与渠道 |
| `warehouse` | 导入批次；质检、发货、收货、下架、退货收货、检测、翻新、报废 |
| `sales` | 收货、下架、售出 |
| `support` | 申请/取消退货、维修、挂失/解锁 |
| `viewer` | 只读 |

- 登录用户名 + 口令（PBKDF2-SHA256 存哈希），会话令牌只存 SHA-256；同一用户名 10 分钟内连续输错 5 次锁定 10 分钟。
- 首个管理员由配置里的 `bootstrap_admin` 在库里没有任何账号时创建，登录后应立即改口令。
- 所有写操作记录操作人；人员被停用后会话立即失效。

## 7. 范围

本期做：批次与出厂清单、全部动作与状态机、事件日志、渠道名录、人员与角色、概览与异常、管理页面（shadcn）、`Platform` 端口与 HTTP 实现、`palpabled` 事件入口。

不在本期（见 [tasks.md](tasks.md)）：`palpabled` 一侧的 `/internal/*` 实现与事件推送；订单系统对接（订单号目前手填）；物流单号与运费；固件 OTA 版本管理；多工厂/多租户；报表导出。
