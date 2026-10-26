# web

前端 · `web/src` · React 19 + TypeScript strict + Vite + zustand

## 边界

前端模块分两类，边界不能混：

| 类别 | 模块 | 规则 |
| --- | --- | --- |
| **设备** | `device-sim`、`plush` | 与未来固件是同一份协议的两个客户端；只发协议里的上行消息、只响应下行消息；**不含业务判断**，不得读取其他虚拟设备的状态 |
| **Web 展示** | `site`、`scenes/*`、`map`、`parent-sim`、`ui` | 只存在于演示，不进协议；展示数据来自 `/world`、`/api` 等非设备通道 |

`shared/protocol.gen.ts` 由 schema 生成（`make gen`），不手改；协议类型只从这里导入。

## 模块一览

| 模块 | 职责 |
| --- | --- |
| `shared` | 生成的协议类型 |
| `device-sim` | 虚拟设备（见下） |
| `plush` | 玩偶与灯带 3D 表现；响应 `light`、`speaker.state`、`haptic`，只渲染，不含业务逻辑 |
| `site` | 官网首页（`#/`）；体验入口 `#/play`，家长端 `#/parent`；不连设备、不申请麦克风；3D 按需加载 |
| `scenes/world` | 第一人称控制器与互动点（`Interactor`：站在互动点上按 E） |
| `scenes/room` | 房间：开盒 → 抱出玩偶 → 桌面通电（`activate`）→ 出门转场 |
| `scenes/park` | 公园：别的玩偶按 `/world` 位置出现；位置经传感器上报；小地图只读 |
| `scenes/hud` | 画面提示与演示遥控抽屉（起名兜底、示例功能、换皮套） |
| `map` | 订阅 `/world`，显示位置、谁在说话、「已屏蔽」；`demoTool` 调 `POST /world/tool` |
| `parent-sim` | 家长端页面（`#/parent`），调 `/api`，是另一个客户端 |
| `ui` | 儿童友好的统一文案表（`copy.ts`）与提示音（`sounds.ts`） |

## device-sim

一个页面 = 一台设备 = 一个小朋友；多个小朋友靠多开页面，页面之间只通过服务端互通。

```text
client.ts      认领设备：?device=<mac> > 本浏览器上次用且未被占的 > 新建；用 Web Locks 保证一页一只
device.ts      VirtualDevice：连接、上行消息、下行消息处理；状态存 zustand store（DeviceView）
audio.ts       MicCapture（采集）、FrameEncoder（WebCodecs Opus 编码）、Player（按 audio.downlink 解码播放）
vad.ts         设备端端点检测
mcp.ts         设备端 MCP 工具
environment.ts 物理环境输入：摄像头看到什么、BLE 强度（距离 → 强度）
identity.ts    随机 MAC、皮套
```

### 对照设备抽象

| 能力 | 实现 |
| --- | --- |
| `mic` | `MicCapture` → `Vad` → `FrameEncoder` → WS 二进制帧 |
| `speaker` | `Player` 按 `audio.downlink` 的采样率配置解码器，收到前按 16kHz / 60ms |
| `light` / `haptic` | 写 store，由 `plush` 渲染；`haptic` 同时 `navigator.vibrate` |
| `proximity` | 场景里的距离经 `strengthAt` 换算，`reportProximity` 上报完整快照（≤16 个） |
| `location` | 场景坐标，`reportLocation` 限频 1s |
| `wake` | 点击玩偶发 `wake{source: touch}` |
| 设备端 MCP | `handleMcp` 实现 `self.light.set`、`self.haptic.pulse`、`self.camera.take_photo`（场景描述代替摄像头） |

### 端点检测（VAD）

```text
push(rms):
    未说话: 连续 2 帧（120ms）rms >= 阈值 → 'start'      // 过滤咳嗽、碰撞
    说话中: 连续 10 帧（600ms）静音      → 'stop'          // 句中停顿不切断
说话开始前保留 3 帧 preroll，开始时先补发，保住第一个字
```

`start` → 发 `mic.state start` 并开始编码上行；`stop` → 发 `mic.state stop`。服务端另有看门狗，设备端只要不切碎句子即可。

### 下行处理

| 消息 | 处理 |
| --- | --- |
| `hello.ack` | `link = online` |
| `audio.downlink` | `player.configure(sample_rate)` |
| `light` | 写 `light` |
| `speaker.state start/stop` | 换来源时 `player.flush()`；`peer` 时记录说话的好友与灯色 `peer_talking` |
| `channel.state` / `presence` / `friend.state` | 更新频道、在场好友、好友表并提示 |
| `sound` / `haptic` | 提示音 / 抖动；`busy` 弹占线提示 |
| `mcp` | 交给 `handleMcp`，有回复则回 `mcp` |
| `error` | 记 `lastError` |

### 约束

- 麦克风默认打开，不放「按住说话」按钮（`setPushToTalk` 仅为调试保留）。
- 设备协议外的信息一律不用：好友、在场、频道全部来自下行消息。
- 浏览器音频依赖 WebCodecs，仅 Chromium 系；麦克风需要 `localhost` 或 HTTPS。
