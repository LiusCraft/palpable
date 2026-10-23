// Package zone 把设备坐标映射到区域，处理边界抖动；进入只发事件等孩子同意，离开直接退出区域频道。
package zone

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"

	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/id"
)

const (
	// 进出各设 20 米滞回：进到边界内 20 米才算进入，出到边界外 20 米才算离开。
	hysteresis  = 20.0
	eventBuffer = 256
	earthRadius = 6371000.0
)

var ErrNotInZone = errors.New("zone: not in any zone")

type ZoneID string // 对应频道 "zone:<id>"

type LatLng struct{ Lat, Lng float64 }

type Zone struct {
	ID      ZoneID
	Name    string
	Polygon []LatLng
}

// Channel 是区域对应的频道 ID。
func (z Zone) Channel() id.ChannelID { return channel.ZoneID(string(z.ID)) }

// Park 是演示用的公园，Web 地图与这里用同一组坐标（经 /world 下发）。
var Park = Zone{
	ID:   "park",
	Name: "公园",
	Polygon: []LatLng{
		{31.2290, 121.4700}, {31.2290, 121.4760}, {31.2250, 121.4760}, {31.2250, 121.4700},
	},
}

type EventKind int

const (
	Entered EventKind = iota
	Left
)

type Event struct {
	Kind   EventKind
	Device id.DeviceID
	Zone   Zone
}

// Channels 是 zone 用到的 channel 能力。
type Channels interface {
	DefineZone(ch id.ChannelID, name string)
	JoinZone(ctx context.Context, d id.DeviceID, ch id.ChannelID) error
	LeaveZone(ctx context.Context, d id.DeviceID) error
}

type Zones struct {
	zones  []Zone
	chans  Channels
	events chan Event

	mu  sync.Mutex
	in  map[id.DeviceID]ZoneID // 当前所在区域（已越过进入滞回）
	pos map[id.DeviceID]LatLng
}

func New(chans Channels, zones ...Zone) *Zones {
	for _, z := range zones {
		chans.DefineZone(z.Channel(), z.Name)
	}
	return &Zones{zones: zones, chans: chans, events: make(chan Event, eventBuffer), in: map[id.DeviceID]ZoneID{}, pos: map[id.DeviceID]LatLng{}}
}

func (z *Zones) Subscribe() <-chan Event { return z.events }

func (z *Zones) All() []Zone { return z.zones }

func (z *Zones) OnLocation(ctx context.Context, d id.DeviceID, lat, lng float64) error {
	p := LatLng{lat, lng}
	z.mu.Lock()
	z.pos[d] = p
	cur, inside := z.in[d]
	var ev *Event
	if inside {
		zn := z.byID(cur)
		if signedDistance(p, zn.Polygon) <= -hysteresis {
			delete(z.in, d)
			ev = &Event{Kind: Left, Device: d, Zone: zn}
		}
	} else {
		for _, zn := range z.zones {
			if signedDistance(p, zn.Polygon) >= hysteresis {
				z.in[d] = zn.ID
				ev = &Event{Kind: Entered, Device: d, Zone: zn}
				break
			}
		}
	}
	z.mu.Unlock()

	if ev == nil {
		return nil
	}
	if ev.Kind == Left {
		if err := z.chans.LeaveZone(ctx, d); err != nil {
			return fmt.Errorf("leave zone %s: %w", ev.Zone.ID, err)
		}
	}
	z.emit(*ev)
	return nil
}

// JoinCurrent 在孩子同意后把设备加入当前区域频道。
func (z *Zones) JoinCurrent(ctx context.Context, d id.DeviceID) error {
	z.mu.Lock()
	cur, ok := z.in[d]
	z.mu.Unlock()
	if !ok {
		return ErrNotInZone
	}
	return z.chans.JoinZone(ctx, d, z.byID(cur).Channel())
}

// Current 返回设备当前所在区域。
func (z *Zones) Current(d id.DeviceID) (Zone, bool) {
	z.mu.Lock()
	defer z.mu.Unlock()
	cur, ok := z.in[d]
	if !ok {
		return Zone{}, false
	}
	return z.byID(cur), true
}

// Forget 在设备断线时清掉位置，重连后重新判定进入。
func (z *Zones) Forget(d id.DeviceID) {
	z.mu.Lock()
	defer z.mu.Unlock()
	delete(z.in, d)
	delete(z.pos, d)
}

// ZoneAt 不带滞回，只看点是否在区域内。
func (z *Zones) ZoneAt(lat, lng float64) (ZoneID, bool) {
	for _, zn := range z.zones {
		if signedDistance(LatLng{lat, lng}, zn.Polygon) >= 0 {
			return zn.ID, true
		}
	}
	return "", false
}

func (z *Zones) byID(zid ZoneID) Zone {
	for _, zn := range z.zones {
		if zn.ID == zid {
			return zn
		}
	}
	return Zone{}
}

func (z *Zones) emit(e Event) {
	select {
	case z.events <- e:
	default:
		slog.Error("zone event dropped: subscriber too slow", "kind", e.Kind, "device_id", e.Device)
	}
}

// signedDistance 返回点到多边形边界的距离（米），在内为正、在外为负。
// 区域只有几百米，用以第一个顶点为原点的等距投影足够准确。
func signedDistance(p LatLng, poly []LatLng) float64 {
	if len(poly) < 3 {
		return math.Inf(-1)
	}
	o := poly[0]
	toXY := func(q LatLng) (float64, float64) {
		x := (q.Lng - o.Lng) * math.Pi / 180 * earthRadius * math.Cos(o.Lat*math.Pi/180)
		y := (q.Lat - o.Lat) * math.Pi / 180 * earthRadius
		return x, y
	}
	px, py := toXY(p)
	inside := false
	minD := math.Inf(1)
	for i := range poly {
		ax, ay := toXY(poly[i])
		bx, by := toXY(poly[(i+1)%len(poly)])
		if (ay > py) != (by > py) && px < (bx-ax)*(py-ay)/(by-ay)+ax {
			inside = !inside
		}
		minD = math.Min(minD, segDist(px, py, ax, ay, bx, by))
	}
	if inside {
		return minD
	}
	return -minD
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l))
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}
