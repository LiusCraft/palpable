package linx

import (
	"encoding/json"
	"fmt"
	"os"
)

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

// ConfigFromEnv 读取 LINX_WS_URL 与 LINX_TOKEN；URL 为空表示未配置灵矽。
func ConfigFromEnv() (Config, bool) {
	cfg := Config{URL: os.Getenv("LINX_WS_URL"), Token: os.Getenv("LINX_TOKEN")}
	return cfg, cfg.URL != ""
}
