# protocol

基础层 · `server/internal/protocol` · 无依赖

## 边界

**职责**：设备协议的 Go 类型；解码并校验上行消息，编码下行消息。以 `protocol/palpable.schema.json` 为单一事实来源，Go 类型手写，TS 类型由 schema 生成。已生效的协议全文见 [protocol/README.md](../../protocol/README.md)。

**不负责**：路由与业务判断。

## 接口

```go
func DecodeUpstream(data []byte) (Message, error)   // 未知类型 / 未知字段 / 越界值 → ErrBadMessage
func Encode(msg Message) ([]byte, error)
func DefaultAudioParams() AudioParams               // Opus, 16kHz, 单声道, 60ms

type Message interface{ MsgType() string }
```

## 消息一览

| 方向 | 类型 | 关键字段 | 取值 / 校验 |
| --- | --- | --- | --- |
| 上行 | `hello` | `device_id`, `skin?`, `firmware?`, `audio_params`, `capabilities` | `device_id` 为小写冒号分隔 MAC；`skin` ∈ `^[a-z0-9_-]{1,32}$`；`audio_params` 必须等于默认；能力 ∈ mic / speaker / light / haptic / proximity / location / wake / mcp |
| | `wake` | `source` | voice \| touch |
| | `mic.state` | `state` | start \| stop |
| | `proximity` | `peers[{device_id, strength}]` | ≤ 16 个；`strength` ∈ [0, 1]；完整快照 |
| | `location` | `lat`, `lng` | 经纬度范围 |
| | `mcp` | `payload` | JSON-RPC 对象；双向 |
| 下行 | `hello.ack` | `session_id`, `audio_params` | — |
| | `audio.downlink` | `sample_rate`, `frame_duration` | 16000 \| 24000；20 \| 40 \| 60；早于任何下行音频帧 |
| | `light` | `state`, `color?`, `pattern?` | state ∈ idle / listening / thinking / speaking / peer_talking / muted；pattern ∈ solid / breathe / pulse |
| | `haptic` | `pattern` | tap \| double_tap \| long |
| | `sound` | `name` | connected / joined / left / switched / error / busy / friend |
| | `speaker.state` | `state`, `source`, `from?`, `name?` | start \| stop；ai \| peer |
| | `presence` | `event`, `device_id`, `name?`, `channel` | join \| leave；仅含好友 |
| | `channel.state` | `event`, `channel`, `name`, `color?` | joined / switched / left |
| | `friend.state` | `event`, `device_id`, `name?` | pending / confirmed / removed |
| | `mcp` | `payload` | 设备声明 `mcp` 能力才会收到 |
| | `error` | `code`, `message` | bad_message / unsupported / unauthorized / internal |

二进制帧：Opus 音频。上行固定 16kHz 单声道 60ms；下行以 `audio.downlink` 为准。

## 校验规则

```text
DecodeUpstream(data):
    head.type 不在上行集合内        → ErrBadMessage
    严格解码（DisallowUnknownFields）→ 未知字段 ErrBadMessage
    validate(msg)                   → 上表的取值与范围，越界 ErrBadMessage
```

## 变更流程与计划中的增量

新增或修改消息：先改 schema 与 `protocol/examples/`，再改 Go，再 `make gen`，最后 `make check`。

计划中的增量（落地时先改 schema，再从本表删除）：

| 方向 | 消息 | 变更 | 用途 |
| --- | --- | --- | --- |
| 下行 | `speaker.state` | 可选 `from_kind`（device \| agent） | 预留：智能体入频道时才加 |
