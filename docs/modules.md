# 可感 Palpable 模块拆解与任务包

> 依据：[产品文档](product.md)、[架构](architecture.md)、[设备协议](../protocol/README.md)
> 目的：让多人能并行开发；每个模块有清晰的职责、对外接口、数据归属和验收标准。

## 1. 拆分原则

1. **按"谁拥有状态"拆。** 每份状态只有一个模块能写，其他模块只读或经接口调用。
2. **先定接口再写实现。** 下文第 3 节给出 Go 接口约定，各模块可以先对着接口开发、用假实现联调。
3. **区分"设备行为"和"Web 展示行为"。** 设备行为要能在未来由固件复现；Web 展示行为只存在于演示，不进协议。
4. **设备只做传感器和执行器。** 好友、频道、屏蔽、认主、历史记录等一律在服务端。

### 设备行为与 Web 展示行为

| 行为 | 归属 | 说明 |
| --- | --- | --- |
| 开箱动画、取出玩偶、放到桌面 | Web 展示 | 不进协议 |
| 公园场景、地图、小朋友走动 | Web 展示 | 位置通过设备的 `location` 上报进入协议 |
| 家长端手机窗口 | Web 展示 | 真实形态是微信小程序，调用同一套家长 API |
| 第一次上线问候、命名、认主 | 设备 | 由服务端驱动 |
| 唤醒、说话、收听、灯光、振动、提示音 | 设备 | 协议已有 |
| 贴近发现（`proximity`）、定位（`location`） | 设备 | 协议已有；Web 用地图距离模拟 |

## 2. 服务端模块（`server/internal/`）

状态：✅ 已完成　🚧 部分完成　⬜ 待做

| 模块 | 状态 | 职责 | 拥有的数据 | 依赖 |
| --- | --- | --- | --- | --- |
| `protocol` | ✅ | 设备协议类型、编解码、校验 | — | — |
| `gateway` | ✅ | 设备 WS 接入、握手、会话、路由 | 在线会话表 | protocol |
| `bridge/linx` | 🚧 | 设备 ↔ 灵矽对话桥接。已有：收音转发、TTS 回传、状态映射。待做：`Say`（让设备说一句固定话）、接收灵矽自定义工具调用 | 灵矽会话 | gateway |
| `store` | ⬜ | 存储接口与 SQLite 实现、迁移 | 全部持久化数据 | — |
| `identity` | ⬜ | 设备命名、认主（声纹绑定与校验）、"谁是主人" | 设备档案、声纹绑定 | bridge、store |
| `relation` | ⬜ | 好友关系：贴近配对、待确认、已确认、解除 | 好友关系表 | store |
| `zone` | ⬜ | 坐标 → 区块 → 频道映射（演示只有公园） | 区块配置 | — |
| `channel` | ⬜ | 频道与群：成员、4 位临时码、切换、音频转发、`presence`。**唯一决定"谁能听见谁"的地方** | 频道成员（内存）、群与临时码 | relation、zone、gateway |
| `command` | ⬜ | 把灵矽的语音意图（创建群、加入群、切群、当前什么群）转成对 channel 的调用 | — | bridge、channel |
| `history` | ⬜ | 记录今日回顾：聊了什么摘要、遇到了哪些小朋友、分享了什么画面 | 历史事件表 | store |
| `describe` | ⬜ | 画面描述：让小伙伴描述当前画面并播放给频道（Web 演示用场景快照，真实设备后续接摄像头与视觉模型） | — | bridge、channel、history |
| `parentapi` | ⬜ | 家长端 REST API：好友与群聊、今日回顾、确认或解除好友、屏蔽 | — | relation、history、identity、channel |
| `world` | ⬜ | 演示用观察通道：向地图推送设备位置、谁在说话、频道成员 | — | gateway、channel |

## 3. 接口约定（Go）

下列接口是模块之间的契约，放在各模块的 `api.go` 中；先写接口和假实现，再写真实实现。

