// Package history 是只追加的事件日志，生成今日回顾：遇到哪些小朋友、分享了什么、聊了什么。
// 先展示原话，不做摘要；谁能看由 parentapi 决定。
package history

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

const (
	sessionPage = 20
	maxPages    = 10
)

type EventKind int

const (
	EventMetFriend EventKind = iota
	EventSharedScene
	EventChat // 预留：聊天现在从灵矽实时拉取，不入库
)

type Event struct {
	ID      string
	Device  id.DeviceID
	Kind    EventKind
	Payload json.RawMessage // MetFriend / SharedScene
	At      time.Time
}

type MetFriend struct {
	Friend  id.DeviceID `json:"friend"`
	Name    string      `json:"name"`
	Channel string      `json:"channel"`
	At      time.Time   `json:"at"`
}

type SharedScene struct {
	From    id.DeviceID `json:"from"`
	Name    string      `json:"name"` // 分享者小伙伴的名字
	Text    string      `json:"text"`
	Channel string      `json:"channel"`
	At      time.Time   `json:"at"`
}

type ChatLine struct {
	Role string    `json:"role"` // user / assistant
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

type Digest struct {
	Met    []MetFriend   `json:"met"`
	Shared []SharedScene `json:"shared"`
	Chats  []ChatLine    `json:"chats"`
}

// AgentOf 返回设备当前绑定的智能体，用于拉取聊天记录。
type AgentOf func(ctx context.Context, d id.DeviceID) (string, error)

type History struct {
	repo    store.HistoryRepo
	linx    linxapi.Client
	agentOf AgentOf
	now     func() time.Time
	loc     *time.Location
}

func New(repo store.HistoryRepo, linx linxapi.Client, agentOf AgentOf) *History {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &History{repo: repo, linx: linx, agentOf: agentOf, now: time.Now, loc: loc}
}

func (h *History) Record(ctx context.Context, e Event) error {
	if e.ID == "" {
		e.ID = id.New()
	}
	if e.At.IsZero() {
		e.At = h.now()
	}
	if err := h.repo.Append(ctx, store.HistoryEvent{ID: e.ID, Device: e.Device, Kind: int(e.Kind), Payload: e.Payload, At: e.At}); err != nil {
		return fmt.Errorf("record history: %w", err)
	}
	return nil
}

func (h *History) RecordMet(ctx context.Context, d id.DeviceID, m MetFriend) error {
	if m.At.IsZero() {
		m.At = h.now()
	}
	raw, _ := json.Marshal(m)
	return h.Record(ctx, Event{Device: d, Kind: EventMetFriend, Payload: raw, At: m.At})
}

func (h *History) RecordShared(ctx context.Context, d id.DeviceID, s SharedScene) error {
	if s.At.IsZero() {
		s.At = h.now()
	}
	raw, _ := json.Marshal(s)
	return h.Record(ctx, Event{Device: d, Kind: EventSharedScene, Payload: raw, At: s.At})
}

// Today 返回今天（北京时间）的回顾。同一个好朋友只出现一次。
func (h *History) Today(ctx context.Context, d id.DeviceID) (Digest, error) {
	now := h.now().In(h.loc)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.loc)
	evs, err := h.repo.ListByDevice(ctx, d, from, from.AddDate(0, 0, 1))
	if err != nil {
		return Digest{}, fmt.Errorf("today of %s: %w", d, err)
	}
	dg := Digest{Met: []MetFriend{}, Shared: []SharedScene{}, Chats: []ChatLine{}}
	seen := map[id.DeviceID]bool{}
	for _, e := range evs {
		switch EventKind(e.Kind) {
		case EventMetFriend:
			var m MetFriend
			if json.Unmarshal(e.Payload, &m) == nil && !seen[m.Friend] {
				seen[m.Friend] = true
				dg.Met = append(dg.Met, m)
			}
		case EventSharedScene:
			var s SharedScene
			if json.Unmarshal(e.Payload, &s) == nil {
				dg.Shared = append(dg.Shared, s)
			}
		}
	}
	chats, err := h.chats(ctx, d, from)
	if err != nil {
		// 聊天来自灵矽，拉不到时其余部分照常展示。
		slog.Warn("load chats", "device_id", d, "error", err)
	}
	dg.Chats = append(dg.Chats, chats...)
	return dg, nil
}

// chats 从灵矽拉今天的对话。风格智能体被多台设备共用，只保留标着本机 MAC 的消息，
// 没有 MAC 的一律不展示，避免把别的孩子的对话给家长看。
func (h *History) chats(ctx context.Context, d id.DeviceID, from time.Time) ([]ChatLine, error) {
	agent, err := h.agentOf(ctx, d)
	if err != nil || agent == "" {
		return nil, err
	}
	var out []ChatLine
	for page := 1; page <= maxPages; page++ {
		sessions, err := h.linx.Sessions(ctx, agent, page, sessionPage)
		if err != nil {
			return out, err
		}
		older := false
		for _, s := range sessions {
			if !s.CreatedAt.IsZero() && s.CreatedAt.Before(from) {
				older = true
				continue
			}
			if s.MAC != "" && s.MAC != string(d) {
				continue // 别的孩子的会话，不必拉消息
			}
			msgs, err := h.linx.SessionMessages(ctx, agent, s.ID)
			if err != nil {
				return out, err
			}
			for _, m := range msgs {
				if m.MAC != string(d) || m.Text == "" || (m.Role != "user" && m.Role != "assistant") || m.At.Before(from) {
					continue
				}
				out = append(out, ChatLine{Role: m.Role, Text: m.Text, At: m.At})
			}
		}
		if older || len(sessions) < sessionPage {
			break
		}
	}
	slices.SortFunc(out, func(a, b ChatLine) int { return a.At.Compare(b.At) })
	return out, nil
}
