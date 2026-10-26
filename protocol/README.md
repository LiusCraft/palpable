# 可感设备协议

`palpable.schema.json` 是协议的单一事实来源。

- 传输：WebSocket，端点 `/ws`。文本帧为 JSON，二进制帧为 Opus 音频（16kHz、单声道、60ms 一帧）。
- 握手：设备连接后 10 秒内必须发送 `hello`；服务端回 `hello.ack`。同一 `device_id` 同时只允许一条连接。
- 身份：`hello.device_id` 是设备 MAC（小写、冒号分隔），也是设备在灵矽侧的 `Device-Id`；可选的 `skin` 是皮套款式，服务端据此选风格智能体。
- 上行类型：`hello`、`wake`、`mic.state`、`proximity`、`location`、`mcp`。
- 下行类型：`hello.ack`、`audio.downlink`、`light`、`haptic`、`sound`、`speaker.state`、`presence`、`channel.state`、`friend.state`、`mcp`、`error`。
- `mcp` 双向，`payload` 为 JSON-RPC 2.0；设备在 `capabilities` 里声明 `mcp` 才会收到。
- 音频参数：上行固定 Opus、16kHz、单声道、60ms；下行以 `audio.downlink` 为准（灵矽的 TTS 可能是 24kHz），该消息在 AI 通道就绪后发送，早于任何下行音频帧。设备必须按它配置解码器，收到前按 16kHz / 60ms 处理。
- 未知字段、未知类型、超出范围的值一律返回 `error`（`bad_message`），连接保持。

## 如何保持一致

- 前端类型由 schema 生成：`make gen`，生成物在 `web/src/shared/protocol.gen.ts`，不要手改。
- Go 类型手写在 `server/internal/protocol`，由 `protocol_test.go` 对照 schema 校验：`examples/` 下的合法样例必须被 Go 解码，`invalid-` 开头的样例必须被拒绝，Go 编码出的下行消息必须通过 schema。
- 新增或修改消息：先改 schema 与 `examples/`，再改 Go，最后 `make check`。

## 给固件的说明

固件只需实现设备端：发送上行消息和麦克风音频帧，接收下行消息和扬声器音频帧。业务规则（好友、频道、屏蔽、认主）全在服务端，设备端不做判断。能力清单通过 `hello.capabilities` 声明，没有的能力服务端不会下发对应消息。

ESP32 的构建、硬件适配与验证方法见 [固件说明](../firmware/README.md)。固件的消息校验器也由本 schema 生成，协议改动后执行 `make firmware-test` 验证。
