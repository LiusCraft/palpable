# 灵矽对接：能力、事实与约束

> 来源：[灵矽官方文档](https://linx.qiniu.com/docs/)。灵矽的智能体就是玩偶小伙伴的"大脑"，可感只负责把它接到玩偶上。

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
| `Device-Id` 必须是 MAC 形式，且设备要先注册或绑定到智能体；`POST /xiaozhi/device/preregister` 可按 MAC 批量预注册。**伪 MAC 已确认可用** | 服务端由设备 ID 生成伪 MAC（`bridge/linx.PseudoMAC`），预注册后才能握手 |
| 握手头需要 `Authorization`、`Protocol-Version: 1`、`Device-Id`、`Client-Id`（UUID）；可选 `X-Agent-ID` 指定智能体 | 已在 M1 修正中实现 |
| `listen.mode` 推荐 `realtime`；`auto` 是唤醒词触发 | 已改为 `realtime` |
| 下行音频采样率可能是 24000 或 16000，帧长 20/40/60ms，以服务端 `hello` 的 `audio_params` 为准 | 协议新增 `audio.downlink`，设备按它解码 |
| `hello.agent_params.custom_replace_prompt` 可替换系统提示词里的 `{{变量}}` | 孩子与小伙伴的名字可每次会话注入，不必改智能体配置 |
| 智能体有 REST 接口（`POST/PUT/DELETE /xiaozhi/agent`），可设系统提示词、记忆模型、声纹模型、`extra` 参数 | linxapi 可自动开通智能体 |
| 切换智能体靠握手头 `X-Agent-ID`；会话内只能用 `listen.agent_params.voice` 切音色 | 智能体与设备的对应关系要在连接前定好 |
| 记忆值与片段按 `identify_id` 隔离，可读写 | 记忆隔离按设备做即可 |
| 声纹说话人绑定、"只回应已绑定说话人"都是**智能体级**配置 | 要做到"只有主人能和这个玩偶说话"，需要**每个玩偶一个智能体** |
| 声纹登记接收 `audio_url`，并可从智能体的 `chat-history` 里取最近的语音样本 | 不需要服务端把 Opus 转成 wav；认主时从聊天记录取样本登记 |
| 声纹识别结果只体现在智能体的行为上，接口不返回识别详情 | 代码里无法读到"当前说话的是不是主人" |
| 聊天记录接口按智能体查会话，消息含文本与音频地址；**无日期过滤、无摘要**；音频约 60 天后清理 | 今日回顾要自己按会话时间筛选，摘要先不做 |

## 待验证

| 事项 | 影响 | 处理 |
| --- | --- | --- |
| 从聊天记录取的样本能否成功登记声纹（格式、时长） | device 认主流程 | 联调时验证；样本太短就让孩子多说一句 |
| 拍照上传流程：端侧 MCP 初始化下发视觉地址与令牌，文档没写明图片如何上传 | share 与设备端拍照工具 | 联调时验证；Web 用 canvas 截图 |
| 网关合并业务工具后，灵矽能否正常列出并调用（含重名、分页） | bridge/mcp | 联调时验证 |
| 每个玩偶一个智能体的创建成本与数量限制 | device 的开通流程 | 联调时验证；成本高则退化为共用模板智能体，只放弃"只回应主人" |
| 今日回顾的摘要由谁生成 | history 工作量 | 先展示聊天原话与事件 |

## 备选：云端 MCP

若以后要把业务工具开放给其他智能体：实现 streamable HTTP 端点，在灵矽控制台手动添加。注意文档目前只写了 SSE（`GET /mcp` 加 `POST /mcp/message`），没有提 streamable HTTP，也没有说明认证，需要先向灵矽确认。
