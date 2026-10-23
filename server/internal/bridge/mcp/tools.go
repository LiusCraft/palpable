package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/share"
	"github.com/LiusCraft/palpable/server/internal/zone"
)

// 业务工具用到的领域能力，由 app 注入。返回给 AI 的文字要儿童友好，并且不含个人信息。

type Devices interface {
	SetNames(ctx context.Context, d id.DeviceID, doll, kid string) error
	EnrollVoice(ctx context.Context, d id.DeviceID) error
	Profile(ctx context.Context, d id.DeviceID) (device.Profile, error)
}

type Zones interface {
	JoinCurrent(ctx context.Context, d id.DeviceID) error
}

type Channels interface {
	CreateGroup(ctx context.Context, owner id.DeviceID, ttl channel.GroupTTL) (string, error)
	JoinGroup(ctx context.Context, d id.DeviceID, code string) error
	Switch(ctx context.Context, d id.DeviceID, target string) error
	Current(ctx context.Context, d id.DeviceID) (channel.ChannelInfo, error)
	Members(ch id.ChannelID) []id.ParticipantID
}

type Relations interface {
	Friends(ctx context.Context, d id.DeviceID) ([]relation.Friendship, error)
	FilterFriends(ctx context.Context, viewer id.ParticipantID, ps []id.ParticipantID) []id.ParticipantID
}

type Sharer interface {
	Publish(ctx context.Context, from id.DeviceID, description string) (int, error)
}

type Services struct {
	Devices   Devices
	Zones     Zones
	Channels  Channels
	Relations Relations
	Share     Sharer // 可为 nil
}

func schema(props string, required ...string) json.RawMessage {
	// 没有必填参数时也要是空数组：JSON Schema 不接受 "required": null。
	if required == nil {
		required = []string{}
	}
	req, _ := json.Marshal(required)
	if props == "" {
		props = "{}"
	}
	return json.RawMessage(fmt.Sprintf(`{"type":"object","properties":%s,"required":%s}`, props, req))
}

func noArgs() json.RawMessage { return schema("") }

func ok(text string) (ToolResult, error)   { return ToolResult{Text: text}, nil }
func fail(text string) (ToolResult, error) { return ToolResult{Text: text, IsError: true}, nil }

// BusinessTools 返回服务端执行的业务工具。
func BusinessTools(s Services) []Tool {
	tools := baseTools(s)
	if s.Share != nil {
		tools = append(tools, shareTool(s.Share))
	}
	return tools
}

func shareTool(sh Sharer) Tool {
	return Tool{
		Name:        "self.share.publish",
		Description: "把你看到的画面用一两句话讲给频道里的好朋友听，小朋友自己也会听到。先用 self.camera.take_photo 看一看，再把简短、开心的描述传进来；描述里不要有人的长相、名字、地址、学校等信息。",
		InputSchema: schema(`{"description":{"type":"string","description":"一两句话的画面描述"}}`, "description"),
		Call: func(ctx context.Context, d id.DeviceID, args json.RawMessage) (ToolResult, error) {
			var a struct {
				Description string `json:"description"`
			}
			_ = json.Unmarshal(args, &a)
			n, err := sh.Publish(ctx, d, a.Description)
			switch {
			case errors.Is(err, share.ErrNotInChannel):
				return fail("要先和好朋友在同一个频道里，才能分享哦。")
			case errors.Is(err, share.ErrEmpty):
				return fail("先看一看画面，再说说看到了什么吧。")
			case err != nil:
				return ToolResult{}, err
			case n == 0:
				return ok("分享好啦，不过现在频道里还没有好朋友在听。")
			}
			return ok(fmt.Sprintf("分享好啦，%d 位好朋友都听到了。", n))
		},
	}
}

