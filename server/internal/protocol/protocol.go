// Package protocol 定义可感设备协议的 Go 类型。
// 单一事实来源是仓库根目录 protocol/palpable.schema.json，
// protocol_test.go 会用 schema 校验这里编码出的消息，保证两边不漂移。
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/LiusCraft/palpable/server/internal/id"
)

// Message 是所有协议文本消息的公共接口。
type Message interface {
	MsgType() string
}

const (
	TypeHello         = "hello"
	TypeWake          = "wake"
	TypeMicState      = "mic.state"
	TypeProximity     = "proximity"
	TypeLocation      = "location"
	TypeMcp           = "mcp"
	TypeHelloAck      = "hello.ack"
	TypeAudioDownlink = "audio.downlink"
	TypeWakeConfig    = "wake.config"
	TypeLight         = "light"
	TypeHaptic        = "haptic"
	TypeSound         = "sound"
	TypeSpeakerState  = "speaker.state"
	TypePresence      = "presence"
	TypeChannelState  = "channel.state"
	TypeFriendState   = "friend.state"
	TypeError         = "error"
)

// 错误码。
const (
	CodeBadMessage   = "bad_message"
	CodeUnsupported  = "unsupported"
	CodeUnauthorized = "unauthorized"
	CodeInternal     = "internal"
)

// ErrBadMessage 表示收到的消息不符合协议。
var ErrBadMessage = errors.New("bad message")

type AudioParams struct {
	Format        string `json:"format"`
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
	FrameDuration int    `json:"frame_duration"`
}

// DefaultAudioParams 与灵矽对齐：Opus、16kHz、单声道、60ms。
func DefaultAudioParams() AudioParams {
	return AudioParams{Format: "opus", SampleRate: 16000, Channels: 1, FrameDuration: 60}
}

// ---- 上行：设备 → 服务端 ----

type Hello struct {
	Type         string      `json:"type"`
	DeviceID     id.DeviceID `json:"device_id"`
	Skin         string      `json:"skin,omitempty"`
	Firmware     string      `json:"firmware,omitempty"`
	AudioParams  AudioParams `json:"audio_params"`
	Capabilities []string    `json:"capabilities"`
}

type Wake struct {
	Type   string `json:"type"`
	Source string `json:"source"`
}

type MicState struct {
	Type  string `json:"type"`
	State string `json:"state"`
}

type ProximityPeer struct {
	DeviceID string  `json:"device_id"`
	Strength float64 `json:"strength"`
}

type Proximity struct {
	Type  string          `json:"type"`
	Peers []ProximityPeer `json:"peers"`
}

type Location struct {
	Type string  `json:"type"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Mcp 双向：payload 是 JSON-RPC 2.0 消息，服务端只做代理，不改写设备工具的内容。
type Mcp struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ---- 下行：服务端 → 设备 ----

type HelloAck struct {
	Type        string      `json:"type"`
	SessionID   string      `json:"session_id"`
	AudioParams AudioParams `json:"audio_params"`
}

// AudioDownlink 告知设备服务端下行音频的参数；灵矽的 TTS 采样率可能是 24kHz，
// 设备必须按它配置解码器。AI 通道就绪后发送，早于任何下行音频帧。
type AudioDownlink struct {
	Type          string `json:"type"`
	SampleRate    int    `json:"sample_rate"`
	FrameDuration int    `json:"frame_duration"`
}

// WakeConfig 告诉设备本地唤醒词引擎该听什么词；整体替换旧的。
type WakeConfig struct {
	Type  string   `json:"type"`
	Words []string `json:"words"`
}

type Light struct {
	Type    string `json:"type"`
	State   string `json:"state"`
	Color   string `json:"color,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

type Haptic struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern"`
}

type Sound struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type SpeakerState struct {
	Type   string `json:"type"`
	State  string `json:"state"`
	Source string `json:"source"`
	From   string `json:"from,omitempty"`
	Name   string `json:"name,omitempty"`
}

