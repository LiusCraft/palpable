# share

领域层 · `server/internal/share` · 依赖：`channel`、`relation`、`history`、`id`（均经窄接口）；端口 `Announcer`

## 边界

**职责**：画面分享。孩子让小伙伴描述眼前的画面，描述播给前台频道里的好友，也播给孩子自己，并记入历史。

**不负责**：拍照与看图（设备工具与灵矽视觉）；语音转发（经 `Announcer`，不经 `voice`）。

## 接口

```go
func New(m Membership, f Friends, announce Announcer, rec Recorder, names Names) *Share
func (s *Share) Publish(ctx, from DeviceID, description string) (heard int, err error)
// ErrEmpty / ErrNotInChannel

// 端口
type Announcer interface { Say(ctx, d, text) error }                          // bridge/linx.Say
type Membership interface { ForegroundOf(p) (ChannelID, bool); Members(ch) []ParticipantID }
type Friends   interface { FilterFriends(ctx, viewer, ps) []ParticipantID }  // relation
type Recorder  interface { RecordShared(ctx, d, SharedScene) error }         // history
```

## 关键流程

```text
Publish(from, description):
    description = trim；空 → ErrEmpty；超过 200 字 → 截断加「……」
    ch = ForegroundOf(from)；没有 → ErrNotInChannel
    name = 小伙伴名 ?? "好朋友"
    text = name + "看到：" + description
    targets = [from] + FilterFriends(from, Members(ch)) 去掉 from      // 与 voice 同一好友规则
    for d in targets:
        announce.Say(d, text)                         // 小伙伴直接说出，不经 LLM
        history.RecordShared(d, scene)                // 各自的家长都能在回顾里看到
    return len(targets) - 1
```

## 约束

- 非好友永远在 `targets` 之外：与 voice 共用 `relation` 的判定，不另立规则。
- 单个目标播报或记录失败只告警，不影响其他目标。
- 这是「小伙伴讲出来」，不是把照片发给别人；只在前台频道内才会被好友听到。
- 未配置灵矽时不注册 `self.share.publish`。