func baseTools(s Services) []Tool {
	return []Tool{
		{
			Name:        "self.profile.set_names",
			Description: "记住小朋友给玩偶起的名字和小朋友希望被称呼的小名。只在小朋友说出这两个名字后调用；不要记录真实全名、学校、地址等信息。",
			InputSchema: schema(`{"doll_name":{"type":"string","description":"玩偶的新名字"},"kid_name":{"type":"string","description":"小朋友的小名或昵称"}}`, "doll_name", "kid_name"),
			Call: func(ctx context.Context, d id.DeviceID, args json.RawMessage) (ToolResult, error) {
				var a struct {
					DollName string `json:"doll_name"`
					KidName  string `json:"kid_name"`
				}
				_ = json.Unmarshal(args, &a)
				if err := s.Devices.SetNames(ctx, d, a.DollName, a.KidName); errors.Is(err, device.ErrBadName) {
					return fail("名字要短一点哦，请小朋友再说一个一到十二个字的名字。")
				} else if err != nil {
					return ToolResult{}, err
				}
				return ok(fmt.Sprintf("记住啦：玩偶叫%s，小朋友叫%s。", a.DollName, a.KidName))
			},
		},
		{
			Name:        "self.voiceprint.enroll",
			Description: "在小朋友跟着说完一句话后，登记小朋友的声音，让小伙伴以后认得他。只在引导时使用；失败了就跳过，不要勉强小朋友。",
			InputSchema: noArgs(),
			Call: func(ctx context.Context, d id.DeviceID, _ json.RawMessage) (ToolResult, error) {
				switch err := s.Devices.EnrollVoice(ctx, d); {
				case err == nil:
					return ok("声音登记好啦。")
				case errors.Is(err, device.ErrNoConsent):
					return fail("爸爸妈妈还没有同意登记声音，这一步跳过。")
				case errors.Is(err, device.ErrNoSample):
					return fail("这次没有听清楚，这一步先跳过吧。")
				case errors.Is(err, device.ErrNotOnboarding):
					return fail("现在不需要登记声音。")
				default:
					return ToolResult{}, err
				}
			},
		},
		{
			Name:        "self.zone.join",
			Description: "小朋友同意后，加入当前所在区域（比如公园）的语音频道，和区域里的好朋友说话。",
			InputSchema: noArgs(),
			Call: func(ctx context.Context, d id.DeviceID, _ json.RawMessage) (ToolResult, error) {
				if err := s.Zones.JoinCurrent(ctx, d); errors.Is(err, zone.ErrNotInZone) {
					return fail("我们现在不在可以加入的区域里哦。")
				} else if err != nil {
					return ToolResult{}, err
				}
				return ok("已经加入啦！现在说话，好朋友们就能听到。")
			},
		},
		{
			Name:        "self.group.create",
			Description: "创建一个语音群聊，得到 4 位数字群码，请把群码一个数字一个数字地念给小朋友，让好朋友说'加入群聊'加上这个码。",
			InputSchema: schema(`{"ttl":{"type":"string","enum":["trip","today","forever"],"description":"有效期：trip 本次出游，today 今天，forever 长期"}}`),
			Call: func(ctx context.Context, d id.DeviceID, args json.RawMessage) (ToolResult, error) {
				var a struct {
					TTL string `json:"ttl"`
				}
				_ = json.Unmarshal(args, &a)
				ttl := map[string]channel.GroupTTL{"trip": channel.TTLTrip, "today": channel.TTLToday, "forever": channel.TTLForever}[a.TTL]
				if a.TTL == "" {
					ttl = channel.TTLToday
				}
				code, err := s.Channels.CreateGroup(ctx, d, ttl)
				if err != nil {
					return ToolResult{}, err
				}
				return ok(fmt.Sprintf("群聊建好啦，群码是 %s，请一个数字一个数字地念给小朋友。", strings.Join(strings.Split(code, ""), " ")))
			},
		},
		{
			Name:        "self.group.join",
			Description: "用 4 位数字群码加入好朋友的语音群聊。",
			InputSchema: schema(`{"code":{"type":"string","description":"4 位数字群码"}}`, "code"),
			Call: func(ctx context.Context, d id.DeviceID, args json.RawMessage) (ToolResult, error) {
				var a struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal(args, &a)
				code := strings.Map(func(r rune) rune {
					if r >= '0' && r <= '9' {
						return r
					}
					return -1
				}, a.Code)
				switch err := s.Channels.JoinGroup(ctx, d, code); {
				case err == nil:
					return ok("加入群聊啦！")
				case errors.Is(err, channel.ErrTooManyAttempts):
					return fail("试了好多次都不对，休息一分钟再试吧。")
				case errors.Is(err, channel.ErrBadCode):
					return fail("这个群码不对或者已经过期了，请再确认一下。")
				default:
					return ToolResult{}, err
				}
			},
		},
		{
			Name:        "self.group.switch",
			Description: "切换当前说话的群聊：可以说群的名字，或者 next（下一个）、prev（上一个）。",
			InputSchema: schema(`{"target":{"type":"string","description":"群名、next 或 prev"}}`, "target"),
			Call: func(ctx context.Context, d id.DeviceID, args json.RawMessage) (ToolResult, error) {
				var a struct {
					Target string `json:"target"`
				}
				_ = json.Unmarshal(args, &a)
				if err := s.Channels.Switch(ctx, d, a.Target); errors.Is(err, channel.ErrNotInChannel) {
					return fail("我们还没有加入任何群聊哦。")
				} else if errors.Is(err, channel.ErrNoSuchChannel) {
					return fail("没有找到这个群。")
				} else if err != nil {
					return ToolResult{}, err
				}
				info, err := s.Channels.Current(ctx, d)
				if err != nil {
					return ToolResult{}, err
				}
				return ok("现在在「" + info.Channel.Name + "」。")
			},
		},
		{
			Name:        "self.group.current",
			Description: "看看现在在哪个群聊，以及群里有哪些在线的好朋友。",
			InputSchema: noArgs(),
			Call: func(ctx context.Context, d id.DeviceID, _ json.RawMessage) (ToolResult, error) {
				info, err := s.Channels.Current(ctx, d)
				if errors.Is(err, channel.ErrNotInChannel) {
					return ok("现在没有在群聊里，只有我陪着你。")
				} else if err != nil {
					return ToolResult{}, err
				}
				names := friendNames(ctx, s, d, s.Relations.FilterFriends(ctx, id.Device(d), info.Members))
				if len(names) == 0 {
					return ok("现在在「" + info.Channel.Name + "」，好朋友们还没来。")
				}
				return ok("现在在「" + info.Channel.Name + "」，在的好朋友有：" + strings.Join(names, "、") + "。")
			},
		},
		{
			Name:        "self.friends.list",
			Description: "看看小朋友有哪些好朋友。",
			InputSchema: noArgs(),
			Call: func(ctx context.Context, d id.DeviceID, _ json.RawMessage) (ToolResult, error) {
				fs, err := s.Relations.Friends(ctx, d)
				if err != nil {
					return ToolResult{}, err
				}
				var ps []id.ParticipantID
				for _, f := range fs {
					if f.Status == relation.StatusConfirmed {
						ps = append(ps, id.Device(f.Other(d)))
					}
				}
				names := friendNames(ctx, s, d, ps)
				if len(names) == 0 {
					return ok("还没有好朋友哦，和其他小伙伴的玩偶贴一贴就能成为好朋友。")
				}
				return ok("好朋友有：" + strings.Join(names, "、") + "。")
			},
		},
		{
			Name:        "self.friends.nearby",
			Description: "看看当前频道里有哪些好朋友在。",
			InputSchema: noArgs(),
			Call: func(ctx context.Context, d id.DeviceID, _ json.RawMessage) (ToolResult, error) {
				info, err := s.Channels.Current(ctx, d)
				if errors.Is(err, channel.ErrNotInChannel) {
					return ok("我们现在没有在频道里。")
				} else if err != nil {
					return ToolResult{}, err
				}
				names := friendNames(ctx, s, d, s.Relations.FilterFriends(ctx, id.Device(d), s.Channels.Members(info.Channel.ID)))
				if len(names) == 0 {
					return ok("附近还没有好朋友。")
				}
				return ok("附近的好朋友有：" + strings.Join(names, "、") + "。")
			},
		},
	}
}

// friendNames 用好朋友小伙伴的名字称呼他们；没起名的叫"一位好朋友"。
func friendNames(ctx context.Context, s Services, self id.DeviceID, ps []id.ParticipantID) []string {
	var out []string
	for _, p := range ps {
		d := id.OwnerDevice(p)
		if d == self {
			continue
		}
		name := "一位好朋友"
		if prof, err := s.Devices.Profile(ctx, d); err == nil && prof.DollName != "" {
			name = prof.DollName
		}
		out = append(out, name)
	}
	return out
}
