package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/zone"
)

const friend id.DeviceID = "02:00:00:00:00:0b"

type rig struct {
	tools map[string]Tool
	devs  *device.Devices
	cat   *device.Catalog
	zones *zone.Zones
	chans *channel.Channels
	fake  *linxapi.Fake
}

func newRig(t *testing.T) *rig {
	t.Helper()
	ctx := context.Background()
	s := store.NewMemory()
	fake := linxapi.NewFake()
	cat := device.NewCatalog(s.Styles(), fake)
	if err := cat.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	devs := device.New(s.Devices(), cat, fake)
	for _, d := range []id.DeviceID{dev, friend} {
		if _, err := devs.Ensure(ctx, device.DeviceInfo{MAC: string(d)}); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Friendships().Create(ctx, store.Friendship{ID: "f", A: dev, B: friend, Status: int(relation.StatusConfirmed), CreatedAt: time.Now()})
	rel, _ := relation.New(ctx, s.Friendships())
	chans, _ := channel.New(ctx, s.Groups())
	zones := zone.New(chans, zone.Park)
	g := &rig{tools: map[string]Tool{}, devs: devs, cat: cat, zones: zones, chans: chans, fake: fake}
	for _, tl := range BusinessTools(Services{Devices: devs, Zones: zones, Channels: chans, Relations: rel}) {
		g.tools[tl.Name] = tl
	}
	return g
}

func (g *rig) call(t *testing.T, d id.DeviceID, name, args string) ToolResult {
	t.Helper()
	tl, ok := g.tools[name]
	if !ok {
		t.Fatalf("no tool %s", name)
	}
	if args == "" {
		args = "{}"
	}
	res, err := tl.Call(context.Background(), d, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func TestBusinessToolsEndToEnd(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()

	// 起名：太长被拒，正常则记住。
	if r := g.call(t, dev, "self.profile.set_names", `{"doll_name":"一二三四五六七八九十十一十二十三","kid_name":"朵朵"}`); !r.IsError {
		t.Fatal("too long name accepted")
	}
	if r := g.call(t, dev, "self.profile.set_names", `{"doll_name":"小龙","kid_name":"朵朵"}`); r.IsError || !strings.Contains(r.Text, "小龙") {
		t.Fatalf("set_names = %+v", r)
	}
	_ = g.devs.SetNames(ctx, friend, "兔兔", "乐乐")

	// 声纹：没同意 → 拒绝；同意但没样本 → 跳过；引导结束 → 不需要。
	if r := g.call(t, dev, "self.voiceprint.enroll", ""); !r.IsError || !strings.Contains(r.Text, "同意") {
		t.Fatalf("enroll without consent = %+v", r)
	}
	_ = g.devs.SetVoiceConsent(ctx, dev, true)
	if r := g.call(t, dev, "self.voiceprint.enroll", ""); !r.IsError || !strings.Contains(r.Text, "听清") {
		t.Fatalf("enroll without sample = %+v", r)
	}
	g.fake.Samples[g.cat.Onboarding().AgentID] = []linxapi.VoiceSample{{AudioURL: "u", MAC: string(dev), At: time.Now()}}
	if r := g.call(t, dev, "self.voiceprint.enroll", ""); r.IsError {
		t.Fatalf("enroll = %+v", r)
	}
	if _, err := g.devs.Handoff(ctx, dev); err != nil {
		t.Fatal(err)
	}
	if r := g.call(t, dev, "self.voiceprint.enroll", ""); !r.IsError {
		t.Fatalf("enroll after onboarding = %+v", r)
	}

	// 区域：不在公园不能加入；进入后加入。
	if r := g.call(t, dev, "self.zone.join", ""); !r.IsError {
		t.Fatal("join outside park")
	}
	for _, d := range []id.DeviceID{dev, friend} {
		_ = g.zones.OnLocation(ctx, d, 31.2270, 121.4730)
	}
	if r := g.call(t, dev, "self.zone.join", ""); r.IsError {
		t.Fatalf("zone.join = %+v", r)
	}

	// 好友与在场好友。
	if r := g.call(t, dev, "self.friends.list", ""); !strings.Contains(r.Text, "兔兔") {
		t.Fatalf("friends.list = %+v", r)
	}
	if r := g.call(t, dev, "self.friends.nearby", ""); !strings.Contains(r.Text, "还没有") {
		t.Fatalf("nearby before friend joins = %+v", r)
	}
	if r := g.call(t, friend, "self.friends.nearby", ""); !strings.Contains(r.Text, "没有在频道") {
		t.Fatalf("nearby outside channel = %+v", r)
	}
	if err := g.zones.JoinCurrent(ctx, friend); err != nil {
		t.Fatal(err)
	}
	if r := g.call(t, dev, "self.friends.nearby", ""); !strings.Contains(r.Text, "兔兔") {
		t.Fatalf("nearby = %+v", r)
	}
	if r := g.call(t, dev, "self.group.current", ""); !strings.Contains(r.Text, "兔兔") {
		t.Fatalf("group.current = %+v", r)
	}

	// 群聊：建群拿码、凭码入群、切换、当前。
	r := g.call(t, dev, "self.group.create", `{"ttl":"trip"}`)
	if r.IsError || !strings.Contains(r.Text, "群码") {
		t.Fatalf("group.create = %+v", r)
	}
	info, _ := g.chans.Current(ctx, dev)
	if r := g.call(t, friend, "self.group.join", `{"code":"`+strings.Join(strings.Split(info.Channel.Code, ""), " ")+`"}`); r.IsError {
		t.Fatalf("group.join spoken digits = %+v", r)
	}
	if r := g.call(t, friend, "self.group.join", `{"code":"0000"}`); !r.IsError {
		t.Fatal("wrong code accepted")
	}
	if r := g.call(t, dev, "self.group.switch", `{"target":"next"}`); r.IsError || !strings.Contains(r.Text, "现在在") {
		t.Fatalf("group.switch = %+v", r)
	}
	if r := g.call(t, dev, "self.group.switch", `{"target":"不存在"}`); !r.IsError {
		t.Fatal("switch to unknown")
	}
	g.call(t, dev, "self.group.create", "") // 默认今天有效

	// 没加入任何频道的设备。
	const lonely id.DeviceID = "02:00:00:00:00:0c"
	if r := g.call(t, lonely, "self.group.current", ""); !strings.Contains(r.Text, "只有我") {
		t.Fatalf("current alone = %+v", r)
	}
	if r := g.call(t, lonely, "self.group.switch", `{"target":"next"}`); !r.IsError {
		t.Fatal("switch without channels")
	}
	if r := g.call(t, lonely, "self.friends.list", ""); !strings.Contains(r.Text, "还没有好朋友") {
		t.Fatalf("friends alone = %+v", r)
	}
	for range 6 {
		g.call(t, lonely, "self.group.join", `{"code":"0000"}`)
	}
	if r := g.call(t, lonely, "self.group.join", `{"code":"0000"}`); !strings.Contains(r.Text, "休息") {
		t.Fatalf("rate limited = %+v", r)
	}
}

func TestProxyEdgeCases(t *testing.T) {
	p, w := newProxy(t, true)
	// 坏 JSON 静默忽略；通知原样转给设备；未知方法转给设备。
	p.OnLinxMCP(dev, json.RawMessage(`not json`))
	p.OnDeviceMCP(dev, json.RawMessage(`not json`))
	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if got := recv(t, w.toDevice); got["method"] != "notifications/initialized" {
		t.Fatalf("notification = %v", got)
	}
	// 设备主动发的消息（不是代理请求的回复）原样回灵矽。
	p.OnDeviceMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":99,"result":{}}`))
	if got := recv(t, w.toLinx); got["id"] != float64(99) {
		t.Fatalf("passthrough = %v", got)
	}

	// 不支持 MCP 的设备：未知方法回错误。
	p2, w2 := newProxy(t, false)
	p2.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":5,"method":"resources/list"}`))
	if got := recv(t, w2.toLinx); got["error"] == nil {
		t.Fatalf("unknown method = %v", got)
	}
	p2.Forget(dev)

	// 业务工具出错时给 AI 一句儿童友好的话。
	p2.Register(Tool{Name: "self.broken", Call: func(context.Context, id.DeviceID, json.RawMessage) (ToolResult, error) {
		return ToolResult{}, context.DeadlineExceeded
	}})
	p2.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"self.broken"}}`))
	got := recv(t, w2.toLinx)
	if res := got["result"].(map[string]any); res["isError"] != true {
		t.Fatalf("broken tool = %v", got)
	}
}

func TestSchemasAreValidJSONSchema(t *testing.T) {
	for _, tl := range BusinessTools(Services{}) {
		var sch struct {
			Type     string   `json:"type"`
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(tl.InputSchema, &sch); err != nil || sch.Type != "object" || sch.Required == nil {
			t.Errorf("%s schema = %s", tl.Name, tl.InputSchema)
		}
	}
}
