# 可感 Palpable 架构与模块拆解

## 1. 总体结构

```
 Web 模拟设备 ┐                                     ┌─ 灵矽（ASR / LLM / TTS / 声纹）
 Web 模拟设备 ┼── Palpable Device Protocol (WS) ──▶ palpabled ─┤
 真实硬件(未来)┘        一个设备 = 一条连接            │  └─ 存储（SQLite）
                                                     └─ 频道转发：好友之间的 Opus 音频直连转发
```

关键决策：

1. **设备只连我们的网关，不直连灵矽。** 网关为每台设备维护一条到灵矽的 WS 会话（桥接），灵矽凭证只存在服务端。真实硬件落地时，固件只需实现 Palpable 设备协议，无需关心灵矽。
2. **AI 对话和人与人对讲走两条通路。** 对 AI 说话：设备音频 → 网关 → 灵矽。对朋友说话：设备音频 → 网关 → 频道 → 朋友设备。由服务端根据设备当前模式决定走哪条。
3. **设备能力抽象为 HAL（硬件抽象层）：** 麦克风、扬声器、灯带、振动、近场感知、定位、唤醒。Web 模拟器与固件各自实现这七项，其余全在服务端。
4. **灵矽智能体就是玩偶小伙伴的"大脑"。** 记忆、听说、声纹识别、看图片都由灵矽提供；可感通过云端 MCP 服务给它配上建群、切群等业务工具，设备端通过 MCP 向它暴露灯光、振动、拍照等工具。详见 [模块拆解](modules.md) 第 2 节。

## 2. 设备抽象（为硬件落地做准备）

| 能力 | 方向 | Web 模拟 | 未来硬件 |
| --- | --- | --- | --- |
| `mic` 上行音频流 | 设备→服务端 | WebAudio + Opus 编码 | I2S 麦克风 + Opus |
| `speaker` 下行音频流 | 服务端→设备 | WebAudio 解码播放 | I2S 功放 |
| `light` 灯带状态 | 服务端→设备 | 3D 发光材质动画 | LED 灯带 |
| `haptic` 振动 | 服务端→设备 | 玩偶抖动动画 + 提示音 | 振动马达 |
| `proximity` 近场发现 | 设备→服务端 | 由地图中的距离生成 | BLE/NFC 扫描 |
| `location` 定位 | 设备→服务端 | 地图中的坐标 | GPS/模拟 |
| `wake` 唤醒事件 | 设备→服务端 | 点击玩偶或本地语音唤醒 | 本地唤醒词引擎 |

不属于设备的事情：好友关系、频道、屏蔽、认主、AI 人设、游戏逻辑、家长控制。

## 3. 设备协议（`protocol/`，草案）

文本帧为 JSON，音频为二进制 Opus 帧；所有文本帧带 `type`。

上行（设备→服务端）
- `hello`：`device_id`、`firmware`、`audio_params`、`capabilities`
- `wake`：唤醒事件，附 `source`（voice / touch）
- `mic.state`：`start` / `stop`（免按键时由设备端 VAD 决定）
- `proximity`：`peers: [{device_id, strength}]`
- `location`：`lat`、`lng`
- 二进制：麦克风 Opus 帧

下行（服务端→设备）
- `hello.ack`：`session_id`、`audio_params`
- `light`：`state`（idle / listening / thinking / speaking / peer_talking / muted）、`color`、`pattern`
- `haptic`：`pattern`
- `sound`：`name`（提示音标识，设备本地播放）
- `speaker.state`：`start` / `stop`，附 `source`（ai / peer）和 `from`
- `presence`：频道成员增减事件，供提示"小兔也来公园啦"
- `error`：协议错误
- 二进制：扬声器 Opus 帧

## 4. 服务端模块（`server/internal/`）

> 更细的拆分、接口约定、数据模型与任务包见 [模块拆解与任务包](modules.md)，下表为概览。

