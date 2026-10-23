// Package share 处理画面分享：孩子让小伙伴描述眼前的画面，描述播给前台频道里的好友，
// 也播给孩子自己，并记入历史。拍照与看图由设备工具与灵矽视觉完成，这里只处理描述。
package share

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/LiusCraft/palpable/server/internal/history"
	"github.com/LiusCraft/palpable/server/internal/id"
)

const maxRunes = 200

var (
	ErrNotInChannel = errors.New("share: not in any channel")
	ErrEmpty        = errors.New("share: empty description")
)

// Announcer 让小伙伴对某台设备说一句话（bridge/linx 的 Say）。
type Announcer interface {
	Say(ctx context.Context, d id.DeviceID, text string) error
}

type Membership interface {
	ForegroundOf(p id.ParticipantID) (id.ChannelID, bool)
	Members(ch id.ChannelID) []id.ParticipantID
}

type Friends interface {
	FilterFriends(ctx context.Context, viewer id.ParticipantID, ps []id.ParticipantID) []id.ParticipantID
}

type Recorder interface {
	RecordShared(ctx context.Context, d id.DeviceID, s history.SharedScene) error
}

// Names 返回设备小伙伴的名字。
type Names func(ctx context.Context, d id.DeviceID) string

type Share struct {
	members  Membership
	friends  Friends
	announce Announcer
	history  Recorder
	names    Names
}

func New(members Membership, friends Friends, announce Announcer, rec Recorder, names Names) *Share {
	return &Share{members: members, friends: friends, announce: announce, history: rec, names: names}
}

// Publish 返回听到分享的好友人数（不含自己）。
func (s *Share) Publish(ctx context.Context, from id.DeviceID, description string) (int, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return 0, ErrEmpty
	}
	if utf8.RuneCountInString(description) > maxRunes {
		description = string([]rune(description)[:maxRunes]) + "……"
	}
	ch, ok := s.members.ForegroundOf(id.Device(from))
	if !ok {
		return 0, ErrNotInChannel
	}
	name := s.names(ctx, from)
	if name == "" {
		name = "好朋友"
	}
	text := fmt.Sprintf("%s看到：%s", name, description)

	friends := s.friends.FilterFriends(ctx, id.Device(from), s.members.Members(ch))
	targets := []id.DeviceID{from}
	for _, f := range friends {
		if d := id.OwnerDevice(f); d != from {
			targets = append(targets, d)
		}
	}
	scene := history.SharedScene{From: from, Name: name, Text: description, Channel: string(ch)}
	for _, d := range targets {
		if err := s.announce.Say(ctx, d, text); err != nil {
			slog.Warn("announce share", "device_id", d, "error", err)
		}
		if err := s.history.RecordShared(ctx, d, scene); err != nil {
			slog.Warn("record share", "device_id", d, "error", err)
		}
	}
	return len(targets) - 1, nil
}
