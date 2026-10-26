# zone

领域层 · `server/internal/zone` · 依赖：`channel`（经窄接口）、`id`

## 边界

**职责**：把设备坐标映射到区域，用滞回处理边界抖动；进入区域只发事件，孩子同意后才加入区域频道；离开直接退出频道。

**不负责**：频道成员管理（channel）；询问孩子（`app` 收到 `Entered` 后让小伙伴问）。

## 接口

```go
func New(chans Channels, zones ...Zone) *Zones       // 启动时对每个区域调用 chans.DefineZone

func (z *Zones) OnLocation(ctx, d, lat, lng) error
func (z *Zones) JoinCurrent(ctx, d) error            // 不在任何区域 → ErrNotInZone
func (z *Zones) Current(d) (Zone, bool)
func (z *Zones) ZoneAt(lat, lng) (ZoneID, bool)      // 不带滞回
func (z *Zones) Forget(d)                            // 断线时清位置，重连后重新判定
func (z *Zones) All() []Zone                         // 供 world 下发区域轮廓
func (z *Zones) Subscribe() <-chan Event             // Entered | Left

// zone 用到的 channel 能力
type Channels interface { DefineZone(ch, name); JoinZone(ctx, d, ch) error; LeaveZone(ctx, d) error }
```

## 数据结构

```go
type Zone struct { ID ZoneID; Name string; Polygon []LatLng }   // 频道 ID = "zone:<id>"；演示只有 Park
in  map[DeviceID]ZoneID     // 已越过进入滞回的设备当前所在区域
pos map[DeviceID]LatLng
```

## 关键流程

```text
OnLocation(d, p):
    cur, inside = in[d]
    if inside:
        if signedDistance(p, cur.polygon) <= -20m:        // 出到边界外 20 米
            delete in[d];  事件 Left；channel.LeaveZone(d)
    else:
        for z in zones:
            if signedDistance(p, z.polygon) >= +20m:      // 进到边界内 20 米
                in[d] = z;  事件 Entered; break           // 只发事件，不加入频道

JoinCurrent(d): z = in[d] 或 ErrNotInZone;  channel.JoinZone(d, z.channel)

signedDistance(p, poly):       // 在内为正、在外为负，单位米
    以 poly[0] 为原点做等距投影（区域只有几百米，足够准确）
    inside = 射线法
    minD  = p 到各边线段的最短距离
    return inside ? minD : -minD
```

## 约束

- 滞回：进入 +20m、离开 −20m，之间不变；边界附近抖动不会反复触发。
- 离开不经询问；进入必须孩子同意（AI 调 `self.zone.join` → `JoinCurrent`）。
- 单把互斥锁保护 `in / pos`；调用 `channel` 与发事件在解锁后。
- 事件缓冲 256，满了丢并记错误日志。
- 演示公园 `zone.Park` 的坐标同时用于 Web 地图（经 `/world` snapshot 下发）。
