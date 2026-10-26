# device

领域层 · `server/internal/device` · 依赖：`store`、`linxapi`、`id`

## 边界

**职责**：设备档案与自动开通。新设备先绑定引导智能体；起名后交棒，按皮套选风格智能体并切换；换皮套时切换智能体；记录名字；经监护人同意后登记声纹。整体流程见 [架构 §4](../architecture.md#4-设备如何组织)。

**不负责**：灵矽 REST 细节（linxapi）；打招呼与语音交互、重连灵矽会话（app 调 bridge/linx）；好友与频道。

## 接口

```go
type Devices struct{ /* 一把互斥锁串行化开通与交棒 */ }

func (d *Devices) Ensure(ctx, DeviceInfo) (Profile, error)               // 幂等；换皮套时切换智能体
func (d *Devices) Profile(ctx, id) (Profile, error)                      // 不存在 ErrNotFound
func (d *Devices) SetNames(ctx, id, doll, kid string) error              // fresh → named，发 Named 事件
func (d *Devices) Handoff(ctx, id) (Profile, error)                      // 幂等；named → ready
func (d *Devices) EnrollVoice(ctx, id) error                             // ErrNoConsent / ErrNotOnboarding / ErrNoSample
func (d *Devices) Bind(ctx, id, GuardianID) error
func (d *Devices) SetVoiceConsent(ctx, id, on bool) error
func (d *Devices) ByGuardian(ctx, GuardianID) ([]Profile, error)
func (d *Devices) Delete(ctx, id) error                                  // 同一事务删好友、群成员、历史
func (d *Devices) Subscribe() <-chan Named                               // {Device, DollName, KidName}
func (d *Devices) EnrollAttempts() <-chan id.DeviceID                    // 每次声纹登记尝试结束（成败都发）

type Catalog struct{ /* 引导智能体 + 按 skin 的风格智能体 */ }
func (c *Catalog) EnsureAll(ctx) error     // 启动时确保灵矽侧的智能体存在并同步提示词
func (c *Catalog) Onboarding() Style
func (c *Catalog) ForSkin(skin string) Style   // 未知 skin 返回默认风格（bear）
```

## 数据结构

```go
type Profile struct {
    ID           id.DeviceID    // = MAC，存储时统一小写
    Skin         string
    StyleID      string         // fresh/named 时 "welcome"，ready 后为风格 ID
    AgentID      string         // 当前绑定的灵矽智能体
    DollName, KidName string    // 各 1–12 个字符
    Guardian     id.GuardianID
    VoiceConsent bool           // 监护人对声纹登记的单独同意
    SpeakerID    string         // 声纹说话人；只登记，不绑定到智能体
    Stage        Stage          // StageFresh → StageNamed → StageReady
    CreatedAt    time.Time
}

type Style struct { ID, Skin, AgentName, SystemPrompt, VoiceID, AgentID string }   // 提示词带 {{doll_name}} {{kid_name}}；引导智能体另带 {{voice_step}}
```

不变量：`Stage=ready` ⇔ 已绑定风格智能体；`SpeakerID` 只在引导期写入；档案不存音频、不存声纹特征。

## 状态机

```text
            Ensure（无档案）                SetNames（首次）               Handoff
  (none) ───────────────────▶ fresh ─────────────────────▶ named ─────────────────▶ ready
                          绑定引导智能体      发 Named 事件            切风格智能体 + 写记忆焦点
                                                                      ↺ 换 skin：Ensure 时重新绑定风格
```

| 当前 | 调用 | 结果 |
| --- | --- | --- |
| fresh | `Ensure` | 原样返回；此时换 `skin` 只更新档案，交棒时按当时的 skin 选风格 |
| fresh | `Handoff` | `ErrNotNamed` |
| named | `Ensure` | 原样返回，由 app 重试交棒 |
| named | `Handoff` | 绑定风格 → ready |
| ready | `Handoff` | 直接返回（幂等） |
| ready | `Ensure(新 skin)` | 风格变了则切换智能体 |
| ready | `SetNames` | 只更新名字并刷新记忆焦点，不发 Named |

## 关键流程

```text
provision(mac):                                         // Ensure 无档案时
    welcome = catalog.Onboarding(); AgentID 为空 → 失败（目录未就绪）
    err = linx.PreregisterDevice(welcome.AgentID, mac)
    if err == ErrAlreadyRegistered: linx.SwitchAgent(mac, welcome.AgentID)   // 本地库被清空等情况
    linx.ActivateDevice(mac)                            // 以设备身份访问 OTA 完成绑定；失败只告警
    repo.Create(Profile{stage: fresh, style: welcome})

bindStyle(p, style):                                    // Handoff / 换皮套
    linx.SwitchAgent(mac, style.AgentID, migrateHistory=false)   // 引导期对话不迁移
    p.StyleID, p.AgentID = style.ID, style.AgentID
    writeMemory(p)          // EnableMemory → UpsertAttention(小伙伴的名字 / 孩子的称呼) → SetAttentionValue；失败只告警

EnrollVoice(d):
    !VoiceConsent → ErrNoConsent;   StyleID != welcome → ErrNotOnboarding
    defer 通知 enrolled 通道                            // 成败都通知，app 据此决定何时交棒
    samples = linx.RecentVoiceSamples(p.AgentID)
    取 MAC == d 且 now - At ≤ 10min 的最新一条；没有 → ErrNoSample    // 引导智能体被所有新设备共用
    speaker = p.SpeakerID ?? linx.CreateSpeaker("可感-"+mac)
    linx.EnrollVoiceprint(speaker, audioURL);  p.SpeakerID = speaker

Catalog.ensure(style):
    agent = repo.Get(style) ?? linx.FindAgentByName ?? linx.CreateAgent
    linx.UpdateAgent(agent, spec)       // 每次启动同步提示词与音色，代码里改了提示词已有智能体也生效
    repo.Put(style → agent)
```

引导智能体：不开长期记忆，聊天记录存音频（仅为取声纹样本，产品已允许，见 [linx-integration](../linx-integration.md#产品决定)）；风格智能体开长期记忆，只存文字。

## 约束

- `Devices` 一把互斥锁串行化开通、交棒、更新，避免并发重复调灵矽；`Profile` 读不加锁。
- 名字去空白后必须 1–12 个字符且是合法 UTF-8，否则 `ErrBadName`；不存其他个人信息。
- `SetNames` 的 Named 事件缓冲 64，满了丢并记日志（下次上线由 `stage=named` 重试交棒兜底）。
- 皮套与风格对应在 `styles.go`：dragon / rabbit / bear / cat；所有提示词带安全规则，禁止询问地址、学校、电话。
- 灵矽侧声纹的删除由调用方另行处理（`Delete` 只删本地）。
