// Package id 放跨模块共用的标识类型，避免模块之间为了一个类型互相 import。
package id

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// DeviceID 是设备 MAC，与 hello.device_id 一致。
type DeviceID string

// ChannelID 形如 "zone:park"、"group:<id>"。
type ChannelID string

type GuardianID string

// ParticipantID 是频道成员：设备为 "dev:<mac>"；"agt:<mac>" 预留给入频道的智能体。
type ParticipantID string

type Kind int

const (
	KindUnknown Kind = iota
	KindDevice
	KindAgent // 预留
)

const (
	devicePrefix = "dev:"
	agentPrefix  = "agt:"
)

func (p ParticipantID) Kind() Kind {
	switch {
	case strings.HasPrefix(string(p), devicePrefix):
		return KindDevice
	case strings.HasPrefix(string(p), agentPrefix):
		return KindAgent
	}
	return KindUnknown
}

func Device(d DeviceID) ParticipantID { return ParticipantID(devicePrefix + string(d)) }

// OwnerDevice 返回参与者所属的设备；好友判断与家长控制都以它为准，
// 所以智能体参与者也归到它的设备。无法识别时返回空串。
func OwnerDevice(p ParticipantID) DeviceID {
	switch p.Kind() {
	case KindDevice:
		return DeviceID(strings.TrimPrefix(string(p), devicePrefix))
	case KindAgent:
		return DeviceID(strings.TrimPrefix(string(p), agentPrefix))
	}
	return ""
}

// New 生成随机记录 ID（好友请求、群、历史事件等），16 位十六进制。
func New() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
