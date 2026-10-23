# 灵矽对接：能力、事实与约束

> 来源：[灵矽官方文档](https://linx.qiniu.com/docs/)。灵矽的智能体就是玩偶小伙伴的"大脑"，可感只负责把它接到玩偶上。
> 本文只记灵矽的事实与限制；据此做出的设计取舍写在 [architecture.md](architecture.md#2-关键决策)。

## 能力分工

| 能力 | 灵矽提供 | 可感要做 |
| --- | --- | --- |
| 听与说 | ASR、LLM、TTS；`listen.mode=realtime` 为全双工，说话可打断 AI | 桥接音频（bridge/linx） |
| 记忆 | 每个智能体一个记忆容器；按 `identify_id`（设备 MAC）隔离；有"记忆焦点"与"记忆片段"，均可读写 | 定义记忆焦点（孩子名字、喜好、朋友）；需要时写入记忆片段（device、linxapi） |
| 认得声音 | 声纹 V2：创建说话人、登记声纹、绑定到智能体；可开启"只回应已绑定的说话人" | 认主时登记并绑定（device） |
| 看图片 | 视觉模型；端侧 MCP 初始化时平台下发视觉接口地址与令牌 | 设备端拍照工具；Web 用场景截图代替 |
| 设备端 MCP | 设备 `hello` 声明 `mcp` 后，平台发 `initialize`、`tools/list`、`tools/call`；工具名形如 `self.模块.功能` | 网关做 MCP 代理，合并设备工具与业务工具（bridge/mcp） |
| 云端 MCP | 智能体可调用云端 MCP；调用时平台带 `X-Linx-Device-Id`、`X-Linx-Agent-Id`、`X-Linx-Session-Id` | **暂不做**，见"备选" |

## 已确认的事实与约束

| 事实 | 影响 |
| --- | --- |
| `Device-Id` 必须是 MAC 形式，且设备要先注册或绑定到智能体；`POST /xiaozhi/device/preregister` 可按 MAC 批量预注册并关联智能体。**伪 MAC 已确认可用** | 设备以自己的 MAC 连后端（Web 模拟器自行生成），后端发现没有绑定就自动预注册，再连灵矽；服务端不再生成伪 MAC |
| `PUT /v1/devices/{mac}/agent/{agent_id}` 可切换设备关联的智能体，默认迁移聊天记录（`disable_chat_history_migration` 可关闭） | 换皮套时用它切换风格智能体 |
| 握手头需要 `Authorization`、`Protocol-Version: 1`、`Device-Id`、`Client-Id`（UUID）；可选 `X-Agent-ID` 指定智能体 | 已在 M1 修正中实现 |
| `listen.mode` 推荐 `realtime`；`auto` 是唤醒词触发 | 已改为 `realtime` |
| 下行音频采样率可能是 24000 或 16000，帧长 20/40/60ms，以服务端 `hello` 的 `audio_params` 为准 | 协议新增 `audio.downlink`，设备按它解码 |
| `hello.agent_params.custom_replace_prompt` 可替换系统提示词里的 `{{变量}}` | 孩子与小伙伴的名字可每次会话注入，不必改智能体配置 |
| 智能体有 REST 接口（`POST/PUT/DELETE /xiaozhi/agent`），可设系统提示词、记忆模型、声纹模型、`extra` 参数 | linxapi 可自动开通智能体 |
| 切换智能体靠握手头 `X-Agent-ID`；会话内只能用 `listen.agent_params.voice` 切音色 | 智能体与设备的对应关系要在连接前定好 |
| 记忆值与片段按 `identify_id` 隔离，可读写 | 记忆隔离按设备做即可 |
| 声纹说话人绑定、"只回应已绑定说话人"都是**智能体级**配置 | 设备共用风格智能体时做不到"只有主人能和这个玩偶说话"，默认不做；需要时改为每台设备克隆一个智能体 |
| 声纹登记接收 `audio_url`，并可从智能体的 `chat-history` 里取最近的语音样本 | 认主是可选能力；做的话从聊天记录取样本登记，不需要服务端转 wav |
| 声纹识别结果只体现在智能体的行为上，接口不返回识别详情 | 代码里无法读到"当前说话的是不是主人" |
| 预注册后的 MAC 在**激活前**连 WebSocket，握手成功，但任何 `listen` 都只得到 `stt{error_code:10001, text:"请去绑定智能体。"}` 加一段同文的 TTS；预注册记录 `status=0`。已绑定的伪 MAC（如 `11:22:44:33:55:77`）可正常对话，其预注册记录在激活后消失 | 开通时以设备身份调用 OTA（`POST /v1/ota/`）完成绑定；`bridge/linx` 收到带 `error_code` 的 `stt` 时不把这段提示播给孩子，改发 `error` |
| OTA 激活地址：`https://xrobo.qiniu.com/xiaozhi/ota/` 可用（以设备身份 POST，返回 `websocket.token` 且没有 `activation` 即已绑定，预注册记录随之消失）；文档里的 `xrobo-io.qiniuapi.com/v1/ota/` 在开发网络返回 403 | `linx.ota_url` 默认用前者；开通时预注册后立即激活，新设备即可对话 |
| `listen{state: detect, text_response}` 原样播报指定文字（`stt` 回显该文字，`tts` 逐句念出），不改写 | `bridge/linx.Say` 用于欢迎、交棒问候、分享播报 |
| 聊天记录接口按智能体查会话，消息含文本与音频地址；**无日期过滤、无摘要**；音频约 60 天后清理 | 今日回顾要自己按会话时间筛选，摘要先不做 |
| 实测：会话列表与消息都带 `macAddress`（文档未写会话带） | 今日回顾按会话的 MAC 先过滤，再拉消息 |
| 实测：智能体默认 `chatHistoryConf=1` 只存文字，`/v1/agents/{id}/chat-history` 的语音样本为空 | 引导期声音登记（S12）取不到样本，见下方待决定 |
| 实测：提示词写"如果 {{voice_step}} 是 true 才做"，模型在 false 时仍会请孩子跟读 | `voice_step` 改为注入整段指令（做 / 不做），不让模型判断 |

## 待验证

| 事项 | 影响 | 处理 |
| --- | --- | --- |
| 记忆的 `identify_id` 用裸 MAC 还是 `mac_` 前缀（文档示例两种都有） | 记忆焦点写入孩子与小伙伴的名字 | 当前用裸 MAC；联调时看灵矽记忆里是否读得到 |
| `GET /v1/agents/{id}/chat-history` 的样本记录是否带设备 MAC | S12 能否确认样本属于当前设备 | `linxapi` 兼容 `mac_address` / `mac` / `device_id` 字段；都没有则 `MAC` 为空、不登记 |
| 从聊天记录取的样本能否成功登记声纹（格式、时长） | 引导期声音登记（S12）、可选的认主流程 | 联调时验证；样本太短就让孩子多说一句 |
| 引导智能体被所有新设备共用，聊天记录里的语音样本能否确认属于当前设备（会话是否带 MAC，或按时间窗口足以区分） | S12 能否做 | 联调时验证；确认不了就不登记，不改为服务端暂存音频 |
| 灵矽是否支持用孩子的声音做音色复刻（TTS 克隆） | "复刻孩子的声音"能否进路线图 | 向灵矽确认；未确认前只做声纹登记，儿童声音复刻另需监护人单独同意 |
| 切换智能体时关闭聊天记录迁移（`disable_chat_history_migration`）的效果；切换后旧会话是否需要主动断开 | 交棒 `Handoff` | 联调时验证 |
| 拍照上传流程：端侧 MCP 初始化下发视觉地址与令牌，文档没写明图片如何上传 | share 与设备端拍照工具 | 联调时验证；Web 用 canvas 截图 |
| 网关合并业务工具后，灵矽能否正常列出并调用（含重名、分页） | bridge/mcp | 联调时验证 |
| 风格智能体按名字查找：智能体列表接口按游标分页，`search` 只支持按 MAC 或智能体 ID | catalog 的 `EnsureAll` | 遍历列表按名字匹配，或把 `agent_id` 存进 `style` 表避免重复查找 |
| 预注册对已存在设备的返回（冲突码）与 `failed` 列表的含义 | device 的幂等 | 联调时验证 |
| 聊天记录能否按设备筛选：会话按智能体查询，而风格智能体被多台设备共用 | history 会不会把别的孩子的对话给家长看 | 联调时看会话是否带设备 MAC；不能筛选前，今日回顾不展示聊天 |
| 今日回顾的摘要由谁生成 | history 工作量 | 先展示聊天原话与事件 |

## 待产品决定

| 事项 | 冲突 | 选项 |
| --- | --- | --- |
| S12 需要引导智能体开启语音记录（`chatHistoryConf` 存音频）才能取到样本 | 产品承诺"不长期保留原始音频"，而灵矽的音频约 60 天才清理 | 不做 S12（默认）；或经监护人单独同意后仅对引导智能体开启，登记完成后删除会话 |

## 备选：云端 MCP

若以后要把业务工具开放给其他智能体：实现 streamable HTTP 端点，在灵矽控制台手动添加。注意文档目前只写了 SSE（`GET /mcp` 加 `POST /mcp/message`），没有提 streamable HTTP，也没有说明认证，需要先向灵矽确认。
