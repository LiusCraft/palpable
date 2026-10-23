package world

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/id"
)

func TestSnapshotThenUpdates(t *testing.T) {
	w := New([]Zone{{ID: "park", Name: "公园", Channel: "zone:park", Polygon: []LatLng{{1, 1}, {1, 2}, {2, 2}}}}, func(*http.Request) bool { return true })
	const a, b id.DeviceID = "02:00:00:00:00:0a", "02:00:00:00:00:0b"
	w.Publish(Event{Kind: "device", Device: a, Name: "小龙", Skin: "dragon", Online: Bool(true)})
	w.Publish(Event{Kind: "pos", Device: a, Pos: &LatLng{31.22, 121.47}})
	w.Publish(Event{Kind: "member", Device: a, Channel: "zone:park", Joined: Bool(true)})
	friend := b
	w.Publish(Event{Kind: "friend", Device: a, Friend: &friend, Status: "confirmed"})

	srv := httptest.NewServer(w)
	defer srv.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))

	var snap Event
	if err := ws.ReadJSON(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.Kind != "snapshot" || len(snap.Zones) != 1 || len(snap.Devices) != 1 || len(snap.Friends) != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
	d := snap.Devices[0]
	if d.Name != "小龙" || !d.Online || d.Pos == nil || len(d.Channels) != 1 {
		t.Fatalf("device = %+v", d)
	}

	w.Publish(Event{Kind: "burst", Device: a, Channel: "zone:park", Speaking: Bool(true)})
	var e Event
	if err := ws.ReadJSON(&e); err != nil || e.Kind != "burst" || e.Speaking == nil || !*e.Speaking {
		t.Fatalf("update = %+v %v", e, err)
	}

	// 下线清掉频道与说话状态。
	w.Publish(Event{Kind: "device", Device: a, Online: Bool(false)})
	w.mu.Lock()
	st := *w.devices[a]
	w.mu.Unlock()
	if st.Online || st.Speaking || len(st.Channels) != 0 {
		t.Fatalf("offline state = %+v", st)
	}
}
