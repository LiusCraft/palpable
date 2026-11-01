// Package parentapi 是家长端 REST：微信小程序与 Web 演示手机窗口调用同一套。
// 只做鉴权、归属校验、编排与序列化，业务规则在领域模块。
package parentapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/history"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/relation"
)

type Devices interface {
	ByGuardian(ctx context.Context, g id.GuardianID) ([]device.Profile, error)
	Profile(ctx context.Context, d id.DeviceID) (device.Profile, error)
	SetFriendPolicy(ctx context.Context, d id.DeviceID, p device.FriendPolicy) error
	SetAgentInChannel(ctx context.Context, d id.DeviceID, on bool) error
	BlockAgent(ctx context.Context, listener, owner id.DeviceID, blocked bool) error
	GrantVoice(ctx context.Context, d id.DeviceID) error
	RevokeVoice(ctx context.Context, d id.DeviceID) error
}

type Relations interface {
	Friends(ctx context.Context, d id.DeviceID) ([]relation.Friendship, error)
	Request(ctx context.Context, reqID string) (relation.Friendship, error)
	Confirm(ctx context.Context, reqID string, side id.DeviceID, by id.GuardianID) error
	Remove(ctx context.Context, a, b id.DeviceID) error
}

type Channels interface {
	List(ctx context.Context, d id.DeviceID) []channel.ChannelInfo
}

type History interface {
	Today(ctx context.Context, d id.DeviceID) (history.Digest, error)
}

// Online 回答设备此刻是否在线（gateway 会话表）。
type Online func(d id.DeviceID) bool

// Auth 把令牌解析为家长：产品里是微信登录签发的会话令牌，演示里是固定令牌。
type Auth func(ctx context.Context, token string) (id.GuardianID, bool)

// FixedToken 是演示鉴权：一个令牌对应一个家长。
func FixedToken(token string, g id.GuardianID) Auth {
	return func(_ context.Context, got string) (id.GuardianID, bool) {
		if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			return "", false
		}
		return g, true
	}
}

type API struct {
	auth      Auth
	devices   Devices
	relations Relations
	channels  Channels
	history   History
	online    Online
	bindings  Bindings // nil 时不开放绑定接口
	accounts  Accounts // nil 时不开放登录与账号接口
	mux       *http.ServeMux
}

func New(auth Auth, devices Devices, relations Relations, channels Channels, hist History, online Online) *API {
	a := &API{auth: auth, devices: devices, relations: relations, channels: channels, history: hist, online: online, mux: http.NewServeMux()}
	a.mux.HandleFunc("GET /api/kids", a.guard(a.kids))
	a.mux.HandleFunc("GET /api/kids/{id}/friends", a.guard(a.kid(a.friends)))
	a.mux.HandleFunc("POST /api/friend-requests/{id}/confirm", a.guard(a.confirm))
	a.mux.HandleFunc("DELETE /api/kids/{id}/friends/{fid}", a.guard(a.kid(a.removeFriend)))
	a.mux.HandleFunc("GET /api/kids/{id}/groups", a.guard(a.kid(a.groups)))
	a.mux.HandleFunc("GET /api/kids/{id}/digest", a.guard(a.kid(a.digest)))
	a.mux.HandleFunc("GET /api/kids/{id}/settings", a.guard(a.kid(a.settings)))
	a.mux.HandleFunc("PUT /api/kids/{id}/settings", a.guard(a.kid(a.putSettings)))
	a.mux.HandleFunc("PUT /api/kids/{id}/friends/{fid}/agent", a.guard(a.kid(a.blockAgent)))
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }

// ---- 响应体 ----

type Kid struct {
	ID       id.DeviceID `json:"id"`
	DollName string      `json:"doll_name"`
	KidName  string      `json:"kid_name"`
	Skin     string      `json:"skin"`
	Stage    string      `json:"stage"`
	Online   bool        `json:"online"`
	// WaitingYou 是等这位家长批准的交友请求数，首页角标用。
	WaitingYou int `json:"waiting_you"`
}

type Friend struct {
	RequestID string      `json:"request_id"`
	DeviceID  id.DeviceID `json:"device_id"`
	Name      string      `json:"name"`
	Status    string      `json:"status"` // pending / confirmed
	// WaitingFor 在 pending 时说明在等谁：you（等你批准）或 other（你已批准或自动放行，在等对方家长）。
	WaitingFor string `json:"waiting_for,omitempty"`
	Auto       bool   `json:"auto"` // 两家都设为"孩子自己交友"，自动成为好友，家长只是被通知
	// AgentBlocked：你屏蔽了这位朋友的小伙伴，它在频道里说话时你家孩子听不到。
	AgentBlocked bool      `json:"agent_blocked"`
	Online       bool      `json:"online"`
	Since        time.Time `json:"since"`
}

