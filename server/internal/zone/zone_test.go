package zone

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/id"
)

const dev id.DeviceID = "02:00:00:00:00:0a"

type fakeChannels struct {
	defined map[id.ChannelID]string
	joined  []id.ChannelID
	left    int
}

func (f *fakeChannels) DefineZone(ch id.ChannelID, name string) { f.defined[ch] = name }
func (f *fakeChannels) JoinZone(_ context.Context, _ id.DeviceID, ch id.ChannelID) error {
	f.joined = append(f.joined, ch)
	return nil
}
func (f *fakeChannels) LeaveZone(context.Context, id.DeviceID) error { f.left++; return nil }

// 公园西边界经度 121.4700，纬度取中线；1 米约等于 1/(111320*cos(lat)) 度经度。
const midLat = 31.2270

func lngAt(meters float64) float64 {
	return 121.4700 + meters/(111320*math.Cos(midLat*math.Pi/180))
}

func next(t *testing.T, z *Zones) (Event, bool) {
	t.Helper()
	select {
	case e := <-z.Subscribe():
		return e, true
	default:
		return Event{}, false
	}
}

func TestEnterAskLeaveWithHysteresis(t *testing.T) {
	fc := &fakeChannels{defined: map[id.ChannelID]string{}}
	z := New(fc, Park)
	ctx := context.Background()
	if fc.defined["zone:park"] != "公园" {
		t.Fatalf("zone channel not defined: %v", fc.defined)
	}

	if err := z.JoinCurrent(ctx, dev); !errors.Is(err, ErrNotInZone) {
		t.Fatalf("JoinCurrent outside: %v", err)
	}

	_ = z.OnLocation(ctx, dev, midLat, lngAt(10)) // 只进去 10 米：不算进入
	if _, ok := next(t, z); ok {
		t.Fatal("entered before crossing hysteresis")
	}
	_ = z.OnLocation(ctx, dev, midLat, lngAt(30))
	if e, ok := next(t, z); !ok || e.Kind != Entered || e.Zone.ID != "park" {
		t.Fatalf("event = %+v %v", e, ok)
	}
	if len(fc.joined) != 0 {
		t.Fatal("entering must not join before the kid agrees")
	}
	if err := z.JoinCurrent(ctx, dev); err != nil || len(fc.joined) != 1 || fc.joined[0] != "zone:park" {
		t.Fatalf("JoinCurrent: %v %v", err, fc.joined)
	}

	_ = z.OnLocation(ctx, dev, midLat, lngAt(-10)) // 出去 10 米：不算离开
	if _, ok := next(t, z); ok {
		t.Fatal("left before crossing hysteresis")
	}
	_ = z.OnLocation(ctx, dev, midLat, lngAt(-30))
	if e, ok := next(t, z); !ok || e.Kind != Left {
		t.Fatalf("event = %+v %v", e, ok)
	}
	if fc.left != 1 {
		t.Fatal("leaving must leave the zone channel directly")
	}
}

func TestZoneAt(t *testing.T) {
	z := New(&fakeChannels{defined: map[id.ChannelID]string{}}, Park)
	if zid, ok := z.ZoneAt(midLat, lngAt(5)); !ok || zid != "park" {
		t.Fatalf("ZoneAt inside = %v %v", zid, ok)
	}
	if _, ok := z.ZoneAt(midLat, lngAt(-5)); ok {
		t.Fatal("ZoneAt outside")
	}
}
