# 可感 Palpable 模块拆解与任务包

> 依据：[产品文档](product.md)、[架构](architecture.md)、[设备协议](../protocol/README.md)
> 目的：让多人能并行开发；每个模块有清晰的职责、对外接口、数据归属和验收标准。

## 1. 拆分原则

1. **按"谁拥有状态"拆。** 每份状态只有一个模块能写，其他模块只读或经接口调用。
2. **先定接口再写实现。** 第 4 节给出 Go 接口约定，各模块可以先对着接口开发、用假实现联调。
3. **区分"设备行为"和"Web 展示行为"。** 设备行为要能在未来由固件复现；Web 展示行为只存在于演示，不进协议。
4. **设备只做传感器和执行器。** 好友、频道、屏蔽、认主、历史记录等一律在服务端。
5. **AI 能力交给灵矽，我们只做"接线"和"业务工具"。** 见第 2 节。

### 设备行为与 Web 展示行为

| 行为 | 归属 | 说明 |
| --- | --- | --- |
| 开箱动画、取出玩偶、放到桌面 | Web 展示 | 不进协议 |
| 公园场景、地图、小朋友走动 | Web 展示 | 位置通过设备的 `location` 上报进入协议 |
| 家长端手机窗口 | Web 展示 | 真实形态是微信小程序，调用同一套家长 API |
| 第一次上线问候、命名、认主 | 设备 | 由服务端驱动 |
| 唤醒、说话、收听、灯光、振动、提示音 | 设备 | 协议已有 |
| 贴近发现（`proximity`）、定位（`location`） | 设备 | 协议已有；Web 用地图距离模拟 |
| 设备端工具（灯、振动、摄像头） | 设备 | 以设备端 MCP 暴露给 AI，见第 2 节 |

## 2. 灵矽 AI 就是玩偶小伙伴的"大脑"

把灵矽的智能体当作玩偶小伙伴虚拟的 AI 本体：它有记忆，能听会说，记得"这个声音是谁"，能看图片，还能通过 MCP 扩展本领。可感不自己做这些，只负责把它接到玩偶上，并给它配上业务工具。

| 能力 | 灵矽提供什么 | 可感要做什么 |
| --- | --- | --- |
| 听与说 | ASR、LLM、TTS、免按键的服务端 VAD | 桥接音频（M1 已完成） |
| 记忆 | 按智能体的长期记忆：对话归档后抽取记忆片段与"记忆焦点"，运行时召回 | 为每个玩偶配置记忆焦点（孩子的名字、喜好、朋友等）；确认记忆的粒度与归属 |
| 认得声音 | 声纹：每个智能体登记说话人，每轮语音识别后把"系统声纹标注"附给 AI，AI 据此区分谁在说话 | 认主时登记主人声纹；其他声音识别为"未识别" |
| 看图片 | 视觉模型：设备端拍照后上传到灵矽的视觉解释接口，AI 拿到画面描述 | 提供"拍照"的设备端工具；Web 演示用场景截图代替摄像头 |
| 云端 MCP 扩展 | 智能体可配置多个 MCP 服务，对话中按需连接并调用其工具 | **可感自己提供一个 MCP 服务**，暴露建群、加群、切群、查好友等业务工具 |
| 设备端 MCP | 设备在 `hello` 声明支持 MCP，之后通过 `mcp` 消息（JSON-RPC）向 AI 暴露自己的工具 | 网关透传 `mcp` 消息；Web 模拟器与固件各自实现设备端工具 |

结论：
- **语音建群、切群**不再靠"自定义工具调用下发给设备"，而是做成可感自己的 **MCP 服务**，由灵矽的智能体在对话中调用。这样 AI 理解指令，业务规则仍在我们的服务端。
- **灯光、振动、拍照**做成**设备端 MCP 工具**，AI 可以主动用灯光表达情绪、用振动提示，而不是只靠服务端根据状态硬编码。
- **认主**依赖灵矽的声纹识别结果，我们不再自己做"校验是不是主人"。

> 以上能力依据灵矽的设计文档与源码整理（声纹 v2 观察、长期记忆、视觉、MCP 动态加载）。智能体的创建与配置接口（尤其是给智能体挂 MCP 服务、登记声纹说话人）我还没有验证，列在第 10 节待决事项。

## 3. 服务端模块（`server/internal/`）

状态：✅ 已完成　🚧 部分完成　⬜ 待做