type Group struct {
	ID         id.ChannelID `json:"id"`
	Kind       string       `json:"kind"` // zone / group
	Name       string       `json:"name"`
	Code       string       `json:"code,omitempty"`
	ExpiresAt  *time.Time   `json:"expires_at,omitempty"`
	Foreground bool         `json:"foreground"`
	Members    int          `json:"members"`
}

// ---- 中间件 ----

type handler func(w http.ResponseWriter, r *http.Request, g id.GuardianID)
type kidHandler func(w http.ResponseWriter, r *http.Request, g id.GuardianID, kid device.Profile)

func (a *API) guard(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		g, ok := a.auth(r.Context(), token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r, g)
	}
}

// kid 校验路径里的孩子属于这位家长；不属于时返回 404 而不是 403，避免探测设备是否存在。
func (a *API) kid(h kidHandler) handler {
	return func(w http.ResponseWriter, r *http.Request, g id.GuardianID) {
		p, err := a.devices.Profile(r.Context(), id.DeviceID(r.PathValue("id")))
		if err != nil || p.Guardian != g {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h(w, r, g, p)
	}
}

// ---- 接口 ----

func (a *API) kids(w http.ResponseWriter, r *http.Request, g id.GuardianID) {
	ps, err := a.devices.ByGuardian(r.Context(), g)
	if err != nil {
		internal(w, err)
		return
	}
	out := make([]Kid, 0, len(ps))
	for _, p := range ps {
		fs, err := a.relations.Friends(r.Context(), p.ID)
		if err != nil {
			internal(w, err)
			return
		}
		waiting := 0
		for _, f := range fs {
			if slices.Contains(f.WaitingFor(), p.ID) {
				waiting++
			}
		}
		out = append(out, Kid{ID: p.ID, DollName: p.DollName, KidName: p.KidName, Skin: p.Skin, Stage: stageName(p.Stage),
			Online: a.online(p.ID), WaitingYou: waiting})
	}
	writeJSON(w, out)
}

func (a *API) friends(w http.ResponseWriter, r *http.Request, _ id.GuardianID, kid device.Profile) {
	fs, err := a.relations.Friends(r.Context(), kid.ID)
	if err != nil {
		internal(w, err)
		return
	}
	out := make([]Friend, 0, len(fs))
	for _, f := range fs {
		other := f.Other(kid.ID)
		status := "pending"
		if f.Status == relation.StatusConfirmed {
			status = "confirmed"
		}
		name := ""
		if p, err := a.devices.Profile(r.Context(), other); err == nil {
			name = p.DollName
		}
		waiting := ""
		for _, w := range f.WaitingFor() {
			if w == kid.ID {
				waiting = "you"
			} else if waiting == "" {
				waiting = "other"
			}
		}
		out = append(out, Friend{RequestID: f.ID, DeviceID: other, Name: name, Status: status, WaitingFor: waiting,
			Auto: f.ConfirmedBy == relation.AutoGuardian, AgentBlocked: slices.Contains(kid.BlockedAgents, other),
			Online: a.online(other), Since: f.CreatedAt})
	}
	writeJSON(w, out)
}

func (a *API) confirm(w http.ResponseWriter, r *http.Request, g id.GuardianID) {
	ctx := r.Context()
	f, err := a.relations.Request(ctx, r.PathValue("id"))
	var sides []id.DeviceID
	if err == nil {
		for _, d := range []id.DeviceID{f.A, f.B} {
			if a.owns(ctx, g, d) {
				sides = append(sides, d)
			}
		}
	}
	if len(sides) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// 家长只能放行自家孩子这一侧；两个孩子都是自家的，两侧一起放行。
	for _, side := range sides {
		switch err := a.relations.Confirm(ctx, f.ID, side, g); {
		case errors.Is(err, relation.ErrNotPending):
			writeError(w, http.StatusConflict, "already handled")
			return
		case err != nil:
			internal(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) removeFriend(w http.ResponseWriter, r *http.Request, _ id.GuardianID, kid device.Profile) {
	switch err := a.relations.Remove(r.Context(), kid.ID, id.DeviceID(r.PathValue("fid"))); {
	case errors.Is(err, relation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internal(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *API) groups(w http.ResponseWriter, r *http.Request, _ id.GuardianID, kid device.Profile) {
	infos := a.channels.List(r.Context(), kid.ID)
	out := make([]Group, 0, len(infos))
	for _, info := range infos {
		g := Group{ID: info.Channel.ID, Kind: "zone", Name: info.Channel.Name, Code: info.Channel.Code, Foreground: info.Foreground, Members: len(info.Members)}
		if info.Channel.Kind == channel.KindGroup {
			g.Kind = "group"
		}
		if !info.Channel.ExpiresAt.IsZero() {
			t := info.Channel.ExpiresAt
			g.ExpiresAt = &t
		}
		out = append(out, g)
	}
	writeJSON(w, out)
}

func (a *API) digest(w http.ResponseWriter, r *http.Request, _ id.GuardianID, kid device.Profile) {
	if d := r.URL.Query().Get("date"); d != "" && d != "today" {
		writeError(w, http.StatusBadRequest, "only date=today is supported")
		return
	}
	dg, err := a.history.Today(r.Context(), kid.ID)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, dg)
}

// Settings 是家长对自家孩子的设置（PUT 请求体），见 docs/trust.md §1。
type Settings struct {
	FriendPolicy string `json:"friend_policy"` // approve：需我批准；notify：孩子自己交友、事后通知我
	// AgentInChannel：自家小伙伴被孩子点名时能否在频道里对好友说话。省略表示不改。
	AgentInChannel *bool `json:"agent_in_channel,omitempty"`
	// VoiceConsent：声音登记的单独同意。false 立即删除灵矽侧的声纹；true 只在起名前可开。省略表示不改。
	VoiceConsent *bool `json:"voice_consent,omitempty"`
}

// SettingsView 是 GET 返回的设置，多出声纹的只读状态。
type SettingsView struct {
	FriendPolicy   string `json:"friend_policy"`
	AgentInChannel bool   `json:"agent_in_channel"`
	VoiceConsent   bool   `json:"voice_consent"`
	VoiceEnrolled  bool   `json:"voice_enrolled"`   // 已经登记了声纹
	VoiceCanEnable bool   `json:"voice_can_enable"` // 现在还能补开（登记只在起名前进行）
}

var friendPolicies = map[string]device.FriendPolicy{"approve": device.FriendApprove, "notify": device.FriendNotify}

func (a *API) settings(w http.ResponseWriter, _ *http.Request, _ id.GuardianID, kid device.Profile) {
	out := SettingsView{FriendPolicy: "approve", AgentInChannel: !kid.AgentOff, VoiceConsent: kid.VoiceConsent,
		VoiceEnrolled: kid.SpeakerID != "", VoiceCanEnable: kid.Stage == device.StageFresh}
	if kid.FriendPolicy == device.FriendNotify {
		out.FriendPolicy = "notify"
	}
	writeJSON(w, out)
}

func (a *API) putSettings(w http.ResponseWriter, r *http.Request, _ id.GuardianID, kid device.Profile) {
	var s Settings
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	err := dec.Decode(&s)
	policy, ok := friendPolicies[s.FriendPolicy]
	if err != nil || !ok {
		writeError(w, http.StatusBadRequest, "friend_policy must be approve or notify")
		return
	}
	// 声纹这一步可能被拒或因灵矽不可用失败，放在最前，失败时其余设置都不改。
	if s.VoiceConsent != nil && *s.VoiceConsent != kid.VoiceConsent && !a.setVoice(w, r, kid, *s.VoiceConsent) {
		return
	}
	if err := a.devices.SetFriendPolicy(r.Context(), kid.ID, policy); err != nil {
		internal(w, err)
		return
	}
	if s.AgentInChannel != nil {
		if err := a.devices.SetAgentInChannel(r.Context(), kid.ID, *s.AgentInChannel); err != nil {
			internal(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// setVoice 开关声音登记；已写错误响应时返回 false。
func (a *API) setVoice(w http.ResponseWriter, r *http.Request, kid device.Profile, on bool) bool {
	if on {
		switch err := a.devices.GrantVoice(r.Context(), kid.ID); {
		case errors.Is(err, device.ErrNotOnboarding):
			writeError(w, http.StatusConflict, "voice enrollment only happens before the companion is named")
			return false
		case err != nil:
			internal(w, err)
			return false
		}
		return true
	}
	if err := a.devices.RevokeVoice(r.Context(), kid.ID); err != nil {
		slog.Error("parentapi revoke voice", "device_id", kid.ID, "error", err)
		writeError(w, http.StatusServiceUnavailable, "voiceprint service unavailable, try again later")
		return false
	}
	return true
}

// blockAgent 屏蔽或解除屏蔽一位朋友的小伙伴（《拟人化互动服务管理暂行办法》第十四条"屏蔽特定角色"）。
func (a *API) blockAgent(w http.ResponseWriter, r *http.Request, _ id.GuardianID, kid device.Profile) {
	var req struct {
		Blocked bool `json:"blocked"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `body must be {"blocked": bool}`)
		return
	}
	if err := a.devices.BlockAgent(r.Context(), kid.ID, id.DeviceID(r.PathValue("fid")), req.Blocked); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) owns(ctx context.Context, g id.GuardianID, d id.DeviceID) bool {
	p, err := a.devices.Profile(ctx, d)
	return err == nil && p.Guardian == g
}

func stageName(s device.Stage) string {
	switch s {
	case device.StageNamed:
		return "named"
	case device.StageReady:
		return "ready"
	}
	return "fresh"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func internal(w http.ResponseWriter, err error) {
	slog.Error("parentapi", "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