```go
// relation：好友关系
type Relations interface {
    AreFriends(ctx context.Context, a, b DeviceID) (bool, error)
    Friends(ctx context.Context, d DeviceID) ([]Friend, error)
    Propose(ctx context.Context, from, to DeviceID) (Request, error) // 贴近触发，生成待确认
    Confirm(ctx context.Context, reqID string, by Guardian) error     // 家长（演示中可为孩子）确认
    Remove(ctx context.Context, a, b DeviceID) error
}

// channel：频道与群
type Channels interface {
    Join(ctx context.Context, d DeviceID, ch ChannelID) error
    Leave(ctx context.Context, d DeviceID) error
    CreateGroup(ctx context.Context, owner DeviceID, ttl GroupTTL) (code string, err error)
    JoinGroupByCode(ctx context.Context, d DeviceID, code string) error
    Switch(ctx context.Context, d DeviceID, target string) error // "货队"、"下一个"、"上一个"
    Current(ctx context.Context, d DeviceID) (ChannelInfo, error)
    OnAudio(d DeviceID, frame []byte) // 转发前必须过 Relations.AreFriends
}

// identity：认主与命名
type Identity interface {
    Profile(ctx context.Context, d DeviceID) (Profile, error) // 名字、是否已认主
    SetName(ctx context.Context, d DeviceID, name string) error
    Enroll(ctx context.Context, d DeviceID, wav []byte) error  // 录入主人声纹
    IsOwner(ctx context.Context, d DeviceID, wav []byte) (bool, error)
}

// bridge：让设备说话
type Speaker interface {
    Say(ctx context.Context, d DeviceID, text string) error // 不经 LLM，直接合成
}

// history：只追加的事件日志
type History interface {
    Record(ctx context.Context, e Event) error
    Today(ctx context.Context, d DeviceID) (Digest, error) // 聊了什么、遇到谁、分享了什么
}
```

约定：
- `DeviceID` 与协议 `hello.device_id` 一致，所有模块都用它作为设备主键。
- 所有接口方法带 `context.Context`，随设备连接的生命周期取消。
- `channel.OnAudio` 是热路径：不加长锁、不打日志、不阻塞。

## 4. 协议增量（相对 M0）

只列新增项；先改 `protocol/palpable.schema.json` 与 `examples/`，再改实现。

| 方向 | 消息 | 用途 |
| --- | --- | --- |
| 下行 | `channel.state`（`name`、`color`、`event`：joined / switched / left） | 设备用灯色和提示音确认当前群，对应"切群后灯光变色" |
| 下行 | `friend.state`（`event`：pending / confirmed / removed，`device_id`、`name`） | 贴近成为好友时的震动与提示 |
| 上行 | `scene`（`label`、`tags`） | **仅 Web 演示**：把场景快照交给 `describe` 生成描述；真实设备后续换成图像 |

语音指令（创建群聊、加入群聊 XXXX、切到某某群、当前什么群）**不新增设备协议**，由灵矽的自定义工具调用下发给 `bridge/linx`，再交给 `command` 模块。

## 5. 家长端 API（`parentapi`，REST）

家长端真实形态是微信小程序，演示用 Web 手机窗口，二者调用同一套接口。

| 方法与路径 | 说明 |
| --- | --- |
| `GET /api/kids` | 家长名下的孩子与玩偶 |
| `GET /api/kids/{id}/friends` | 好友列表（含待确认） |
| `POST /api/friend-requests/{id}/confirm` | 确认好友 |
| `DELETE /api/kids/{id}/friends/{fid}` | 解除好友 |
| `GET /api/kids/{id}/groups` | 所在群聊 |
| `GET /api/kids/{id}/digest?date=today` | 今日回顾：聊了什么、遇到谁、分享了什么 |

家长鉴权在演示阶段用固定演示令牌，真实小程序接入时换成微信登录；不得因为演示方便而绕开服务端规则。

## 6. 数据模型（`store`）

| 表 | 关键字段 |
| --- | --- |
| `device` | `device_id`、`name`、`owner_voice_id`、`guardian_id`、`created_at` |
| `guardian` | `guardian_id`、`display_name` |
| `friendship` | `a`、`b`、`status`（pending / confirmed / removed）、`confirmed_by`、`created_at` |
| `group` | `group_id`、`owner`、`code`、`expires_at` |
| `group_member` | `group_id`、`device_id` |
| `history_event` | `id`、`device_id`、`kind`（chat_summary / met_friend / shared_scene）、`payload`、`at` |

声纹原始音频不入库；只保存声纹服务返回的绑定标识。

## 7. 前端模块（`web/src/`）

| 模块 | 状态 | 类别 | 职责 |
| --- | --- | --- | --- |
| `shared` | 🚧 | — | 协议类型（生成）、WS 客户端、store |
| `device-sim` | 🚧 | 设备 | 虚拟设备七项能力：麦克风与扬声器已有，待补 `proximity`、`location`、`channel.state`、`friend.state` 的响应 |
| `plush` | 🚧 | 设备表现 | 玩偶 3D 与灯带，已有状态灯；待补皮套换肤、说话与收听动画 |
| `scenes/unbox` | ⬜ | Web 展示 | 开箱：包装盒、打开、取出、放到桌面 |
| `scenes/desk` | ⬜ | Web 展示 | 桌面场景：与 AI 对话、命名、认主引导 |
| `scenes/park` | ⬜ | Web 展示 | 公园：多个小朋友和玩偶，走动，靠近，对话气泡 |
| `map` | ⬜ | Web 展示 | 地图与区块，显示位置、频道范围、谁在说话（订阅 `world`） |
| `parent-sim` | ⬜ | Web 展示 | 手机窗口：好友与群聊、今日回顾、确认好友；调用 `parentapi` |
| `ui` | ⬜ | Web 展示 | 儿童友好的文案、字幕、提示音 |

