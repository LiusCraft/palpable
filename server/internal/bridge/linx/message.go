package linx

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
)

// welcome 是灵矽对 hello 的回复。
type welcome struct {
	Type        string `json:"type"`
	SessionID   string `json:"session_id"`
	AudioParams struct {
		SampleRate    int `json:"sample_rate"`
		FrameDuration int `json:"frame_duration"`
	} `json:"audio_params"`
}

// normalize 在灵矽没返回音频参数时回落到协议默认值，避免设备拿到 0。
func (w *welcome) normalize() {
	if w.AudioParams.SampleRate == 0 {
		w.AudioParams.SampleRate = 16000
	}
	if w.AudioParams.FrameDuration == 0 {
		w.AudioParams.FrameDuration = 60
	}
}

// PseudoMAC 由设备 ID 稳定地生成灵矽要求的 MAC 形式 Device-Id。
// 最高字节固定为"本地管理的单播地址"（第二位为 1、最低位为 0），不会与真实厂商 MAC 冲突。
// 生成后需要在灵矽侧预注册并关联智能体。
func PseudoMAC(deviceID string) string {
	sum := sha256.Sum256([]byte("palpable:" + deviceID))
	b := sum[:6]
	b[0] = (b[0] | 0x02) &^ 0x01
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
}

// newUUID 生成 v4 UUID，作为每条连接的 Client-Id。
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// linxMsg 是灵矽下行文本消息中我们关心的字段。
type linxMsg struct {
	Type  string `json:"type"`
	State string `json:"state"`
	Text  string `json:"text"`
}

func parseLinx(data []byte) (linxMsg, error) {
	var m linxMsg
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse linx message: %w", err)
	}
	return m, nil
}

// ConfigFromEnv 读取 LINX_WS_URL、LINX_TOKEN、可选的 LINX_AGENT_ID；URL 为空表示未配置灵矽。
func ConfigFromEnv() (Config, bool) {
	cfg := Config{
		URL:     os.Getenv("LINX_WS_URL"),
		Token:   os.Getenv("LINX_TOKEN"),
		AgentID: os.Getenv("LINX_AGENT_ID"),
	}
	return cfg, cfg.URL != ""
}