| 模块 | 职责 | 依赖 |
| --- | --- | --- |
| `protocol` | 协议类型、编解码、校验（由 schema 生成） | — |
| `gateway` | 接受设备 WS，鉴权，维护设备会话，路由消息；每台设备一个 `Session` | protocol |
| `bridge/linx` | 把设备的 AI 对话转给灵矽：建立灵矽 WS、透传音频、转换 `stt/llm/tts` 为设备下行消息；调用灵矽声纹 REST | gateway |
| `identity` | 设备命名、认主（声纹绑定）、"谁是主人"判定 | bridge/linx、store |
| `relation` | 好友关系：贴近配对、确认、解除；对外提供 `AreFriends(a, b)` | store |
| `zone` | 坐标 → 区块 → 频道映射（演示只有公园一个区块） | — |
| `channel` | 频道成员管理与音频转发；转发前必须调用 `relation.AreFriends` 过滤；产生 `presence` 事件 | relation、zone |
| `guardian` | 家长控制：好友确认、查看摘要、屏蔽规则（演示阶段先留接口与最小实现） | relation、store |
| `world` | 演示用世界状态：地图上有哪些设备、位置、谁在说话，推给前端地图（不属于设备协议，走单独的观察 WS） | gateway、channel |
| `store` | 存储接口与 SQLite 实现 | — |

核心规则：`channel` 是唯一决定"谁能听见谁"的地方。`gateway` 与 `bridge` 不得绕过它转发音频。

## 5. 前端模块（`web/src/`）

| 模块 | 职责 |
| --- | --- |
| `device-sim` | 虚拟设备：实现七项 HAL，连接网关，只暴露"设备状态"给渲染层；内部无业务规则 |
| `scenes/unbox` | 开箱场景：包装盒、打开、取出玩偶、放到桌面 |
| `scenes/desk` | 桌面场景：与 AI 一对一对话、命名、认主引导 |
| `scenes/park` | 公园场景：多个小朋友和玩偶，走动，靠近，对话气泡 |
| `map` | 地图与区块：展示设备位置、频道范围、谁在说话（订阅 `world`） |
| `plush` | 玩偶 3D 模型与皮套材质、灯带动画（由 `light` 状态驱动） |
| `ui` | 儿童友好的文案和提示、提示音、字幕 |
| `shared` | 协议类型（生成）、WS 客户端、store |

多设备演示方式：同一页面可开多个"小朋友窗口"，每个窗口是一个独立的 `device-sim` 实例，也可以多个浏览器分别接入。

## 6. 演示主线与模块对应

| 步骤 | 触发 | 参与模块 |
| --- | --- | --- |
| 开箱 | 点击盒子 | `scenes/unbox` |
| 首次上线问候 | `hello` 首次出现新设备 | `gateway` → `bridge/linx`（`text_response` 直出 TTS） |
| 命名 | 小朋友说出名字 | `bridge/linx`、`identity` |
| 认主（声纹） | 同上，录一段音频 | `identity` → 灵矽声纹接口 |
| 普通对话陪伴 | 唤醒后说话 | `bridge/linx` |
| 进入公园 | 场景切换，`location` 上报 | `zone`、`channel` |
| 陌生人屏蔽 | 同频道内非好友 | `channel` + `relation` |
| 贴近成为好友 | 地图距离触发 `proximity` | `relation`、`guardian` |
| 好友互相听见 | 好友说话 | `channel` 转发 Opus，`presence` 提示 |

## 7. 里程碑

1. **M0 骨架：** 仓库、协议 schema、Go 与前端工程、CI 命令。
2. **M1 一对一：** 设备连网关，经桥接与灵矽对话，玩偶灯带随状态变化。
3. **M2 开箱与认主：** 开箱场景、命名、声纹绑定。
4. **M3 公园频道：** `zone`、`channel`、`relation`，两个浏览器互相听见，陌生人屏蔽。
5. **M4 打磨：** 地图、提示音、儿童文案、家长控制最小版、演示脚本。

## 8. 待验证事项

- 灵矽 WebSocket 鉴权：浏览器不能自定义请求头，已决定由网关代连灵矽，不依赖浏览器直连；网关到灵矽的鉴权方式需读 `linxdv0/internal/server/auth.go` 确认。
- 浏览器 Opus 编码：优先用 WebCodecs `AudioEncoder`，不支持时用 WASM 编码库。
- 声纹接口需要 `device_id` 已在灵矽登记，网关需要在设备首次上线时完成登记。
- 好友之间的转发延迟目标：同机房小于 300ms，演示需要实测。
