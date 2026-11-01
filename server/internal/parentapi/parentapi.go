// Package parentapi 是家长端 REST：Web 手机窗口与将来的小程序调用同一套。
// 只做鉴权、归属校验、编排与序列化，业务规则在领域模块。
package parentapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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

// Auth 把令牌解析为家长；演示用固定令牌，将来换微信登录。
type Auth func(token string) (id.GuardianID, bool)

// FixedToken 是演示鉴权：一个令牌对应一个家长。
func FixedToken(token string, g id.GuardianID) Auth {
	return func(got string) (id.GuardianID, bool) {
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
}

type Friend struct {
	RequestID string      `json:"request_id"`
	DeviceID  id.DeviceID `json:"device_id"`
	Name      string      `json:"name"`
	Status    string      `json:"status"` // pending / confirmed
	// WaitingFor 在 pending 时说明在等谁：you（等你批准）或 other（你已批准或自动放行，在等对方家长）。
	WaitingFor string    `json:"waiting_for,omitempty"`
	Auto       bool      `json:"auto"` // 两家都设为"孩子自己交友"，自动成为好友，家长只是被通知
	Online     bool      `json:"online"`
	Since      time.Time `json:"since"`
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
		g, ok := a.auth(token)
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
		out = append(out, Kid{ID: p.ID, DollName: p.DollName, KidName: p.KidName, Skin: p.Skin, Stage: stageName(p.Stage), Online: a.online(p.ID)})
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
			Auto: f.ConfirmedBy == relation.AutoGuardian, Online: a.online(other), Since: f.CreatedAt})
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

// Settings 是家长对自家孩子的设置，见 docs/trust.md §1。
type Settings struct {
	FriendPolicy string `json:"friend_policy"` // approve：需我批准；notify：孩子自己交友、事后通知我
}

var friendPolicies = map[string]device.FriendPolicy{"approve": device.FriendApprove, "notify": device.FriendNotify}

func (a *API) settings(w http.ResponseWriter, _ *http.Request, _ id.GuardianID, kid device.Profile) {
	out := Settings{FriendPolicy: "approve"}
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
	if err := a.devices.SetFriendPolicy(r.Context(), kid.ID, policy); err != nil {
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
