# config

基础层 · `server/internal/config` · 依赖：`gopkg.in/yaml.v3`

## 边界

**职责**：从 YAML 加载服务端配置。**所有密钥只允许出现在这个文件里**，文件不入库（`server/config.yaml` 在 `.gitignore`）。

## 接口

```go
func Load(path string) (Config, error)
```

文件不存在返回默认配置（不启用 AI，本地可直接跑）；其他错误含**未知字段**一律上抛，拼错的键不会被静默忽略。

## 数据结构

```go
type Config struct {
    Addr   string // 默认 ":8080"
    DBPath string // SQLite 文件，默认 "palpable.db"
    Auth   Auth
    WeChat WeChat
    Linx   Linx
    Demo   Demo
}
type Auth struct {
    Enrollment string   // open（默认）| manifest | off，见 docs/trust.md §2.3
}
type WeChat struct {
    AppID, Secret string   // 两者都填才开放微信登录；Secret 只在这个文件里
    APIBase string         // 默认 https://api.weixin.qq.com，测试用
}
type Linx struct {
    WSURL, Token, APIBase, OTAURL, VoiceprintBase string   // WSURL 为空 = 不启用 AI
}
type Demo struct {
    Guardian, GuardianName string   // 新设备自动绑定的家长
    VoiceConsent bool               // 预置声纹登记同意，默认 true
    ParentToken  string             // 家长端令牌；空则不开放 /api
    World        bool               // 开放 /world；会暴露所有设备位置，默认关闭
    FriendNeedsApproval bool        // 演示预置的交友方式：false（默认）→ 自动绑定的孩子设为 notify；true → approve
    ManualBind   bool               // true：不自动绑定 Guardian，演示绑定码流程
}
func (l Linx) Enabled() bool        // WSURL != ""
```

各键的含义与默认值见 [根 README 的配置一节](../../README.md#配置)，不在此重复。

## 约束

- `web/`、`miniprogram/` 与提交历史中不得出现 `linx.token`、`wechat.secret` 等密钥（小程序 AppID 不是密钥，可以出现在 `project.config.json`）。
- 配置只在启动时读取一次，由 `app.Run` 分发给各模块。
