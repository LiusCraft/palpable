package parentapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/history"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	kidA  id.DeviceID = "02:00:00:00:00:0a" // 家长 g1
	kidB  id.DeviceID = "02:00:00:00:00:0b" // 家长 g2
	kidC  id.DeviceID = "02:00:00:00:00:0c" // 家长 g2
	token             = "parent-token"
)

type rig struct {
	srv  *httptest.Server
	rel  *relation.Relations
	s    store.Store
	reqs map[[2]id.DeviceID]string
}

func setup(t *testing.T) *rig {
	t.Helper()
	ctx := context.Background()
	s := store.NewMemory()
	for _, k := range []struct {
		d       id.DeviceID
		g       id.GuardianID
		doll    string
		created time.Duration
	}{{kidA, "g1", "小龙", 0}, {kidB, "g2", "兔兔", time.Second}, {kidC, "g2", "小猫", 2 * time.Second}} {
		if err := s.Devices().Create(ctx, store.Device{ID: k.d, Guardian: k.g, DollName: k.doll, Stage: int(device.StageReady), CreatedAt: time.Now().Add(k.created)}); err != nil {
			t.Fatal(err)
		}
	}
	// 待确认请求：A-B（跨家长），B-C（都属 g2）。
	reqs := map[[2]id.DeviceID]string{}
	for _, p := range [][2]id.DeviceID{{kidA, kidB}, {kidB, kidC}} {
		rid := id.New()
		reqs[p] = rid
		if err := s.Friendships().Create(ctx, store.Friendship{ID: rid, A: p[0], B: p[1], Status: int(relation.StatusPending), CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	fake := linxapi.NewFake()
	cat := device.NewCatalog(s.Styles(), fake)
	devs := device.New(s.Devices(), cat, fake)
	rel, err := relation.New(ctx, s.Friendships())
	if err != nil {
		t.Fatal(err)
	}
	chans, err := channel.New(ctx, s.Groups())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chans.CreateGroup(ctx, kidA, channel.TTLToday); err != nil {
		t.Fatal(err)
	}
	hist := history.New(s.History(), fake, func(context.Context, id.DeviceID) (string, error) { return "", nil })
	_ = hist.RecordMet(ctx, kidA, history.MetFriend{Friend: kidB, Name: "兔兔"})

	api := New(FixedToken(token, "g1"), devs, rel, chans, hist, func(d id.DeviceID) bool { return d == kidB })
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return &rig{srv: srv, rel: rel, s: s, reqs: reqs}
}

func (g *rig) do(t *testing.T, method, path, tok string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, g.srv.URL+path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw json.RawMessage
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	return resp.StatusCode, raw
}

func TestAuthAndOwnership(t *testing.T) {
	g := setup(t)
	if code, _ := g.do(t, "GET", "/api/kids", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token = %d", code)
	}
	if code, _ := g.do(t, "GET", "/api/kids", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("wrong token = %d", code)
	}
	code, body := g.do(t, "GET", "/api/kids", token)
	var kids []Kid
	_ = json.Unmarshal(body, &kids)
	if code != 200 || len(kids) != 1 || kids[0].ID != kidA || kids[0].DollName != "小龙" || kids[0].Stage != "ready" {
		t.Fatalf("kids = %d %s", code, body)
	}
	// 别人家的孩子与不存在的孩子都返回 404。
	for _, path := range []string{"/api/kids/" + string(kidB) + "/friends", "/api/kids/02:00:00:00:00:99/digest", "/api/kids/" + string(kidB) + "/groups"} {
		if code, _ := g.do(t, "GET", path, token); code != http.StatusNotFound {
			t.Fatalf("%s = %d", path, code)
		}
	}
	// 与自家孩子无关的请求不能确认。
	if code, _ := g.do(t, "POST", "/api/friend-requests/"+g.reqs[[2]id.DeviceID{kidB, kidC}]+"/confirm", token); code != http.StatusNotFound {
		t.Fatalf("cross-guardian confirm = %d", code)
	}
	if code, _ := g.do(t, "DELETE", "/api/kids/"+string(kidB)+"/friends/"+string(kidC), token); code != http.StatusNotFound {
		t.Fatalf("cross-guardian remove = %d", code)
	}
}

func TestAutoConfirmedFriendIsFlagged(t *testing.T) {
	g := setup(t)
	rid := g.reqs[[2]id.DeviceID{kidA, kidB}]
	f, _ := g.s.Friendships().Get(context.Background(), rid)
	f.Status, f.ConfirmedBy = int(relation.StatusConfirmed), relation.AutoGuardian
	if err := g.s.Friendships().Update(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	_, body := g.do(t, "GET", "/api/kids/"+string(kidA)+"/friends", token)
	var fs []Friend
	_ = json.Unmarshal(body, &fs)
	if len(fs) != 1 || fs[0].Status != "confirmed" || !fs[0].Auto {
		t.Fatalf("friends = %s", body)
	}
}

func TestFriendsConfirmRemoveGroupsDigest(t *testing.T) {
	g := setup(t)
	ctx := context.Background()
	code, body := g.do(t, "GET", "/api/kids/"+string(kidA)+"/friends", token)
	var fs []Friend
	_ = json.Unmarshal(body, &fs)
	if code != 200 || len(fs) != 1 || fs[0].DeviceID != kidB || fs[0].Status != "pending" || fs[0].Name != "兔兔" || !fs[0].Online {
		t.Fatalf("friends = %d %s", code, body)
	}

	if code, _ := g.do(t, "POST", "/api/friend-requests/"+fs[0].RequestID+"/confirm", token); code != http.StatusNoContent {
		t.Fatalf("confirm = %d", code)
	}
	if ok, _ := g.rel.AreFriends(ctx, id.Device(kidA), id.Device(kidB)); !ok {
		t.Fatal("not friends after confirm")
	}
	if code, _ := g.do(t, "POST", "/api/friend-requests/"+fs[0].RequestID+"/confirm", token); code != http.StatusConflict {
		t.Fatalf("second confirm = %d", code)
	}
	f, _ := g.s.Friendships().Get(ctx, fs[0].RequestID)
	if f.ConfirmedBy != "g1" {
		t.Fatalf("confirmed_by = %q", f.ConfirmedBy)
	}

	if code, _ := g.do(t, "DELETE", "/api/kids/"+string(kidA)+"/friends/"+string(kidB), token); code != http.StatusNoContent {
		t.Fatalf("remove = %d", code)
	}
	if ok, _ := g.rel.AreFriends(ctx, id.Device(kidA), id.Device(kidB)); ok {
		t.Fatal("still friends after remove")
	}

	code, body = g.do(t, "GET", "/api/kids/"+string(kidA)+"/groups", token)
	var groups []Group
	_ = json.Unmarshal(body, &groups)
	if code != 200 || len(groups) != 1 || groups[0].Kind != "group" || len(groups[0].Code) != 4 || !groups[0].Foreground {
		t.Fatalf("groups = %d %s", code, body)
	}

	code, body = g.do(t, "GET", "/api/kids/"+string(kidA)+"/digest?date=today", token)
	var dg history.Digest
	_ = json.Unmarshal(body, &dg)
	if code != 200 || len(dg.Met) != 1 || dg.Met[0].Name != "兔兔" {
		t.Fatalf("digest = %d %s", code, body)
	}
	if code, _ := g.do(t, "GET", "/api/kids/"+string(kidA)+"/digest?date=2020-01-01", token); code != http.StatusBadRequest {
		t.Fatalf("other date = %d", code)
	}
}
