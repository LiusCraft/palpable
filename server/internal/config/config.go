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
	Auth   Auth   `yaml:"auth"`
	WeChat WeChat `yaml:"wechat"`
	Linx   Linx   `yaml:"linx"`
	Demo   Demo   `yaml:"demo"`
	Ops    Ops    `yaml:"ops"`
}

// Ops 是与厂家后台（palpable-admin）的对接，见 docs/factory.md §5。令牌只允许出现在这个文件里。
type Ops struct {
	// Token 非空时开放 /internal/devices/*（登记与吊销公钥、恢复出厂、锁定），后台带它来调用。该路径不应暴露到公网。
	Token string `yaml:"token"`
	// AdminURL 非空时，绑定与解绑会推送给后台；AdminToken 是后台的 inbound_token。
	AdminURL   string `yaml:"admin_url"`
	AdminToken string `yaml:"admin_token"`
	// OwnerHMACKey 用来把家长标识做成后台无法还原的 owner_ref。只存在于这里，不得发给后台。
	OwnerHMACKey string `yaml:"owner_hmac_key"`
}

const minSecretLen = 16

// Validate 检查对接配置：弱令牌、缺密钥都直接拒绝启动，免得上线后才发现后台能被猜中令牌。
func (o Ops) Validate() error {
	if o.Token != "" && len(o.Token) < minSecretLen {
		return fmt.Errorf("ops.token must be at least %d characters", minSecretLen)
	}
	if o.AdminURL == "" {
		return nil
	}
	if len(o.AdminToken) < minSecretLen {
		return fmt.Errorf("ops.admin_token must be at least %d characters when ops.admin_url is set", minSecretLen)
	}
	if len(o.OwnerHMACKey) < minSecretLen {
		return fmt.Errorf("ops.owner_hmac_key must be at least %d characters when ops.admin_url is set", minSecretLen)
	}
	return nil
}

// Auth 是设备认证配置，规则见 docs/trust.md §2。
type Auth struct {
	// Enrollment：open（首次上线登记公钥，默认）| manifest（只认出厂清单）| off（不认证，仅供固件过渡）。
	Enrollment string `yaml:"enrollment"`
}

// WeChat 是家长端小程序的登录配置；AppSecret 只允许出现在这个文件里。
type WeChat struct {
	AppID   string `yaml:"app_id"`
	Secret  string `yaml:"secret"`
	APIBase string `yaml:"api_base"` // 默认 https://api.weixin.qq.com，测试时指向假服务器
}

// Enabled 为 true 时开放微信登录与家长端 /api。
func (w WeChat) Enabled() bool { return w.AppID != "" && w.Secret != "" }

// Demo 是演示环境的预置：家长与同意标记在产品里由家长端绑定时取得。
type Demo struct {
	Guardian     string `yaml:"guardian"`      // 新设备自动绑定到这个家长 ID；空则不绑定
	GuardianName string `yaml:"guardian_name"` // 家长显示名
	VoiceConsent bool   `yaml:"voice_consent"` // 预置声纹登记的同意，默认开启；设为 false 则引导期不做声音登记
	ParentToken  string `yaml:"parent_token"`  // 家长端令牌，代表 Guardian；空则不开放 /api
	World        bool   `yaml:"world"`         // 开放 /world 演示地图；会暴露所有设备位置，默认关闭
	// 设为 true 时不自动绑定 Guardian，新设备播报绑定码，由家长端输入完成绑定（演示绑定流程用）。
	ManualBind bool `yaml:"manual_bind"`
	// 好友默认贴近即生效、家长收到通知；设为 true 则每个新好友都要家长先确认。
	FriendNeedsApproval bool `yaml:"friend_needs_approval"`
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
	cfg := Config{Addr: ":8080", DBPath: "palpable.db", Auth: Auth{Enrollment: "open"}, Demo: Demo{VoiceConsent: true}}
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
