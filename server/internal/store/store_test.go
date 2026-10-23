package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
)

const (
	macA id.DeviceID = "aa:aa:aa:aa:aa:aa"
	macB id.DeviceID = "bb:bb:bb:bb:bb:bb"
	macC id.DeviceID = "cc:cc:cc:cc:cc:cc"
)

var t0 = time.Date(2026, 10, 23, 9, 0, 0, 123_456_789, time.UTC)

// 两种实现跑同一套用例，假实现才不会和真实存储走样。
func implementations(t *testing.T) map[string]func(t *testing.T) Store {
	return map[string]func(t *testing.T) Store{
		"sqlite": func(t *testing.T) Store {
			s, err := Open(context.Background(), filepath.Join(t.TempDir(), "palpable.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		},
		"memory": func(*testing.T) Store { return NewMemory() },
	}
}

func run(t *testing.T, fn func(t *testing.T, ctx context.Context, s Store)) {
	for name, open := range implementations(t) {
		t.Run(name, func(t *testing.T) { fn(t, context.Background(), open(t)) })
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantErr(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("err = %v, want %v", err, target)
	}
}

func TestDevices(t *testing.T) {
	run(t, func(t *testing.T, ctx context.Context, s Store) {
		r := s.Devices()
		_, err := r.Get(ctx, macA)
		wantErr(t, err, ErrNotFound)

		dev := Device{ID: macA, Skin: "dragon", StyleID: "welcome", AgentID: "ag1", Stage: 0, CreatedAt: t0}
		must(t, r.Create(ctx, dev))
		wantErr(t, r.Create(ctx, dev), ErrConflict)

		got, err := r.Get(ctx, macA)
		must(t, err)
		dev.CreatedAt = t0.Truncate(time.Millisecond)
		if got != dev {
			t.Fatalf("got %+v, want %+v", got, dev)
		}

		upd := got
		upd.DollName, upd.KidName, upd.Stage = "小龙", "朵朵", 1
		upd.Guardian, upd.VoiceConsent, upd.SpeakerID = "g1", true, "spk1"
		upd.CreatedAt = t0.Add(time.Hour) // 应被忽略
		must(t, r.Update(ctx, upd))
		got, err = r.Get(ctx, macA)
		must(t, err)
		upd.CreatedAt = dev.CreatedAt
		if got != upd {
			t.Fatalf("got %+v, want %+v", got, upd)
		}

		must(t, r.Create(ctx, Device{ID: macB, Guardian: "g1", CreatedAt: t0.Add(time.Second)}))
		must(t, r.Create(ctx, Device{ID: macC, Guardian: "g2", CreatedAt: t0}))
		mine, err := r.ListByGuardian(ctx, "g1")
		must(t, err)
		if len(mine) != 2 || mine[0].ID != macA || mine[1].ID != macB {
			t.Fatalf("ListByGuardian = %+v", mine)
		}
		must(t, r.Delete(ctx, macB))
		must(t, r.Delete(ctx, macC))

		wantErr(t, r.Update(ctx, Device{ID: macB}), ErrNotFound)
		wantErr(t, r.Delete(ctx, macB), ErrNotFound)
	})
}

func TestDeleteDeviceCascades(t *testing.T) {
	run(t, func(t *testing.T, ctx context.Context, s Store) {
		for _, d := range []id.DeviceID{macA, macB, macC} {
			must(t, s.Devices().Create(ctx, Device{ID: d, CreatedAt: t0}))
		}
		fr := s.Friendships()
		must(t, fr.Create(ctx, Friendship{ID: "f1", A: macA, B: macB, CreatedAt: t0}))
		must(t, fr.Create(ctx, Friendship{ID: "f2", A: macB, B: macC, CreatedAt: t0}))

		gr := s.Groups()
		must(t, gr.Create(ctx, Group{ID: "group:1", Owner: macB, Code: "1234"}))
		must(t, gr.AddMember(ctx, "group:1", Member{Participant: id.Device(macA), JoinedAt: t0}))
		must(t, gr.AddMember(ctx, "group:1", Member{Participant: id.Device(macB), JoinedAt: t0}))

		h := s.History()
		must(t, h.Append(ctx, HistoryEvent{ID: "e1", Device: macA, At: t0}))
		must(t, h.Append(ctx, HistoryEvent{ID: "e2", Device: macB, At: t0}))

		must(t, s.Devices().Delete(ctx, macA))

		_, err := s.Devices().Get(ctx, macA)
		wantErr(t, err, ErrNotFound)
		_, err = fr.Get(ctx, "f1")
		wantErr(t, err, ErrNotFound)
		if _, err := fr.Get(ctx, "f2"); err != nil {
			t.Fatalf("unrelated friendship removed: %v", err)
		}
		ms, err := gr.Members(ctx, "group:1")
		must(t, err)
		if len(ms) != 1 || ms[0].Participant != id.Device(macB) {
			t.Fatalf("members = %+v", ms)
		}
		evs, err := h.ListByDevice(ctx, macA, time.Time{}, t0.Add(time.Hour))
		must(t, err)
		if len(evs) != 0 {
			t.Fatalf("history not removed: %+v", evs)
		}
		evs, err = h.ListByDevice(ctx, macB, time.Time{}, t0.Add(time.Hour))
		must(t, err)
		if len(evs) != 1 {
			t.Fatalf("unrelated history removed: %+v", evs)
		}
	})
}

func TestStylesAndGuardians(t *testing.T) {
	run(t, func(t *testing.T, ctx context.Context, s Store) {
		st := s.Styles()
		_, err := st.Get(ctx, "welcome")
		wantErr(t, err, ErrNotFound)
		must(t, st.Put(ctx, Style{ID: "welcome", AgentID: "a0"}))
		must(t, st.Put(ctx, Style{ID: "dragon", Skin: "dragon", AgentID: "a1"}))
		must(t, st.Put(ctx, Style{ID: "dragon", Skin: "dragon", AgentID: "a2"}))
		got, err := st.Get(ctx, "dragon")
		must(t, err)
		if got.AgentID != "a2" {
			t.Fatalf("put did not overwrite: %+v", got)
		}
		all, err := st.List(ctx)
		must(t, err)
		if len(all) != 2 || all[0].ID != "dragon" || all[1].ID != "welcome" {
			t.Fatalf("list = %+v", all)
		}

		gr := s.Guardians()
		_, err = gr.Get(ctx, "g1")
		wantErr(t, err, ErrNotFound)
		must(t, gr.Put(ctx, Guardian{ID: "g1", DisplayName: "妈妈"}))
		must(t, gr.Put(ctx, Guardian{ID: "g1", DisplayName: "爸爸"}))
		g, err := gr.Get(ctx, "g1")
		must(t, err)
		if g.DisplayName != "爸爸" {
			t.Fatalf("guardian = %+v", g)
		}
	})
}

func TestFriendships(t *testing.T) {
	run(t, func(t *testing.T, ctx context.Context, s Store) {
		r := s.Friendships()
		must(t, r.Create(ctx, Friendship{ID: "f1", A: macB, B: macA, Status: 0, CreatedAt: t0}))

		f, err := r.Get(ctx, "f1")
		must(t, err)
		if f.A != macA || f.B != macB {
			t.Fatalf("pair not ordered: %+v", f)
		}
		if _, err := r.ByPair(ctx, macB, macA); err != nil {
			t.Fatalf("ByPair reversed: %v", err)
		}
		_, err = r.ByPair(ctx, macA, macC)
		wantErr(t, err, ErrNotFound)

		wantErr(t, r.Create(ctx, Friendship{ID: "f2", A: macA, B: macB, CreatedAt: t0}), ErrConflict)
		wantErr(t, r.Create(ctx, Friendship{ID: "f1", A: macA, B: macC, CreatedAt: t0}), ErrConflict)
		if err := r.Create(ctx, Friendship{ID: "f3", A: macA, B: macA, CreatedAt: t0}); err == nil {
			t.Fatal("self friendship accepted")
		}

		must(t, r.Update(ctx, Friendship{ID: "f1", Status: 1, ConfirmedBy: "g1", A: macC}))
		f, err = r.Get(ctx, "f1")
		must(t, err)
		if f.Status != 1 || f.ConfirmedBy != "g1" || f.A != macA {
			t.Fatalf("update = %+v", f)
		}
		wantErr(t, r.Update(ctx, Friendship{ID: "nope"}), ErrNotFound)

		must(t, r.Create(ctx, Friendship{ID: "f4", A: macC, B: macB, Status: 0, CreatedAt: t0.Add(time.Second)}))
		byB, err := r.ListByDevice(ctx, macB)
		must(t, err)
		if len(byB) != 2 || byB[0].ID != "f1" || byB[1].ID != "f4" {
			t.Fatalf("ListByDevice = %+v", byB)
		}
		confirmed, err := r.ListByStatus(ctx, 1)
		must(t, err)
		if len(confirmed) != 1 || confirmed[0].ID != "f1" {
			t.Fatalf("ListByStatus = %+v", confirmed)
		}
	})
}

func TestGroups(t *testing.T) {
	run(t, func(t *testing.T, ctx context.Context, s Store) {
		r := s.Groups()
		trip := Group{ID: "group:trip", Name: "春游", Owner: macA, Code: "1234", ExpiresAt: t0.Add(time.Hour)}
		forever := Group{ID: "group:forever", Name: "好朋友", Owner: macB, Code: "5678"}
		must(t, r.Create(ctx, trip))
		must(t, r.Create(ctx, forever))
		wantErr(t, r.Create(ctx, trip), ErrConflict)

		got, err := r.ByCode(ctx, "1234", t0)
		must(t, err)
		trip.ExpiresAt = trip.ExpiresAt.Truncate(time.Millisecond)
		if got != trip {
			t.Fatalf("ByCode = %+v, want %+v", got, trip)
		}
		_, err = r.ByCode(ctx, "1234", t0.Add(time.Hour))
		wantErr(t, err, ErrNotFound)
		if _, err := r.ByCode(ctx, "5678", t0.Add(1000*time.Hour)); err != nil {
			t.Fatalf("long-term group expired: %v", err)
		}

		pa, pb := id.Device(macA), id.Device(macB)
		must(t, r.AddMember(ctx, trip.ID, Member{Participant: pa, JoinedAt: t0}))
		must(t, r.AddMember(ctx, trip.ID, Member{Participant: pa, JoinedAt: t0.Add(time.Minute)}))
		must(t, r.AddMember(ctx, trip.ID, Member{Participant: pb, JoinedAt: t0.Add(time.Second)}))
		must(t, r.AddMember(ctx, forever.ID, Member{Participant: pa, JoinedAt: t0.Add(2 * time.Second)}))
		wantErr(t, r.AddMember(ctx, "group:nope", Member{Participant: pa, JoinedAt: t0}), ErrNotFound)

		ms, err := r.Members(ctx, trip.ID)
		must(t, err)
		if len(ms) != 2 || ms[0].Participant != pa || !ms[0].JoinedAt.Equal(t0.Truncate(time.Millisecond)) {
			t.Fatalf("members = %+v", ms)
		}
		of, err := r.ListByMember(ctx, pa)
		must(t, err)
		if len(of) != 2 || of[0].ID != trip.ID || of[1].ID != forever.ID {
			t.Fatalf("ListByMember = %+v", of)
		}

		must(t, r.RemoveMember(ctx, trip.ID, pb))
		must(t, r.RemoveMember(ctx, trip.ID, pb))
		ms, err = r.Members(ctx, trip.ID)
		must(t, err)
		if len(ms) != 1 {
			t.Fatalf("members after remove = %+v", ms)
		}

		n, err := r.DeleteExpired(ctx, t0.Add(time.Hour))
		must(t, err)
		if n != 1 {
			t.Fatalf("DeleteExpired = %d", n)
		}
		_, err = r.Get(ctx, trip.ID)
		wantErr(t, err, ErrNotFound)
		ms, err = r.Members(ctx, trip.ID)
		must(t, err)
		if len(ms) != 0 {
			t.Fatalf("members of deleted group = %+v", ms)
		}

		all, err := r.List(ctx)
		must(t, err)
		if len(all) != 1 || all[0].ID != forever.ID {
			t.Fatalf("List = %+v", all)
		}
		must(t, r.Delete(ctx, forever.ID))
		wantErr(t, r.Delete(ctx, forever.ID), ErrNotFound)
		of, err = r.ListByMember(ctx, pa)
		must(t, err)
		if len(of) != 0 {
			t.Fatalf("ListByMember after delete = %+v", of)
		}
	})
}

func TestHistory(t *testing.T) {
	run(t, func(t *testing.T, ctx context.Context, s Store) {
		r := s.History()
		must(t, r.Append(ctx, HistoryEvent{ID: "e2", Device: macA, Kind: 1, Payload: []byte(`{"text":"小鸭子"}`), At: t0.Add(time.Minute)}))
		must(t, r.Append(ctx, HistoryEvent{ID: "e1", Device: macA, Kind: 0, At: t0}))
		must(t, r.Append(ctx, HistoryEvent{ID: "e3", Device: macA, Kind: 0, At: t0.Add(24 * time.Hour)}))
		must(t, r.Append(ctx, HistoryEvent{ID: "e4", Device: macB, Kind: 0, At: t0}))
		wantErr(t, r.Append(ctx, HistoryEvent{ID: "e1", Device: macB, At: t0}), ErrConflict)

		day := t0.Truncate(24 * time.Hour)
		evs, err := r.ListByDevice(ctx, macA, day, day.Add(24*time.Hour))
		must(t, err)
		if len(evs) != 2 || evs[0].ID != "e1" || evs[1].ID != "e2" {
			t.Fatalf("ListByDevice = %+v", evs)
		}
		if string(evs[0].Payload) != "{}" || string(evs[1].Payload) != `{"text":"小鸭子"}` {
			t.Fatalf("payloads = %s / %s", evs[0].Payload, evs[1].Payload)
		}
		if !evs[0].At.Equal(t0.Truncate(time.Millisecond)) {
			t.Fatalf("at = %v", evs[0].At)
		}
	})
}

func TestReopenKeepsDataAndSkipsAppliedMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "palpable.db")
	s, err := Open(ctx, path)
	must(t, err)
	must(t, s.Devices().Create(ctx, Device{ID: macA, CreatedAt: t0}))
	must(t, s.Close())

	s, err = Open(ctx, path)
	must(t, err)
	defer s.Close()
	if _, err := s.Devices().Get(ctx, macA); err != nil {
		t.Fatalf("data lost after reopen: %v", err)
	}
}
