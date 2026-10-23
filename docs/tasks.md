# 路线图与任务

> 本文是**完成状态的唯一出处**。模块契约见 [modules.md](modules.md)，演示主线见 [AGENTS.md](../AGENTS.md#演示约束)。
> 每个任务包可独立开发：先改 `protocol/`，再改服务端，最后改 Web；"依赖"列写前置任务，"验收"写怎样算做完。

状态：✅ 完成　🚧 进行中　⬜ 待做

## 里程碑

| 阶段 | 目标 | 任务 | 状态 |
| --- | --- | --- | --- |
| M0 | 骨架：协议、网关、Web 设备模拟器 | — | ✅ |
| M1 | 设备经后端与灵矽对话，灯带随状态变化（含对接修正） | — | ✅ |
| M2 | **命名认主**：设备自动开通、开箱、起名 | S1 S2 S6 S7 W1 W3 | ⬜ |
| M3 | **语音主线**：公园频道、陌生人屏蔽、贴近成为好友、好友互相听见 | S3 S4 S5 S11 W2 W5 | ⬜ |
| M4 | 主线之外：群聊、画面分享、今日回顾与家长端、设备工具、文案与提示音、引导期声音登记 | S8 S9 S10 S12 W4 W6 W7 | ⬜ |
| 之后 | 真实硬件、皮套系列、微信小程序、声纹认主、智能体入频道、B 端车队版 | — | — |

M2 + M3 即 AGENTS.md 规定的演示主线。W7（文案）不阻塞主线，但演示前应完成。

## 服务端

| 编号 | 任务 | 依赖 | 验收 | 状态 |
| --- | --- | --- | --- | --- |
| S1 | `store`：SQLite、迁移、各表、仓储接口与假实现；`id` 包 | — | 增删改查有测试；业务模块换假实现不改代码 | ✅ |
| S2 | **接入重构**：新建 `app`，`main.go` 只做组装；协议 `hello` 增加 `skin`、`device_id` 限定 MAC；`bridge/linx` 改为 `Open / Close / Forward / Interrupt / Say / OnAIState`，经 `DeviceLink` 收发，删除 `PseudoMAC` 与 `Config.AgentID`，`hello.agent_params` 注入名字，断线重连 | M1 | `app` 按[分发表](modules.md#app)转发消息；假灵矽下 `Say` 不经 LLM 直出语音；灵矽断线后自动重连，设备断开后无 goroutine 泄漏 | ✅ |
| S3 | `relation`：状态机、`proximity` 防抖、`AreFriends` 缓存、`FilterFriends`；协议新增 `friend.state` | S1 | 双方靠近 1.5 秒生成 pending；确认后 `AreFriends` 为真；解除立即为假并发事件 | ✅ |
| S4 | `channel` + `zone`：区域与群、4 位码、前台频道、滞回；成员用 `ParticipantID`；协议新增 `channel.state` | S1 | 入区发 `Entered`，`JoinCurrent` 后进频道；码过期失效；错码限流；`presence` 里没有非好友 | ✅ |
| S5 | **`voice`：上行路由、发言权、下行转发**；经 `Sink` 收发；协议 `sound` 增加 `busy` | S2、S3、S4 | 见下方"语音验收" | ✅ |
| S6 | `linxapi` + `device`：引导智能体与风格目录、`EnsureAll`；新设备自动绑定引导智能体；`Handoff` 按 `skin` 交棒到风格智能体并写记忆焦点；换皮套切换智能体；`stage` 为 fresh / named / ready | S1、S2 | 新 MAC 上线后自动绑定引导智能体并能对话；同一 MAC 重复上线不重复开通；`Handoff` 后设备绑定对应风格智能体，重复调用无副作用；`stage=named` 上线时重试交棒而不重新起名；换 `skin` 后切换到新风格；开通失败时设备收到 `error` 且仍可进频道 | 🚧 代码完成；真实环境下"能对话"依赖 OTA 激活，见[待验证](linx-integration.md#待验证) |
| S7 | `bridge/mcp` 代理核心 + `self.profile.set_names`、`self.zone.join`；协议新增 `mcp`；`app` 处理 `Named` 事件：等 AI 说完 → `Handoff` → 重连会话 → 新智能体问候 | S2、S6 | 假灵矽发 `tools/list` 得到合并后的列表（含设备分页）；`tools/call` 按名字路由；说出名字后档案 `stage=named`，AI 说完后自动交棒、会话换成风格智能体并用孩子起的名字问候；交棒失败时下次上线重试 | ✅ |
| S8 | 群聊与好友业务工具：`self.group.*`、`self.friends.*` | S4、S7 | "创建群聊"拿到 4 位码并播报；"加入群聊 XXXX"进群；"下一个群"切换 | ✅ |
| S9 | `history` + `share` + `self.share.publish` | S3、S4、S7 | 今日遇到的小朋友、分享可查；分享后好友与本人都听到描述，非好友听不到 | ✅ |
| S10 | `parentapi` | S3、S9 | 接口有测试；未授权与跨家长访问被拒绝 | ✅ |
| S11 | `world`，仅演示模式注册 | S4、S5 | 地图实时看到位置、谁在说话、谁被屏蔽；默认启动时 `/world` 不可访问 | ✅ |
| S12 | **引导期声音登记**（可选）：`self.voiceprint.enroll`、`device.EnrollVoice`、`linxapi` 声纹登记；`voice_consent` 与 `speaker_id` 入档案 | S6、S7，且先过[待验证](linx-integration.md#待验证)中的样本来源 | 无监护人同意时调用被拒绝且不影响交棒；有同意时登记成功并记下 `speaker_id`；样本取不到时跳过、起名与交棒照常；日志与库里无音频和声纹特征 | 🚧 代码与测试完成；真实样本来源待联调 |

### 语音验收（S5）

1. **路由：** 不在频道时音频只去 AI；在频道时只去频道；`wake` 后一个回合去 AI，之后回到频道；AI 不可用且不在频道时丢弃。
2. **屏蔽：** 同频道的非好友听不到；好友听得到；好友被解除后正在进行的转发立即中断。
3. **发言权：** 同一频道同时只有一个人的音频被转发；其他人收到 `busy`；发言人断线立即释放。
4. **优先级：** 听者正在播 AI 语音时，好友发言打断 AI；同一设备同一时刻只有一种来源的音频。
5. **背压：** 一个听者慢速消费不影响其他听者，队列满时丢最旧帧。
6. **延迟：** 服务端内部延迟 p95 < 100ms，每个 `Burst` 有首帧进出时间戳。

## 前端

| 编号 | 任务 | 依赖 | 验收 | 状态 |
| --- | --- | --- | --- | --- |
| W1 | `scenes/unbox` 开箱场景 | — | 点击盒子打开，取出玩偶，放到桌面，进入桌面场景 | ⬜ |
| W2 | `device-sim`：设备端 VAD 与 `mic.state`、`wake`；多来源下行音频；`busy`、`friend.state`、`channel.state` 的响应 | S3、S4、S5 的协议部分 | 说话时发 `mic.state`；好友说话时播放；占线时玩偶抖动并提示 | ⬜ |
| W3 | `scenes/desk` + `plush` 动画（Web 模拟器生成并保存 MAC、`hello` 带 `skin` 已随 S2 完成） | S2 | 灯带随 listening / thinking / speaking / peer_talking 变化；起名对话走通 | 🚧 |
| W4 | `device-sim` 设备端 MCP：`self.light.set`、`self.haptic.pulse`、`self.camera.take_photo` | S7 | AI 调用灯光工具玩偶变色；拍照工具返回场景截图的描述 | ⬜ |
| W5 | `scenes/park` + `map`：多设备、位置与 `proximity` 上报 | S4、S11 | 两个窗口进入公园，地图同步；靠近触发好友；非好友标注"已屏蔽" | ⬜ |
| W6 | `parent-sim` 手机窗口 | S10 | 能看好友、确认好友、看今日回顾 | ⬜ |
| W7 | `ui` 儿童文案与提示音 | — | 文案走统一文案表，无诱导留下个人信息的措辞 | ✅ |

## 并行路线

```
轨道 A（语音主线，关键路径）  S1 → S3 ┐
                              S1 → S4 ┼→ S5 → S11 → W5
                                 S2 ──┘
轨道 B（命名认主）            S2 → S6 → S7 → W3
轨道 C（M4）                  S7 → S8；S7 → S9 → S10 → W6；S7 → W4；S7 → S12
前端随时可做                  W1、W7
```

最先做 S1 与 S2：S2 是 `voice` 与开通的共同前置，S1 是所有领域模块的前置。
