# 路线图与任务

> 本文是**完成状态的唯一出处**。模块设计见 [modules/](modules/README.md)，演示主线见 [AGENTS.md](../AGENTS.md#演示约束)。
> 每个任务包可独立开发：先改 `protocol/`，再改服务端，最后改 Web；"依赖"列写前置任务，"验收"写怎样算做完。

状态：✅ 完成　🚧 进行中　⬜ 待做

## 里程碑

| 阶段 | 目标 | 任务 | 状态 |
| --- | --- | --- | --- |
| M0 | 骨架：协议、网关、Web 设备模拟器 | — | ✅ |
| M1 | 设备经后端与灵矽对话，灯带随状态变化（含对接修正） | — | ✅ |
| M2 | **命名认主**：设备自动开通、开箱、起名 | S1 S2 S6 S7 W1 W3 | ✅（新设备直连对话依赖 OTA 激活，见 S6） |
| M3 | **语音主线**：公园频道、陌生人屏蔽、贴近成为好友、好友互相听见 | S3 S4 S5 S11 W2 W5 | ✅ |
| M4 | 主线之外：群聊、画面分享、今日回顾与家长端、设备工具、文案与提示音、引导期声音登记 | S8 S9 S10 S12 W4 W6 W7 | ✅（S12 样本来源待联调） |
| M5 | **信任与同意**：设备认证、家长先绑定、交友两关同意、小伙伴进频道（回应初评反馈，规则见 [trust.md](trust.md)） | S13 S14 S15 S16 | 🚧（S17、F5 待做） |
| M6 | **家长端小程序上线**：微信登录与账号、面向真实家长的小程序、声纹开关、交友请求提醒 | S18 S19 S20 P1 P2 | 🚧（S19、P2 待做；上线清单见 [小程序 README](../miniprogram/README.md#上线清单)） |
| M7 | **厂家后台**：设备从出厂到报废的台账、退货翻新、二手转让、挂失（设计见 [factory.md](factory.md)） | S21 A1 S22 | ✅ |
| 之后 | 真实硬件、皮套系列、声纹认主、B 端车队版 | — | — |

M2 + M3 即 AGENTS.md 规定的演示主线。W7（文案）不阻塞主线，但演示前应完成。

## 服务端

| 编号 | 任务 | 依赖 | 验收 | 状态 |
| --- | --- | --- | --- | --- |
| S1 | `store`：SQLite、迁移、各表、仓储接口与假实现；`id` 包 | — | 增删改查有测试；业务模块换假实现不改代码 | ✅ |
| S2 | **接入重构**：新建 `app`，`main.go` 只做组装；协议 `hello` 增加 `skin`、`device_id` 限定 MAC；`bridge/linx` 改为 `Open / Close / Forward / Interrupt / Say / OnAIState`，经 `DeviceLink` 收发，删除 `PseudoMAC` 与 `Config.AgentID`，`hello.agent_params` 注入名字，断线重连 | M1 | `app` 按[分发表](modules/app.md#上行分发)转发消息；假灵矽下 `Say` 不经 LLM 直出语音；灵矽断线后自动重连，设备断开后无 goroutine 泄漏 | ✅ |
| S3 | `relation`：状态机、`proximity` 防抖、`AreFriends` 缓存、`FilterFriends`；协议新增 `friend.state` | S1 | 双方靠近 1.5 秒生成 pending；确认后 `AreFriends` 为真；解除立即为假并发事件 | ✅ |
| S4 | `channel` + `zone`：区域与群、4 位码、前台频道、滞回；成员用 `ParticipantID`；协议新增 `channel.state` | S1 | 入区发 `Entered`，`JoinCurrent` 后进频道；码过期失效；错码限流；`presence` 里没有非好友 | ✅ |
| S5 | **`voice`：上行路由、发言权、下行转发**；经 `Sink` 收发；协议 `sound` 增加 `busy` | S2、S3、S4 | 见下方"语音验收" | ✅ |
| S6 | `linxapi` + `device`：引导智能体与风格目录、`EnsureAll`；新设备自动绑定引导智能体；`Handoff` 按 `skin` 交棒到风格智能体并写记忆焦点；换皮套切换智能体；`stage` 为 fresh / named / ready | S1、S2 | 新 MAC 上线后自动绑定引导智能体并能对话；同一 MAC 重复上线不重复开通；`Handoff` 后设备绑定对应风格智能体，重复调用无副作用；`stage=named` 上线时重试交棒而不重新起名；换 `skin` 后切换到新风格；开通失败时设备收到 `error` 且仍可进频道 | ✅ |
| S7 | `bridge/mcp` 代理核心 + `self.profile.set_names`、`self.zone.join`；协议新增 `mcp`；`app` 处理 `Named` 事件：等 AI 说完 → `Handoff` → 重连会话 → 新智能体问候 | S2、S6 | 假灵矽发 `tools/list` 得到合并后的列表（含设备分页）；`tools/call` 按名字路由；说出名字后档案 `stage=named`，AI 说完后自动交棒、会话换成风格智能体并用孩子起的名字问候；交棒失败时下次上线重试 | ✅ |
| S8 | 群聊与好友业务工具：`self.group.*`、`self.friends.*` | S4、S7 | "创建群聊"拿到 4 位码并播报；"加入群聊 XXXX"进群；"下一个群"切换 | ✅ |
| S9 | `history` + `share` + `self.share.publish` | S3、S4、S7 | 今日遇到的小朋友、分享可查；分享后好友与本人都听到描述，非好友听不到 | ✅ |
| S10 | `parentapi` | S3、S9 | 接口有测试；未授权与跨家长访问被拒绝 | ✅ |
| S11 | `world`，仅演示模式注册 | S4、S5 | 地图实时看到位置、谁在说话、谁被屏蔽；默认启动时 `/world` 不可访问 | ✅ |
| S12 | **引导期声音登记**（可选）：`self.voiceprint.enroll`、`device.EnrollVoice`、`linxapi` 声纹登记；`voice_consent` 与 `speaker_id` 入档案 | S6、S7，且先过[待验证](linx-integration.md#待验证)中的样本来源 | 无监护人同意时调用被拒绝且不影响交棒；有同意时登记成功并记下 `speaker_id`；样本取不到时跳过、起名与交棒照常；日志与库里无音频和声纹特征 | ✅ 引导智能体存音频已获产品允许（见[产品决定](linx-integration.md#产品决定)） |

| S13 | **设备认证**：协议 `hello.pubkey`、`hello.challenge` / `hello.proof`；`device.Authenticator`（open / manifest / off）；`device_key` 只存公钥；认证后新连接顶掉旧连接 | S2 | 伪造 MAC 用别的钥匙被拒；签名错、超时、非 proof 都被拒且不登记会话；顶号时旧会话清理完才登记新会话；Web 模拟器用不可导出私钥通过握手 | ✅ |
| S14 | **家长先绑定**：未绑定设备只收 `bind.code`、上行全部丢弃；`POST /api/bindings` 须同意 AI 陪伴，声纹单独勾选；错码限流；`DELETE /api/kids/{id}` 解绑即恢复出厂 | S13、S10 | 绑定前不进 AI、频道、好友、位置；码一次有效、10 分钟过期、掉线作废；已绑定的设备不能被别人认领；解绑后好友立即听不到、设备重连重新播报绑定码 | ✅ |
| S15 | **交友两关**：asking（两个孩子都说"好"，`self.friends.accept/decline`）→ pending（两家各自放行）→ confirmed；家长端 `friend_policy` 与 `waiting_for` | S3、S7 | 一个孩子说好不够；一家放行不够；设为"自己交友"的一侧自动放行；拒绝后 24 小时不再问；询问 60 秒、请求 7 天过期 | ✅ |
| S16 | **小伙伴进频道**：`agt:<mac>` 参与者；`self.channel.speak` 点名后下一段 AI 语音播给好友（`speaker.state{source: agent}`）；家长端 `agent_in_channel` 与屏蔽某位朋友的小伙伴 | S5、S15 | 没点名不外放；陌生人、被屏蔽的听者听不到；和设备共用发言权；主人开口小伙伴让路；家长关闭后工具返回错误结果 | ✅ |
| S18 | **微信登录与家长账号**：`wechat`（code2Session）、`account`（openid → 家长、不透明会话令牌只存哈希、30 天）、`POST/DELETE /api/session`、`GET/PUT/DELETE /api/me`（注销先逐台解绑）；`Kid.waiting_you`；`parentapi.Auth` 带 `ctx`；配置 `wechat.*` | S10、S14 | 同一 openid 并发首次登录只建一个账号；退出、过期、注销后令牌立即失效；注销后名下设备恢复出厂、好友解除；只配微信时演示令牌被拒；错误信息里没有 AppSecret | ✅ |
| S19 | 交友请求的**订阅消息**提醒：`wechat` 取 `access_token` 与发送接口；家长在小程序同意一次性订阅后，`relation` 进入 pending 且等这位家长时发一条 | S18 | 未订阅不发；同一请求只发一次；发送失败不影响交友流程 | ⬜ |
| S20 | **家长端开关声纹**（声纹 V2）：`linxapi` 加 `BoundSpeakers / DeleteVoiceprint / DeleteSpeaker`；`device.GrantVoice`（只在起名前）/ `RevokeVoice`（先解绑再删）；settings 增加 `voice_consent` 等；解绑与注销时删除声纹 | S12、S18 | 关闭后灵矽侧说话人被删、同智能体上别人的说话人不受影响；灵矽失败时档案与其余设置不变（503）；起名后补开 409；注销账号后说话人被删 | ✅ |
| S21 | **厂家后台服务端**：`factory`（状态机、事件日志、批次导入、人员与角色）、`factorydb`（独立 SQLite，事件表触发器只追加）、`adminapi`、`adminapp`、`palpclient`（`Platform` 的 HTTP 实现）、`cmd/palpable-admin` | — | 全部动作与角色有测试；擦除失败时状态不变；批次整批成功或整批回滚；未授权与越权被拒绝；不返回公钥与 `owner_ref` | ✅ |
| S22 | **palpabled 一侧的对接**：`opsapi`（`/internal/devices/{mac}/key` 登记/吊销、`/wipe` 恢复出厂、`/lock` 锁定）、`device.Registry`（公钥与封锁，吊销永久）、网关准入、`adminhook`（绑定/解绑推送，`owner_ref` 为家长标识的 HMAC）；配置 `ops.*` | S21 | 与后台的 `palpclient` 有契约测试；擦除后档案、好友、历史都没了且重复调用无副作用；锁定后在线连接断开、新连接被拒，解锁恢复；吊销后不能再登记、不能解锁；推送失败有退避重试且不阻塞绑定；后台下的擦除不回声；弱令牌拒绝启动 | ✅ |
| S23 | 台账与线上**自动对账**：后台定期核对 palpabled 的绑定状态，补上丢失的绑定/解绑事件；批量文件导入出厂清单 | S22 | 事件丢失后台账能自动纠正；对账结果进异常列表 | ⬜ |
| S17 | 合规补齐：连续对话 2 小时提醒；孩子主动解除好友的业务工具；出厂公钥清单导入；极端情境联系监护人 | S15 | 见 [trust.md §7](trust.md#7-还没做的) | ⬜ |

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
| W1 | `scenes/unbox` 开箱场景 | — | 点击盒子打开，取出玩偶，放到桌面，进入桌面场景 | ✅ |
| W2 | `device-sim`：设备端 VAD 与 `mic.state`、`wake`；多来源下行音频；`busy`、`friend.state`、`channel.state` 的响应 | S3、S4、S5 的协议部分 | 说话时发 `mic.state`；好友说话时播放；占线时玩偶抖动并提示 | ✅ |
| W3 | `scenes/desk` + `plush` 动画（Web 模拟器生成并保存 MAC、`hello` 带 `skin` 已随 S2 完成） | S2 | 灯带随 listening / thinking / speaking / peer_talking 变化；起名对话走通 | ✅ |
| W4 | `device-sim` 设备端 MCP：`self.light.set`、`self.haptic.pulse`、`self.camera.take_photo` | S7 | AI 调用灯光工具玩偶变色；拍照工具返回场景截图的描述 | ✅ |
| W5 | `scenes/park` + `map`：多设备、位置与 `proximity` 上报 | S4、S11 | 两个窗口进入公园，地图同步；靠近触发好友；非好友标注"已屏蔽" | ✅ |
| W6 | `parent-sim` 手机窗口 | S10 | 能看好友、确认好友、看今日回顾 | ✅ |
| W7 | `ui` 儿童文案与提示音 | — | 文案走统一文案表，无诱导留下个人信息的措辞 | ✅ |
| P1 | **家长端小程序**（`miniprogram/`）：原生 + TypeScript + TDesign，绘本画风主题；首页、绑定、孩子详情（今天 / 朋友 / 设置，含声音登记开关）、我的、法律文本；隐私同意先于登录；401 静默重登 | S18、S20 | `pnpm check`（tsc + WXML/JSON 检查）通过；开发者工具构建 npm 后能运行；页面文字全部来自文案表；同意前不发任何请求 | ✅（真机走查待做） |
| P2 | 小程序订阅交友提醒、真机走查与提审 | S19 | 见 [上线清单](../miniprogram/README.md#上线清单) | ⬜ |
| A1 | **厂家后台页面**（`admin/`）：React + shadcn/ui；登录、概览与异常核对、设备台账（筛选、批量动作）、设备详情与履历、批次导入、渠道、人员 | S21 | `pnpm build`、`pnpm lint` 通过；用真实后端走通登记售出、批量发货、挂失；手机宽度无横向溢出 | ✅ |
| W8 | 官网首页：湖畔公园主视觉、四只毛绒伙伴插画、功能带与产品介绍 | — | 320–1536px 布局无横向溢出；导航与移动端菜单可用；保留演示和家长端入口；首页不连接设备或申请麦克风；前端构建、lint 与浏览器无障碍自动检查通过 | ✅ |

## 固件

硬件复用与构建方式见 [固件说明](../firmware/README.md)。

| 编号 | 任务 | 验收 | 状态 |
| --- | --- | --- | --- |
| F1 | 小智硬件层接入可感协议；Schema 生成校验、VAD 与原始 Opus、音源切换、灯光/振动、MCP 与传感器上行接口 | 主机契约测试覆盖合法/非法样例、VAD 顺序、采样率切换、重连清理；真实 Go 网关接受 C++ 客户端握手及语音上行 | ✅ |
| F2 | ESP32-S3 参考板编译 | ESP-IDF 6.0.1+ 完整编译，生成可感固件和烧录参数 | ✅ |
| F3 | 实际板型与 GPS / BLE / NFC 驱动 | 选择实际板型与引脚；真实位置和完整靠近扫描上报，未接硬件时不声明能力 | 🚧 WGH-C2 板型已接入，传感器待选型 |
| F4 | 双设备实机联调 | 配网与掉线恢复、AI 起名交棒、本地唤醒、VAD 校准、提示灯/振动、入区与贴近交友、好友互听及陌生设备屏蔽 | ⬜ 待硬件 |
| F5 | 固件实现认证握手（`hello.pubkey` / `hello.proof`）、私钥入 NVS 并开启 Flash 加密；播报 `bind.code`；`speaker.state{source: agent}` 按 AI 下行参数解码 | 主机契约测试覆盖握手；冒烟测试去掉 `auth.enrollment: off` | ⬜ |

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
