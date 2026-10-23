# AGENTS.md

可感（Palpable）：面向儿童的 AI 语音陪伴对讲设备。当前阶段设备由 Web 页面模拟，后续落地真实硬件。

## 第一原则：设备端只做"传感器和执行器"

Web 模拟器与未来固件是**同一个协议的两个客户端**。所有业务规则（谁能听见谁、好友关系、频道、认主、家长控制）MUST 放在服务端；设备端 MUST NOT 含业务判断。

- 新增设备行为时，先改 `protocol/`，再改服务端，最后改 Web 模拟器。禁止 Web 端私自约定协议外的消息。
- Web 模拟器里禁止直接读取其他"设备"的状态，所有跨设备信息必须来自服务端下行消息。
- 灵矽（xrobotd/linxd）的 token、API Key 只允许存在服务端环境变量，MUST NOT 出现在 `web/` 或提交到仓库。

## 技术栈

| 层 | 选型 |
| --- | --- |
| 后端 | Go 1.26，标准库 `net/http`，`gorilla/websocket`，`log/slog`；存储先用 SQLite（`modernc.org/sqlite`，免 CGO），接口隔离便于换库 |
| 前端 | React 19 + TypeScript（strict）+ Vite；3D 用 `three` + `@react-three/fiber` + `@react-three/drei`；状态用 `zustand`；样式用 Tailwind CSS；包管理 pnpm |
| 协议 | JSON Schema 为单一事实来源，放 `protocol/`；TS 类型由其生成（`make gen`），Go 类型手写并由契约测试对照 schema 校验 |
| 音频 | 与灵矽对齐：Opus、16kHz、单声道、60ms 帧，WebSocket 二进制帧 |

## 目录结构

```
palpable/
├── protocol/          设备协议 schema 与文档（单一事实来源）
├── server/            Go 后端（架构见 docs/architecture.md，模块规格见 docs/modules.md）
│   ├── cmd/palpabled/ 入口
│   └── internal/      gateway / voice / bridge / channel / relation / zone / device / history / share / parentapi / world / store
├── web/               前端
│   └── src/           device-sim / scenes / map / ui / shared
├── firmware/          预留：未来硬件固件，只实现 protocol/ 的设备端
└── docs/              架构、产品、演示脚本
```

## 编码规则

通用
- 提交信息用中文或英文均可，一次提交只做一件事；不得提交 `.env`、密钥、声纹原始音频。
- 注释只写"为什么"，不复述代码。命名跟随所在目录的现有风格。
- 每个模块对外只暴露接口；跨模块调用走接口，不直接读对方内部状态。

Go
- 每次修改后执行 `go build ./... && go vet ./... && go test ./...`，全部通过才算完成。
- 错误向上返回并带上下文（`fmt.Errorf("...: %w", err)`）；禁止吞错误。
- 每个 WebSocket 连接的生命周期由 `context` 控制，断开后 MUST 释放所有 goroutine 与订阅，避免泄漏。
- 音频帧热路径禁止加锁等待和日志打印；日志只记录元数据，MUST NOT 记录音频内容、声纹向量、儿童的真实姓名之外的敏感字段。
- 外部输入（WS 消息、HTTP 请求）一律校验，非法消息返回协议错误而不是 panic。

TypeScript / React
- `strict` 打开，禁止 `any`；协议类型只从生成的 `protocol` 包导入。
- 3D 组件只负责渲染，状态放 store；动画由"设备状态"驱动，不在组件里写业务逻辑。
- 新增依赖前先确认必要性，优先用 `drei` 已有能力。

## 儿童隐私与安全（硬约束）

- 陌生设备之间默认互相屏蔽：未建立好友关系的设备，音频 MUST NOT 被转发，也不应出现在对方的在线列表中。
- 声纹只存特征与绑定关系，不长期保留原始音频；删除设备时同时删除声纹。
- 面向儿童的所有文案、AI 提示词必须使用儿童友好的语气，禁止诱导留下个人信息（地址、学校、电话）。
- 家长控制（好友确认、录音与摘要查看）是产品功能，不得因为演示方便而绕开服务端规则。

## 演示约束

- 演示只含"开箱 → 命名认主 → 公园区域频道 → 贴近成为好友 → 好友互相听见"这条主线，其余功能留在路线图。
- 演示环境的灵矽账号与地址通过环境变量注入：`LINX_WS_URL`、`LINX_TOKEN`、`LINX_VOICEPRINT_BASE`。
