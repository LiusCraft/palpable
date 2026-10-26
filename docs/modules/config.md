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
    Linx   Linx
    Demo   Demo
}
type Linx struct {
    WSURL, Token, APIBase, OTAURL, VoiceprintBase string   // WSURL 为空 = 不启用 AI
}
type Demo struct {
    Guardian, GuardianName string   // 新设备自动绑定的家长
    VoiceConsent bool               // 预置声纹登记同意，默认 true
    ParentToken  string             // 家长端令牌；空则不开放 /api
    World        bool               // 开放 /world；会暴露所有设备位置，默认关闭
    FriendNeedsApproval bool        // true：每个新好友都先经家长确认
}
func (l Linx) Enabled() bool        // WSURL != ""
```

各键的含义与默认值见 [根 README 的配置一节](../../README.md#配置)，不在此重复。

## 约束

- `web/` 与提交历史中不得出现 `linx.token` 等密钥。
- 配置只在启动时读取一次，由 `app.Run` 分发给各模块。
