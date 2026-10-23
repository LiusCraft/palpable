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
	Addr   string `yaml:"addr"`
	DBPath string `yaml:"db_path"` // SQLite 文件
	Linx   Linx   `yaml:"linx"`
	Demo   Demo   `yaml:"demo"`
}

// Demo 是演示环境的预置：家长与同意标记在产品里由家长端绑定时取得。
type Demo struct {
	Guardian     string `yaml:"guardian"`      // 新设备自动绑定到这个家长 ID；空则不绑定
	GuardianName string `yaml:"guardian_name"` // 家长显示名
	VoiceConsent bool   `yaml:"voice_consent"` // 预置监护人对声纹登记的同意
}

// Linx 是灵矽接入配置；WSURL 为空表示不启用 AI 对话。
type Linx struct {
	WSURL          string `yaml:"ws_url"`          // 例如 wss://xrobo-io.qiniuapi.com/v1/ws/
	Token          string `yaml:"token"`           // Bearer 令牌，WebSocket 与 REST 共用
	APIBase        string `yaml:"api_base"`        // REST 地址，默认 https://xrobo.qiniu.com
	OTAURL         string `yaml:"ota_url"`         // OTA 地址，激活预注册的设备；默认 https://xrobo.qiniu.com/xiaozhi/ota/
	VoiceprintBase string `yaml:"voiceprint_base"` // 声纹服务地址（演示环境）
}

func (l Linx) Enabled() bool { return l.WSURL != "" }

// Load 读取并解析 path。文件不存在时返回默认配置，便于本地不接灵矽直接跑；
// 其他错误（含未知字段）一律上抛，避免拼错的键被静默忽略。
func Load(path string) (Config, error) {
	cfg := Config{Addr: ":8080", DBPath: "palpable.db"}
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
