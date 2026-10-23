package zone

import (
	"context"
	"math"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/id"
)

func TestCurrentForgetAllAndEmitDrop(t *testing.T) {
	z := New(&fakeChannels{defined: map[id.ChannelID]string{}}, Park)
	ctx := context.Background()
	if len(z.All()) != 1 || z.All()[0].Channel() != "zone:park" {
		t.Fatalf("All = %+v", z.All())
	}
	if _, ok := z.Current(dev); ok {
		t.Fatal("no zone before any location")
	}
	_ = z.OnLocation(ctx, dev, midLat, lngAt(40))
	if zn, ok := z.Current(dev); !ok || zn.ID != "park" {
		t.Fatalf("Current = %+v %v", zn, ok)
	}
	z.Forget(dev)
	if _, ok := z.Current(dev); ok {
		t.Fatal("Forget must clear the zone")
	}
	if z.byID("nope").ID != "" {
		t.Fatal("unknown zone id")
	}
	for range eventBuffer + 2 {
		z.emit(Event{Kind: Entered, Device: dev})
	}
	if len(z.events) != eventBuffer {
		t.Fatalf("buffered = %d", len(z.events))
	}
}

func TestSignedDistanceDegenerate(t *testing.T) {
	if !math.IsInf(signedDistance(LatLng{0, 0}, []LatLng{{0, 0}, {1, 1}}), -1) {
		t.Fatal("polygon with fewer than 3 points is never inside")
	}
	if d := segDist(1, 1, 0, 0, 0, 0); math.Abs(d-math.Sqrt2) > 1e-9 {
		t.Fatalf("segDist to a point = %v", d)
	}
}
