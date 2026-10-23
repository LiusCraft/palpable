// Package config 从 YAML 文件加载服务端配置。密钥只允许出现在这个文件里，文件本身不入库。
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Addr string `yaml:"addr"`
	Linx Linx   `yaml:"linx"`
}

// Linx 是灵矽接入配置；WSURL 为空表示不启用 AI 对话。
type Linx struct {
	WSURL          string `yaml:"ws_url"`          // 例如 wss://xrobo-io.qiniuapi.com/v1/ws/
	Token          string `yaml:"token"`           // Bearer 令牌
	AgentID        string `yaml:"agent_id"`        // 可选，经 X-Agent-ID 指定智能体
	VoiceprintBase string `yaml:"voiceprint_base"` // 声纹服务地址（演示环境）
}

func (l Linx) Enabled() bool { return l.WSURL != "" }

// Load 读取并解析 path。文件不存在时返回默认配置，便于本地不接灵矽直接跑；
// 其他错误（含未知字段）一律上抛，避免拼错的键被静默忽略。
func Load(path string) (Config, error) {
	cfg := Config{Addr: ":8080"}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}