| 模块 | 状态 | 职责 | 拥有的数据 | 依赖 |
| --- | --- | --- | --- | --- |
| `protocol` | ✅ | 设备协议类型、编解码、校验 | — | — |
| `gateway` | ✅ | 设备 WS 接入、握手、会话、路由 | 在线会话表 | protocol |
| `bridge/linx` | 🚧 | 设备 ↔ 灵矽桥接。已有：收音转发、TTS 回传、状态映射。待做：`mcp` 消息透传、`Say`（让设备说一句固定话）、为设备选择智能体（`X-Agent-ID`） | 灵矽会话 | gateway |
| `agent` | ⬜ | 玩偶智能体的开通与配置：人设（儿童友好）、记忆焦点、挂载可感 MCP 服务、登记声纹说话人 | 设备与智能体的对应关系 | bridge、store |
| `mcpserver` | ⬜ | **可感的云端 MCP 服务**：把业务能力做成工具给 AI 调用（建群、加群、切群、当前群、好友列表、附近的小朋友） | — | channel、relation |
| `store` | ⬜ | 存储接口与 SQLite 实现、迁移 | 全部持久化数据 | — |
| `identity` | ⬜ | 设备档案与认主：保存名字、主人声纹绑定标识；首次上线引导 | 设备档案 | agent、store |
| `relation` | ⬜ | 好友关系：贴近配对、待确认、已确认、解除 | 好友关系表 | store |
| `zone` | ⬜ | 坐标 → 区块 → 频道映射（演示只有公园） | 区块配置 | — |
| `channel` | ⬜ | 频道与群：成员、4 位临时码、切换、音频转发、`presence`。**唯一决定"谁能听见谁"的地方** | 频道成员（内存）、群与临时码 | relation、zone、gateway |
| `history` | ⬜ | 今日回顾：遇到了哪些小朋友、分享了什么画面、聊天摘要（摘要可来自灵矽的对话归档） | 历史事件表 | store |
| `describe` | ⬜ | 画面分享：AI 经设备端拍照工具得到画面描述，播放给频道里的好友并写入历史 | — | bridge、channel、history |
| `parentapi` | ⬜ | 家长端 REST API：好友与群聊、今日回顾、确认或解除好友、屏蔽 | — | relation、history、identity、channel |
| `world` | ⬜ | 演示用观察通道：向地图推送设备位置、谁在说话、频道成员 | — | gateway、channel |

模块关系中最重要的一条：**AI 通过 `mcpserver` 影响频道，但 `channel` 仍然是唯一裁决者。** `mcpserver` 只是 `channel` 与 `relation` 的另一个入口，不拥有规则。

## 4. 接口约定（Go）

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

// agent：玩偶智能体
type Agents interface {
    Ensure(ctx context.Context, d DeviceID) (AgentID, error)            // 首次上线时开通
    SetPersona(ctx context.Context, d DeviceID, name string) error      // 孩子起名后更新人设
    EnrollOwnerVoice(ctx context.Context, d DeviceID, wav []byte) error // 登记主人声纹
}

// identity：设备档案
type Identity interface {
    Profile(ctx context.Context, d DeviceID) (Profile, error) // 名字、是否已认主
    MarkOwned(ctx context.Context, d DeviceID, voiceID string) error
}

// bridge：让设备说话
type Speaker interface {
    Say(ctx context.Context, d DeviceID, text string) error // 不经 LLM，直接合成
}

