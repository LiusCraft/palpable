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
| 听与说 | ASR、LLM、TTS；`listen` 的 `realtime` 模式为全双工，说话可打断 AI | 桥接音频（M1 已完成，有待修正项，见 2.1） |
| 记忆 | 每个智能体一个记忆容器；按身份（`identify_id`，文档示例为设备 MAC）隔离；有"记忆焦点"（结构化字段）和"记忆片段"，均可通过 API 读写 | 开通容器，定义记忆焦点（孩子名字、喜好、朋友等）；需要时主动写入记忆片段，如"小兔是我的好朋友" |
| 认得声音 | 声纹 V2：创建说话人、登记声纹、把说话人绑到智能体；可开启"只回应已绑定的说话人" | 认主时登记主人声纹并绑定；开启"只和主人说话" |
| 看图片 | 视觉模型；端侧 MCP 初始化时平台下发视觉接口地址和令牌，设备把图片交给它 | 提供拍照的设备端工具；Web 演示用场景截图代替摄像头 |
| 云端 MCP 扩展 | 智能体可调用云端 MCP 服务；调用时平台带上设备、智能体、会话三个标识头 | **暂不做**，保留为备选（见第 10 节）。业务工具改走设备端 MCP |
| 设备端 MCP | 设备在 `hello` 声明 `mcp`，平台发 `initialize` 与 `tools/list`，之后按需 `tools/call`；工具名形如 `self.模块.功能` | 网关做 **MCP 代理**：设备自己的工具（灯光、振动、拍照）转给设备，业务工具（建群、切群等）由网关代设备提供，两者合并成一份工具列表给 AI；可用"设备工具描述 API"补充工具说明 |

结论：
- **语音建群、切群**做成**网关代设备提供的业务工具**，由灵矽的智能体在对话中调用。对灵矽来说它们就是"设备端工具"，不需要部署云端 MCP 服务、不需要在控制台挂载、也没有额外的认证问题；AI 理解指令，业务规则仍在我们的服务端。
- **灯光、振动、拍照**做成**设备端 MCP 工具**，AI 可以主动用灯光表达情绪、用振动提示。
- **认主**依赖灵矽的声纹识别，我们不再自己做"校验是不是主人"。

### 2.1 灵矽接入的已确认事实与约束