多设备演示：同一页面可开多个"小朋友窗口"，每个是独立的 `device-sim` 实例；也可以多个浏览器分别接入。

## 8. 任务包

每个任务包可独立开发，括号内是依赖。验收标准写明"怎样算做完"。

### 服务端

| 编号 | 任务 | 依赖 | 验收 |
| --- | --- | --- | --- |
| S1 | `store`：SQLite、迁移、第 6 节的表、仓储接口 | — | 增删改查有测试；换假实现不改业务代码 |
| S2 | `bridge/linx` 补 `Say` 与自定义工具调用接收 | M1 | 假灵矽下发工具调用能到达订阅者；`Say` 不经 LLM 直出语音 |
| S3 | `identity`：命名、认主、声纹录入与校验 | S1、S2 | 首次上线触发问候；说名字后保存；非主人声音校验失败 |
| S4 | `relation`：好友状态机与 `proximity` 处理 | S1 | 贴近产生 pending；确认后 `AreFriends` 为真；解除后为假 |
| S5 | `zone` + `channel`：频道、成员、音频转发、`presence` | S4 | 非好友收不到音频；好友收到；加入频道好友收到提示 |
| S6 | `command`：语音建群、加群、切群、查询 | S2、S5 | 4 位码过期失效；切群后收到 `channel.state` |
| S7 | `history`：事件记录与今日回顾 | S1 | 一天内的聊天摘要、遇到的小朋友、分享记录可查询 |
| S8 | `describe`：画面描述并播放给频道 | S2、S5、S7 | 小朋友让小伙伴描述画面，频道好友听到，孩子本人也听到，并写入历史 |
| S9 | `parentapi`：第 5 节接口 | S4、S7 | 接口有测试；未授权请求被拒绝 |
| S10 | `world`：观察通道 | S5 | 地图能实时看到位置与谁在说话 |

### 前端

| 编号 | 任务 | 依赖 | 验收 |
| --- | --- | --- | --- |
| W1 | `scenes/unbox` 开箱场景 | — | 点击盒子打开，取出玩偶，放到桌面，进入桌面场景 |
| W2 | `scenes/desk` + `plush` 动画 | M1 | 灯带随 listening / thinking / speaking 变化；命名对话可走通 |
| W3 | `device-sim` 补 `proximity` / `location` 与新下行消息 | S4、S5 的协议部分 | 地图上靠近触发 `proximity`；收到 `friend.state` 时玩偶抖动并播放提示音 |
| W4 | `scenes/park` + `map` 多设备 | S10、W3 | 两个浏览器进入公园，地图同步显示；非好友标注"已屏蔽" |
| W5 | `parent-sim` 手机窗口 | S9 | 能看好友、确认好友、看今日回顾 |
| W6 | `ui` 儿童文案与提示音 | — | 所有文案走统一文案表，无诱导留下个人信息的措辞 |

### 并行路线

```
轨道 A（设备与 AI）   S2 → S3 ───────────────→ S8
轨道 B（关系与频道）   S1 → S4 → S5 → S6 / S10
轨道 C（记录与家长）   S1 → S7 → S9
前端                  W1、W6 可立即开始；W2 依赖 M1；W3、W4 跟随 S4/S5/S10；W5 跟随 S9
```

关键路径：`S1 → S4 → S5 → S10 → W4`，对应演示主线的"陌生人屏蔽、贴近成为好友、好友互相听见"，应优先安排。

## 9. 待决事项

| 事项 | 影响 | 建议 |
| --- | --- | --- |
| 灵矽声纹接口需要 wav，而设备上行是 Opus | `identity` 需要服务端把 Opus 解码成 wav | 先确认 Go 侧用哪种解码方案（CGO 的 libopus 或纯 Go 实现），Windows 上免 CGO 更省事 |
| 灵矽自定义工具调用的配置方式 | `command` 能否实现语音建群与切群 | 先在灵矽管理端建一个工具做验证，再定接口 |
| 设备首次上线时灵矽侧的设备登记 | 声纹接口要求设备已存在 | 在 `identity` 首次上线流程中完成登记 |
| 画面描述的演示实现 | `describe` 的工作量 | 演示用场景标签生成描述，不接真实视觉模型 |
| 家长确认在演示中的简化 | `relation` 的状态机是否多一步 | 保留 pending 状态，演示中用家长端窗口或孩子语音确认 |