type Presence struct {
	Type     string `json:"type"`
	Event    string `json:"event"`
	DeviceID string `json:"device_id"`
	Name     string `json:"name,omitempty"`
	Channel  string `json:"channel"`
}

type ChannelState struct {
	Type    string `json:"type"`
	Event   string `json:"event"`
	Channel string `json:"channel"`
	Name    string `json:"name"`
	Color   string `json:"color,omitempty"`
}

type FriendState struct {
	Type     string      `json:"type"`
	Event    string      `json:"event"`
	DeviceID id.DeviceID `json:"device_id"`
	Name     string      `json:"name,omitempty"`
}

type Error struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (*Hello) MsgType() string         { return TypeHello }
func (*Wake) MsgType() string          { return TypeWake }
func (*MicState) MsgType() string      { return TypeMicState }
func (*Proximity) MsgType() string     { return TypeProximity }
func (*Location) MsgType() string      { return TypeLocation }
func (*HelloAck) MsgType() string      { return TypeHelloAck }
func (*AudioDownlink) MsgType() string { return TypeAudioDownlink }
func (*WakeConfig) MsgType() string    { return TypeWakeConfig }
func (*Light) MsgType() string         { return TypeLight }
func (*Haptic) MsgType() string        { return TypeHaptic }
func (*Sound) MsgType() string         { return TypeSound }
func (*SpeakerState) MsgType() string  { return TypeSpeakerState }
func (*Presence) MsgType() string      { return TypePresence }
func (*Error) MsgType() string         { return TypeError }
func (*Mcp) MsgType() string           { return TypeMcp }
func (*ChannelState) MsgType() string  { return TypeChannelState }
func (*FriendState) MsgType() string   { return TypeFriendState }

// DecodeUpstream 解析设备上行的文本消息；只接受上行类型，未知或非法一律返回 ErrBadMessage。
func DecodeUpstream(data []byte) (Message, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadMessage, err)
	}

	var msg Message
	switch head.Type {
	case TypeHello:
		msg = &Hello{}
	case TypeWake:
		msg = &Wake{}
	case TypeMicState:
		msg = &MicState{}
	case TypeProximity:
		msg = &Proximity{}
	case TypeLocation:
		msg = &Location{}
	case TypeMcp:
		msg = &Mcp{}
	default:
		return nil, fmt.Errorf("%w: unknown upstream type %q", ErrBadMessage, head.Type)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(msg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadMessage, err)
	}
	if err := validate(msg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadMessage, err)
	}
	return msg, nil
}

// Encode 编码一条下行消息。
func Encode(msg Message) ([]byte, error) {
	return json.Marshal(msg)
}

var (
	macRe  = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
	skinRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
)

func validate(msg Message) error {
	switch m := msg.(type) {
	case *Hello:
		if !macRe.MatchString(string(m.DeviceID)) {
			return errors.New("device_id must be a lowercase colon-separated MAC")
		}
		if m.Skin != "" && !skinRe.MatchString(m.Skin) {
			return errors.New("invalid skin")
		}
		if m.AudioParams != DefaultAudioParams() {
			return errors.New("unsupported audio_params")
		}
	case *Wake:
		if m.Source != "voice" && m.Source != "touch" {
			return errors.New("invalid wake source")
		}
	case *MicState:
		if m.State != "start" && m.State != "stop" {
			return errors.New("invalid mic state")
		}
	case *Proximity:
		if len(m.Peers) > 16 {
			return errors.New("too many peers")
		}
		for _, p := range m.Peers {
			if p.DeviceID == "" || p.Strength < 0 || p.Strength > 1 {
				return errors.New("invalid proximity peer")
			}
		}
	case *Mcp:
		if len(m.Payload) == 0 || m.Payload[0] != '{' {
			return errors.New("mcp payload must be a JSON object")
		}
	case *Location:
		if m.Lat < -90 || m.Lat > 90 || m.Lng < -180 || m.Lng > 180 {
			return errors.New("location out of range")
		}
	}
	return nil
}
