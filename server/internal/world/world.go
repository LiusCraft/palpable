// Package world 是演示用的只读观察通道：把设备位置、谁在说话、频道成员、好友关系实时推给 Web 地图。
// 它会暴露所有设备的位置，只在演示模式注册；不属于设备协议，也不做任何业务判断。
package world

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/id"
)

const (
	subscriberBuffer = 256
	writeTimeout     = 5 * time.Second
)

type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Zone struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Channel string   `json:"channel"`
	Polygon []LatLng `json:"polygon"`
}

// Event 的 Kind：
//   - "device"：上线 / 下线 / 名字或皮套变化（Online、Name、Skin）
//   - "pos"：位置（Pos）
//   - "member"：进出频道（Channel，Joined）
//   - "burst"：开始 / 结束说话（Channel，Speaking）
//   - "friend"：好友关系（Friend，Status：pending / confirmed / removed）
//   - "snapshot"：连接时的全量状态（Devices、Friends、Zones）
type Event struct {
	Kind     string        `json:"kind"`
	Device   id.DeviceID   `json:"device,omitempty"`
	Name     string        `json:"name,omitempty"`
	Skin     string        `json:"skin,omitempty"`
	Online   *bool         `json:"online,omitempty"`
	Pos      *LatLng       `json:"pos,omitempty"`
	Channel  id.ChannelID  `json:"channel,omitempty"`
	Joined   *bool         `json:"joined,omitempty"`
	Speaking *bool         `json:"speaking,omitempty"`
	Friend   *id.DeviceID  `json:"friend,omitempty"`
	Status   string        `json:"status,omitempty"`
	Devices  []DeviceState `json:"devices,omitempty"`
	Friends  []Friendship  `json:"friends,omitempty"`
	Zones    []Zone        `json:"zones,omitempty"`
}

type DeviceState struct {
	Device   id.DeviceID    `json:"device"`
	Name     string         `json:"name"`
	Skin     string         `json:"skin"`
	Online   bool           `json:"online"`
	Pos      *LatLng        `json:"pos,omitempty"`
	Channels []id.ChannelID `json:"channels"`
	Speaking bool           `json:"speaking"`
}

type Friendship struct {
	A      id.DeviceID `json:"a"`
	B      id.DeviceID `json:"b"`
	Status string      `json:"status"`
}

type World struct {
	upgrader websocket.Upgrader

	mu      sync.Mutex
	zones   []Zone
	devices map[id.DeviceID]*DeviceState
	friends map[[2]id.DeviceID]string
	subs    map[chan Event]struct{}
}

func New(zones []Zone, checkOrigin func(*http.Request) bool) *World {
	return &World{
		upgrader: websocket.Upgrader{CheckOrigin: checkOrigin},
		zones:    zones,
		devices:  map[id.DeviceID]*DeviceState{},
		friends:  map[[2]id.DeviceID]string{},
		subs:     map[chan Event]struct{}{},
	}
}

// Publish 更新状态并推给所有观察者；由 app 调用，慢的观察者丢事件，不阻塞调用方。
func (w *World) Publish(e Event) {
	w.mu.Lock()
	w.apply(e)
	for ch := range w.subs {
		select {
		case ch <- e:
		default:
		}
	}
	w.mu.Unlock()
}

func (w *World) apply(e Event) {
	if e.Kind == "friend" && e.Friend != nil {
		k := pairKey(e.Device, *e.Friend)
		if e.Status == "removed" {
			delete(w.friends, k)
		} else {
			w.friends[k] = e.Status
		}
		return
	}
	if e.Device == "" {
		return
	}
	d := w.devices[e.Device]
	if d == nil {
		d = &DeviceState{Device: e.Device, Channels: []id.ChannelID{}}
		w.devices[e.Device] = d
	}
	switch e.Kind {
	case "device":
		if e.Online != nil {
			d.Online = *e.Online
			if !d.Online {
				d.Speaking = false
				d.Channels = []id.ChannelID{}
			}
		}
		if e.Name != "" {
			d.Name = e.Name
		}
		if e.Skin != "" {
			d.Skin = e.Skin
		}
	case "pos":
		d.Pos = e.Pos
	case "member":
		out := d.Channels[:0:0]
		for _, c := range d.Channels {
			if c != e.Channel {
				out = append(out, c)
			}
		}
		if e.Joined != nil && *e.Joined {
			out = append(out, e.Channel)
		}
		d.Channels = out
	case "burst":
		if e.Speaking != nil {
			d.Speaking = *e.Speaking
		}
	}
}

func (w *World) snapshot() Event {
	devs := make([]DeviceState, 0, len(w.devices))
	for _, d := range w.devices {
		c := *d
		c.Channels = append([]id.ChannelID{}, d.Channels...)
		devs = append(devs, c)
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].Device < devs[j].Device })
	fs := make([]Friendship, 0, len(w.friends))
	for k, s := range w.friends {
		fs = append(fs, Friendship{A: k[0], B: k[1], Status: s})
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].A+fs[i].B < fs[j].A+fs[j].B })
	return Event{Kind: "snapshot", Devices: devs, Friends: fs, Zones: w.zones}
}

// ServeHTTP 把 /world 升级为 WebSocket：先发快照，再推增量，直到断开。
func (w *World) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ws, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	ch := make(chan Event, subscriberBuffer)
	w.mu.Lock()
	first := w.snapshot()
	w.subs[ch] = struct{}{}
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.subs, ch)
		w.mu.Unlock()
	}()

	// 读循环只为发现断开；观察者不发消息。
	go func() {
		defer cancel()
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}()

	send := func(e Event) bool {
		_ = ws.SetWriteDeadline(time.Now().Add(writeTimeout))
		return ws.WriteJSON(e) == nil
	}
	if !send(first) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
			if !send(e) {
				slog.Debug("world observer gone")
				return
			}
		}
	}
}

func pairKey(a, b id.DeviceID) [2]id.DeviceID {
	if a > b {
		a, b = b, a
	}
	return [2]id.DeviceID{a, b}
}

func Bool(b bool) *bool { return &b }
