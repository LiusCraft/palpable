package share

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/history"
	"github.com/LiusCraft/palpable/server/internal/id"
)

const (
	me       id.DeviceID = "02:00:00:00:00:0a"
	friend   id.DeviceID = "02:00:00:00:00:0b"
	stranger id.DeviceID = "02:00:00:00:00:0c"
)

type members struct{ in bool }

func (m members) ForegroundOf(id.ParticipantID) (id.ChannelID, bool) { return "zone:park", m.in }
func (m members) Members(id.ChannelID) []id.ParticipantID {
	return []id.ParticipantID{id.Device(me), id.Device(friend), id.Device(stranger)}
}

type friends struct{}

func (friends) FilterFriends(_ context.Context, _ id.ParticipantID, ps []id.ParticipantID) []id.ParticipantID {
	var out []id.ParticipantID
	for _, p := range ps {
		if p == id.Device(friend) {
			out = append(out, p)
		}
	}
	return out
}

type said map[id.DeviceID]string

func (s said) Say(_ context.Context, d id.DeviceID, text string) error { s[d] = text; return nil }

type recorded map[id.DeviceID]history.SharedScene

func (r recorded) RecordShared(_ context.Context, d id.DeviceID, sc history.SharedScene) error {
	r[d] = sc
	return nil
}

func TestPublishReachesSelfAndFriendsOnly(t *testing.T) {
	sd, rec := said{}, recorded{}
	s := New(members{in: true}, friends{}, sd, rec, func(context.Context, id.DeviceID) string { return "小龙" })
	n, err := s.Publish(context.Background(), me, "  一棵很高很高的大树  ")
	if err != nil || n != 1 {
		t.Fatalf("Publish = %d %v", n, err)
	}
	var got []string
	for d := range sd {
		got = append(got, string(d))
	}
	sort.Strings(got)
	if len(got) != 2 || sd[me] != "小龙看到：一棵很高很高的大树" || sd[friend] != sd[me] {
		t.Fatalf("said = %v", sd)
	}
	if _, heard := sd[stranger]; heard {
		t.Fatal("stranger must not hear the share")
	}
	if rec[friend].Text != "一棵很高很高的大树" || rec[me].From != me {
		t.Fatalf("recorded = %+v", rec)
	}
}

func TestPublishOutsideChannel(t *testing.T) {
	s := New(members{in: false}, friends{}, said{}, recorded{}, func(context.Context, id.DeviceID) string { return "" })
	if _, err := s.Publish(context.Background(), me, "x"); !errors.Is(err, ErrNotInChannel) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Publish(context.Background(), me, " "); !errors.Is(err, ErrEmpty) {
		t.Fatalf("err = %v", err)
	}
}