// history：只追加的事件日志
type History interface {
    Record(ctx context.Context, e Event) error
    Today(ctx context.Context, d DeviceID) (Digest, error) // 遇到谁、分享了什么、聊了什么
}
```

约定：
- `DeviceID` 与协议 `hello.device_id` 一致，所有模块都用它作为设备主键。
- 所有接口方法带 `context.Context`，随设备连接的生命周期取消。
- `channel.OnAudio` 是热路径：不加长锁、不打日志、不阻塞。

## 5. 协议增量（相对 M0/M1）

只列新增项；先改 `protocol/palpable.schema.json` 与 `examples/`，再改实现。

| 方向 | 消息 | 用途 |
| --- | --- | --- |
| 双向 | `mcp`（`payload`：JSON-RPC） | 设备端 MCP。设备在 `hello.capabilities` 声明 `mcp`，网关把设备与灵矽之间的 `mcp` 消息原样透传，不解析工具内容 |
| 下行 | `channel.state`（`name`、`color`、`event`：joined / switched / left） | 设备用灯色和提示音确认当前群 |
| 下行 | `friend.state`（`event`：pending / confirmed / removed，`device_id`、`name`） | 贴近成为好友时的震动与提示 |

不再新增 `scene` 之类仅 Web 用的上行消息：画面分享走设备端 MCP 的拍照工具。语音指令也不新增设备协议，由灵矽的智能体调用可感的云端 MCP 服务。

### 设备端 MCP 工具（Web 模拟器与固件各自实现）

| 工具 | 作用 |
| --- | --- |
| `self.light.set` | 设置灯带状态、颜色、节奏，让 AI 用灯光表达情绪 |
| `self.haptic.pulse` | 触发振动模式 |
| `self.camera.take_photo` | 拍照并上传到灵矽的视觉解释接口，返回画面描述。Web 演示用 three.js 场景截图代替摄像头 |
| `self.audio.set_volume` | 调节音量 |

### 可感云端 MCP 工具（`mcpserver` 提供）

| 工具 | 作用 | 落到的模块 |
| --- | --- | --- |
| `create_group` | 创建群聊，返回 4 位临时码 | `channel` |
| `join_group` | 按 4 位码加入群聊 | `channel` |
| `switch_group` | 切换到某个群或"下一个/上一个" | `channel` |
| `current_group` | 当前在哪个群、群里有谁 | `channel` |
| `list_friends` | 好友列表 | `relation` |
| `who_is_nearby` | 附近有哪些已是好友的小朋友 | `relation`、`zone` |

工具的调用身份由连接带来：每个玩偶智能体挂载可感 MCP 服务时携带该设备的凭证，`mcpserver` 据此知道是哪台设备在调用，工具参数里不让 AI 自己填设备 ID。

## 6. 家长端 API（`parentapi`，REST）

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

## 7. 数据模型（`store`）

| 表 | 关键字段 |
| --- | --- |
| `device` | `device_id`、`name`、`agent_id`、`owner_voice_id`、`guardian_id`、`created_at` |
| `guardian` | `guardian_id`、`display_name` |
| `friendship` | `a`、`b`、`status`（pending / confirmed / removed）、`confirmed_by`、`created_at` |
| `group` | `group_id`、`owner`、`code`、`expires_at` |
| `group_member` | `group_id`、`device_id` |
| `history_event` | `id`、`device_id`、`kind`（chat_summary / met_friend / shared_scene）、`payload`、`at` |

声纹原始音频不入库；只保存灵矽声纹服务返回的绑定标识。记忆本体存在灵矽，不在我们的库里重复保存。

## 8. 前端模块（`web/src/`）

| 模块 | 状态 | 类别 | 职责 |
| --- | --- | --- | --- |
| `shared` | 🚧 | — | 协议类型（生成）、WS 客户端、store |
| `device-sim` | 🚧 | 设备 | 虚拟设备：麦克风与扬声器已有；待补 `proximity`、`location`、`channel.state`、`friend.state` 的响应，以及**设备端 MCP 服务**（响应 `tools/list`、`tools/call`） |
| `plush` | 🚧 | 设备表现 | 玩偶 3D 与灯带，已有状态灯；待补皮套换肤、说话与收听动画，并接受 `self.light.set` 驱动 |
| `scenes/unbox` | ⬜ | Web 展示 | 开箱：包装盒、打开、取出、放到桌面 |
| `scenes/desk` | ⬜ | Web 展示 | 桌面场景：与 AI 对话、命名、认主引导 |
| `scenes/park` | ⬜ | Web 展示 | 公园：多个小朋友和玩偶，走动，靠近，对话气泡；提供场景截图给拍照工具 |
| `map` | ⬜ | Web 展示 | 地图与区块，显示位置、频道范围、谁在说话（订阅 `world`） |
| `parent-sim` | ⬜ | Web 展示 | 手机窗口：好友与群聊、今日回顾、确认好友；调用 `parentapi` |
| `ui` | ⬜ | Web 展示 | 儿童友好的文案、字幕、提示音 |

多设备演示：同一页面可开多个"小朋友窗口"，每个是独立的 `device-sim` 实例；也可以多个浏览器分别接入。

## 9. 任务包

每个任务包可独立开发，括号内是依赖。验收标准写明"怎样算做完"。

### 服务端

| 编号 | 任务 | 依赖 | 验收 |
| --- | --- | --- | --- |
| S0 | **验证灵矽接入能力**：创建智能体、挂 MCP 服务、登记声纹说话人、设备端 MCP 透传 | M1 | 用最小例子跑通：AI 调用一个自建 MCP 工具；设备端暴露一个工具被 AI 调用；结论写进待决事项 |
| S1 | `store`：SQLite、迁移、第 7 节的表、仓储接口 | — | 增删改查有测试；换假实现不改业务代码 |
| S2 | `bridge/linx`：`mcp` 消息透传、`Say`、按设备选择智能体 | S0 | 假灵矽下发 `tools/call` 能到设备并带回结果；`Say` 不经 LLM 直出语音 |
| S3 | `agent` + `identity`：开通智能体、起名更新人设、登记主人声纹、首次上线引导 | S0、S1、S2 | 首次上线触发问候；说名字后保存；主人声纹登记成功，AI 能区分"主人"与"未识别" |
| S4 | `relation`：好友状态机与 `proximity` 处理 | S1 | 贴近产生 pending；确认后 `AreFriends` 为真；解除后为假 |
| S5 | `zone` + `channel`：频道、成员、音频转发、`presence` | S4 | 非好友收不到音频；好友收到；加入频道好友收到提示 |
| S6 | `mcpserver`：建群、加群、切群、当前群、好友、附近的小朋友 | S0、S5 | AI 说"创建群聊"能拿到 4 位码并播报；4 位码过期失效；切群后设备收到 `channel.state` |
| S7 | `history`：事件记录与今日回顾 | S1 | 一天内遇到的小朋友、分享记录、聊天摘要可查询 |
| S8 | `describe`：画面分享 | S2、S5、S7 | 小朋友让小伙伴描述画面，AI 通过拍照工具得到描述，频道好友听到，孩子本人也听到，并写入历史 |
| S9 | `parentapi`：第 6 节接口 | S4、S7 | 接口有测试；未授权请求被拒绝 |
| S10 | `world`：观察通道 | S5 | 地图能实时看到位置与谁在说话 |

### 前端

| 编号 | 任务 | 依赖 | 验收 |
| --- | --- | --- | --- |
| W1 | `scenes/unbox` 开箱场景 | — | 点击盒子打开，取出玩偶，放到桌面，进入桌面场景 |
| W2 | `scenes/desk` + `plush` 动画 | M1 | 灯带随 listening / thinking / speaking 变化；命名对话可走通 |
| W3 | `device-sim` 补 `proximity` / `location` 与新下行消息 | S4、S5 的协议部分 | 地图上靠近触发 `proximity`；收到 `friend.state` 时玩偶抖动并播放提示音 |
| W4 | `device-sim` 设备端 MCP：`self.light.set`、`self.haptic.pulse`、`self.camera.take_photo` | S0、S2 | AI 调用灯光工具玩偶变色；拍照工具返回场景截图的描述 |
| W5 | `scenes/park` + `map` 多设备 | S10、W3 | 两个浏览器进入公园，地图同步显示；非好友标注"已屏蔽" |
| W6 | `parent-sim` 手机窗口 | S9 | 能看好友、确认好友、看今日回顾 |
| W7 | `ui` 儿童文案与提示音 | — | 所有文案走统一文案表，无诱导留下个人信息的措辞 |

### 并行路线

```
S0（验证灵矽能力，最先做）
轨道 A（AI 与设备端）  S0 → S2 → S3 ─────────→ S8，W4
轨道 B（关系与频道）    S1 → S4 → S5 → S6 / S10
轨道 C（记录与家长）    S1 → S7 → S9
前端                   W1、W7 可立即开始；W2 依赖 M1；W3、W5 跟随 S4/S5/S10；W6 跟随 S9
```

关键路径：`S1 → S4 → S5 → S10 → W5`，对应演示主线的"陌生人屏蔽、贴近成为好友、好友互相听见"，应优先安排。**S0 是最大的不确定项，要最先做**，因为 S2、S3、S6、S8、W4 都依赖它的结论。

## 10. 待决事项

| 事项 | 影响 | 建议 |
| --- | --- | --- |
| 灵矽是否提供创建与配置智能体的接口（挂 MCP 服务、登记声纹说话人、设置人设与记忆焦点） | `agent`、`mcpserver`、`identity` 能否自动化 | S0 先验证；若没有接口，演示阶段手动配置一个模板智能体，所有玩偶共用，记忆按设备区分 |
| 一个玩偶一个智能体，还是共用模板智能体 | 记忆隔离与配置成本 | 记忆按智能体隔离，所以倾向一个玩偶一个智能体；若创建成本高则退化为共用模板，记忆区分靠设备标识 |
| 灵矽声纹登记需要 wav，而设备上行是 Opus | `agent.EnrollOwnerVoice` 需要服务端把 Opus 解码成 wav | 确认 Go 侧解码方案（CGO 的 libopus 或纯 Go 实现），Windows 上免 CGO 更省事；或确认能否用浏览器直接上传 wav |
| 可感云端 MCP 服务如何识别调用的是哪台设备 | `mcpserver` 的安全与归属 | 每个智能体挂载 MCP 时带设备凭证；需确认灵矽是否支持按智能体配置请求头 |
| 拍照工具的上传流程（设备把图片传到灵矽视觉解释接口，需要它下发的地址与令牌） | `describe` 与 W4 | 按灵矽视觉文档的流程在 S0 里跑通；Web 用 canvas 截图 |
| `mcp` 透传的安全边界 | 设备端工具由 AI 调用，输出会进入对话 | 网关只透传不解析；工具返回内容需要长度限制，避免被滥用 |
| 家长确认在演示中的简化 | `relation` 的状态机是否多一步 | 保留 pending 状态，演示中用家长端窗口或孩子语音确认 |