以下来自[灵矽官方文档](https://linx.qiniu.com/docs/)，直接影响设计。

| 事实 | 影响 |
| --- | --- |
| 握手 `Device-Id` 必须是设备 MAC，且设备要先在平台注册或绑定到智能体；可用 `POST /xiaozhi/device/preregister` 按 MAC 批量预注册并关联智能体 | 每台虚拟设备需要一个伪 MAC，由我们生成并预注册（**伪 MAC 已确认可用**）。`device` 表增加 `linx_mac` 字段；协议里的 `device_id` 仍可自由格式 |
| 握手头还需要 `Protocol-Version: 1`、`Client-Id`（UUID） | **M1 待修正**：桥接目前没带 `Protocol-Version`，`Client-Id` 也不是 UUID |
| `listen.mode` 推荐 `realtime`（全双工）；文档里 `auto` 是唤醒词触发 | **M1 待修正**：桥接目前用 `auto`，免按键对话应改为 `realtime` |
| 服务端下行音频的采样率可能是 24000 或 16000，帧长 20/40/60ms，以服务端 `hello` 返回的 `audio_params` 为准 | **M1 待修正**：我们的协议把上下行都写死为 16kHz，浏览器按 16k 解码，若灵矽返回 24k 会变调。需要把下行音频参数通过 `hello.ack` 转给设备 |
| `hello.agent_params.custom_replace_prompt` 可以替换系统提示词里的 `{{变量}}` | 孩子的名字、小伙伴的名字可以在每次会话开始时注入，不必改智能体配置 |
| 智能体的创建、更新、删除都有 REST 接口（`POST/PUT/DELETE /xiaozhi/agent`），可设置系统提示词、记忆模型、声纹模型、`extra` 参数等 | `agent` 模块可以自动化开通，不必手动配置 |
| 切换智能体靠握手头 `X-Agent-ID`；会话内只能用 `listen.agent_params.voice` 切换音色，不能切智能体 | 智能体与设备的对应关系要在连接前定好 |
| 记忆容器每个智能体一个；记忆值与片段按 `identify_id` 隔离，可读写 | 记忆隔离按设备做即可，不要求每个玩偶一个智能体 |
| 声纹说话人绑定与"只回应已绑定说话人"都是**智能体级**配置 | 要让"只有主人能和这个玩偶说话"，就需要**每个玩偶一个智能体**，否则孩子 A 的声纹会让孩子 B 的玩偶也认得 |
| 声纹登记接收的是 `audio_url`（平台可访问的地址），并且可以从智能体的 `chat-history` 接口里挑选最近的语音样本 | 不需要服务端自己把 Opus 解码成 wav：认主时让孩子说话，从聊天记录里取样本登记。样本格式文档未说明，需验证 |
| 声纹识别结果只体现在智能体的行为上，接口不返回识别详情 | 我们不能在代码里读到"当前说话的是不是主人"，只能依赖智能体的行为 |
| 云端 MCP 文档写的是 SSE：`GET /mcp` 长连接加 `POST /mcp/message`；在灵矽的 MCP 管理页登记地址；调用时平台自动带 `X-Linx-Device-Id`、`X-Linx-Agent-Id`、`X-Linx-Session-Id` | 已决定不做云端 MCP（见第 10 节备选）。若以后要做，注意文档只写了 SSE，团队说的是 streamable HTTP，需确认平台是否支持 |
| 云端 MCP 工具返回里可带 `structuredContent.funcs` 驱动设备动作；内置函数有 `linx_play`、`linx_stop`、`change_role` | 仅云端 MCP 备选方案会用到 |
| 聊天记录接口按智能体查会话，消息含文本和音频地址；**没有按日期过滤，也没有摘要**；音频约 60 天后清理 | `history` 的"今日回顾"要自己按会话创建时间筛选，摘要要自己生成或只展示原话 |

## 3. 服务端模块（`server/internal/`）

状态：✅ 已完成　🚧 部分完成　⬜ 待做

| 模块 | 状态 | 职责 | 拥有的数据 | 依赖 |
| --- | --- | --- | --- | --- |
| `protocol` | ✅ | 设备协议类型、编解码、校验 | — | — |
| `gateway` | ✅ | 设备 WS 接入、握手、会话、路由 | 在线会话表 | protocol |
| `bridge/linx` | 🚧 | 设备 ↔ 灵矽桥接。已有：收音转发、TTS 回传、状态映射。待做：`mcp` 消息透传、`Say`（让设备说一句固定话）、为设备选择智能体（`X-Agent-ID`） | 灵矽会话 | gateway |
| `agent` | ⬜ | 玩偶智能体的开通与配置：人设（儿童友好）、记忆焦点、挂载可感 MCP 服务、登记声纹说话人 | 设备与智能体的对应关系 | bridge、store |
| `bridge/mcp` | ⬜ | **MCP 代理**：解析灵矽与设备之间的 `mcp` JSON-RPC，合并设备自带工具与业务工具（建群、加群、切群、当前群、好友列表、附近的小朋友），按工具名把 `tools/call` 路由给设备或业务实现 | — | bridge/linx、channel、relation |
| `store` | ⬜ | 存储接口与 SQLite 实现、迁移 | 全部持久化数据 | — |
| `identity` | ⬜ | 设备档案与认主：保存名字、主人声纹绑定标识；首次上线引导 | 设备档案 | agent、store |
| `relation` | ⬜ | 好友关系：贴近配对、待确认、已确认、解除 | 好友关系表 | store |
| `zone` | ⬜ | 坐标 → 区块 → 频道映射（演示只有公园） | 区块配置 | — |
| `channel` | ⬜ | 频道与群：成员、4 位临时码、切换、音频转发、`presence`。**唯一决定"谁能听见谁"的地方** | 频道成员（内存）、群与临时码 | relation、zone、gateway |
| `history` | ⬜ | 今日回顾：遇到了哪些小朋友、分享了什么画面、聊天摘要（摘要可来自灵矽的对话归档） | 历史事件表 | store |
| `describe` | ⬜ | 画面分享：AI 经设备端拍照工具得到画面描述，播放给频道里的好友并写入历史 | — | bridge、channel、history |
| `parentapi` | ⬜ | 家长端 REST API：好友与群聊、今日回顾、确认或解除好友、屏蔽 | — | relation、history、identity、channel |
| `world` | ⬜ | 演示用观察通道：向地图推送设备位置、谁在说话、频道成员 | — | gateway、channel |

模块关系中最重要的一条：**AI 通过 `bridge/mcp` 的业务工具影响频道，但 `channel` 仍然是唯一裁决者。** `bridge/mcp` 只是 `channel` 与 `relation` 的另一个入口，不拥有规则；工具的调用身份来自连接本身（这条会话属于哪台设备），不让 AI 自己填设备 ID。

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

// agent：玩偶智能体（每个玩偶一个智能体，经灵矽 REST 开通）
type Agents interface {
    Ensure(ctx context.Context, d DeviceID) (AgentRef, error)    // 首次上线：创建智能体、记忆容器、预注册伪 MAC
    SetNames(ctx context.Context, d DeviceID, doll, kid string) error // 起名后写入会话变量与记忆焦点
    EnrollOwnerVoice(ctx context.Context, d DeviceID) error      // 从聊天记录取样本登记声纹，并绑定到智能体
    RememberFact(ctx context.Context, d DeviceID, fact string) error // 写入一条记忆片段，如"小兔是好朋友"
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
| 下行 | `hello.ack` 增加 `downlink_audio`（`sample_rate`、`frame_duration`） | 把灵矽下行音频参数转给设备，浏览器和固件按它配置 Opus 解码器，上行仍是 16kHz / 60ms |
| 双向 | `mcp`（`payload`：JSON-RPC） | 设备端 MCP。设备在 `hello.capabilities` 声明 `mcp`。灵矽发来的 `initialize` 与 `tools/list` 由网关合并业务工具后回复，`tools/call` 按工具名路由：设备自带工具转给设备，业务工具由网关执行。设备返回的结果原样回给灵矽 |
| 下行 | `channel.state`（`name`、`color`、`event`：joined / switched / left） | 设备用灯色和提示音确认当前群 |
| 下行 | `friend.state`（`event`：pending / confirmed / removed，`device_id`、`name`） | 贴近成为好友时的震动与提示 |

不再新增 `scene` 之类仅 Web 用的上行消息：画面分享走设备端 MCP 的拍照工具。语音指令也不新增设备协议，由灵矽的智能体调用业务工具，网关在内部处理。

### 设备端 MCP 工具（Web 模拟器与固件各自实现）

| 工具 | 作用 |
| --- | --- |
| `self.light.set` | 设置灯带状态、颜色、节奏，让 AI 用灯光表达情绪 |
| `self.haptic.pulse` | 触发振动模式 |
| `self.camera.take_photo` | 拍照并上传到灵矽的视觉解释接口，返回画面描述。Web 演示用 three.js 场景截图代替摄像头 |
| `self.audio.set_volume` | 调节音量 |

### 业务工具（`bridge/mcp` 代设备提供，固件无需实现）

| 工具 | 作用 | 落到的模块 |
| --- | --- | --- |
| `create_group` | 创建群聊，返回 4 位临时码 | `channel` |
| `join_group` | 按 4 位码加入群聊 | `channel` |
| `switch_group` | 切换到某个群或"下一个/上一个" | `channel` |
| `current_group` | 当前在哪个群、群里有谁 | `channel` |
| `list_friends` | 好友列表 | `relation` |
| `who_is_nearby` | 附近有哪些已是好友的小朋友 | `relation`、`zone` |

这些工具对灵矽来说和设备自带工具没有区别，名字统一用 `self.模块.功能` 规范（如 `self.group.create`、`self.group.join`、`self.group.switch`），避免和设备自带工具重名。工具的调用身份由连接带来，工具参数里不让 AI 自己填设备 ID。固件和 Web 模拟器只需实现设备自己的工具，业务升级不需要改设备。

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
| `device` | `device_id`、`linx_mac`（灵矽用的伪 MAC）、`name`、`agent_id`、`speaker_id`（灵矽声纹说话人）、`guardian_id`、`created_at` |
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
| S0 | **验证灵矽接入的剩余未知项**：① 从聊天记录取样本登记声纹是否成功；② 拍照上传流程；③ 网关合并业务工具后，灵矽能否正常列出并调用（含重名、分页）。伪 MAC 已确认可用，云端 MCP 不再做 | M1 | 每项给出"可行 / 不可行 / 需变通"的结论并写进待决事项；至少跑通：AI 调用一个网关提供的业务工具和一个设备自带工具 |
| S1 | `store`：SQLite、迁移、第 7 节的表、仓储接口 | — | 增删改查有测试；换假实现不改业务代码 |
| S2 | `bridge/linx`：**修正 M1 遗留项**（`realtime` 模式、`Protocol-Version` 与 UUID `Client-Id`、下行音频参数转给设备、`X-Agent-ID`）；`Say`；`hello.agent_params` 注入名字 | M1 | 假灵矽返回 24kHz 时设备按 24kHz 解码；`Say` 不经 LLM 直出语音 |
| S3 | `agent` + `identity`：开通智能体与记忆容器、预注册伪 MAC、起名写入记忆焦点、登记并绑定主人声纹、开启"只回应主人"、首次上线引导 | S0、S1、S2 | 首次上线触发问候；说名字后保存；主人声纹登记成功；非主人说话时玩偶不回应 |
| S4 | `relation`：好友状态机与 `proximity` 处理 | S1 | 贴近产生 pending；确认后 `AreFriends` 为真；解除后为假 |
| S5 | `zone` + `channel`：频道、成员、音频转发、`presence` | S4 | 非好友收不到音频；好友收到；加入频道好友收到提示 |
| S6 | `bridge/mcp`：MCP 代理与业务工具（建群、加群、切群、当前群、好友、附近的小朋友） | S2、S5 | 假灵矽发 `tools/list` 得到设备工具加业务工具；`tools/call` 按名字路由；AI 说"创建群聊"能拿到 4 位码并播报；4 位码过期失效；切群后设备收到 `channel.state` |
| S7 | `history`：事件记录与今日回顾（聊天内容取自灵矽聊天记录接口，按会话时间自行筛选） | S1 | 一天内遇到的小朋友、分享记录、聊天内容可查询 |
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
S0（验证剩余未知项，可与 S2 并行）
轨道 A（AI 与设备端）  S2 → S3 ─────────→ S8，W4
                              └→ S6（还需 S5）
轨道 B（关系与频道）    S1 → S4 → S5 → S6 / S10
轨道 C（记录与家长）    S1 → S7 → S9
前端                   W1、W7 可立即开始；W2 依赖 M1；W3、W5 跟随 S4/S5/S10；W6 跟随 S9
```

关键路径：`S1 → S4 → S5 → S10 → W5`，对应演示主线的"陌生人屏蔽、贴近成为好友、好友互相听见"，应优先安排。S2（修正 M1 遗留项）要最先做，因为它直接影响能不能听到正常的声音；S0 的剩余未知项主要影响 S3（声纹）与 S8（拍照），可以并行验证。

## 10. 待决事项

已确认的事实见 2.1。下面只列仍未解决的。

| 事项 | 影响 | 建议 |
| --- | --- | --- |
| 云端 MCP（备选，当前不做） | 若以后要把业务工具开放给其他智能体 | 实现 streamable HTTP 端点后在灵矽控制台手动添加；注意文档目前只写了 SSE，需确认平台是否支持 streamable HTTP；认证文档未说明 |
| 声纹登记用聊天记录里的样本，样本格式与时长是否满足要求 | `identity` 认主流程 | S0 验证；样本太短时让孩子多说一句再取样本 |
| 拍照上传流程：端侧 MCP 初始化下发视觉接口地址和令牌，文档没有写明图片如何上传 | `describe` 与 W4 | 参考灵矽视觉文档的接口，在 S0 里跑通；Web 用 canvas 截图 |
| 今日回顾的摘要由谁生成 | `history` 工作量 | 先展示聊天原话与事件，摘要后续再做 |
| MCP 代理的边界 | 网关要解析 JSON-RPC 并合并工具，设备工具输出会进入对话 | 业务工具名固定前缀避免重名；设备返回内容限制长度；设备声明工具过多时分页要处理 |
| 家长确认在演示中的简化 | `relation` 的状态机是否多一步 | 保留 pending 状态，演示中用家长端窗口或孩子语音确认 |
