// Package adminapi 是厂家后台的 HTTP 接口：运营人员登录、设备台账与动作、批次导入，
// 以及 palpabled 推送绑定事件的入口。业务规则都在 factory，这里只做解析、鉴权和错误映射。
package adminapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiusCraft/palpable/server/internal/factory"
)

const (
	maxBody      = 64 << 10
	maxBatchBody = 4 << 20
	maxBulk      = 500
)

type API struct {
	svc          *factory.Service
	auth         *factory.Auth
	inboundToken string
	mux          *http.ServeMux
}

type staffKey struct{}

// New 的 inboundToken 为空时 /internal/platform-events 不开放。static 非空时在 / 提供管理页面。
func New(svc *factory.Service, auth *factory.Auth, inboundToken string, static fs.FS) *API {
	a := &API{svc: svc, auth: auth, inboundToken: inboundToken, mux: http.NewServeMux()}
	a.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	a.mux.HandleFunc("POST /api/login", a.login)
	a.mux.HandleFunc("POST /api/logout", a.guard(a.logout))
	a.mux.HandleFunc("GET /api/me", a.guard(a.me))
	a.mux.HandleFunc("POST /api/me/password", a.guard(a.changePassword))

	a.mux.HandleFunc("GET /api/stats", a.guard(a.stats))
	a.mux.HandleFunc("GET /api/units", a.guard(a.listUnits))
	a.mux.HandleFunc("GET /api/units/{serial}", a.guard(a.unit))
	a.mux.HandleFunc("POST /api/units/{serial}/actions", a.guard(a.act))
	a.mux.HandleFunc("POST /api/bulk-actions", a.guard(a.bulk))
	a.mux.HandleFunc("GET /api/batches", a.guard(a.batches))
	a.mux.HandleFunc("POST /api/batches", a.guard(a.importBatch))
	a.mux.HandleFunc("GET /api/channels", a.guard(a.channels))
	a.mux.HandleFunc("PUT /api/channels/{code}", a.guard(a.putChannel))
	a.mux.HandleFunc("GET /api/events", a.guard(a.events))
	a.mux.HandleFunc("POST /api/events/{id}/resolve", a.guard(a.resolve))
	a.mux.HandleFunc("GET /api/staff", a.guard(a.staff))
	a.mux.HandleFunc("POST /api/staff", a.guard(a.createStaff))
	a.mux.HandleFunc("PATCH /api/staff/{id}", a.guard(a.updateStaff))

	if inboundToken != "" {
		a.mux.HandleFunc("POST /internal/platform-events", a.platformEvent)
	}
	if static != nil {
		a.mux.Handle("/", spa(static))
	}
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/internal/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	a.mux.ServeHTTP(w, r)
}

// spa 提供静态文件；找不到的路径回落到 index.html，让前端路由接管。
func spa(root fs.FS) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := root.Open(p); err == nil {
				_ = f.Close()
				files.ServeHTTP(w, r)
				return
			}
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		files.ServeHTTP(w, r2)
	})
}

// ---- 通用 ----

type handler func(w http.ResponseWriter, r *http.Request, who factory.Staff)

func (a *API) guard(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			writeError(w, http.StatusUnauthorized, "login required")
			return
		}
		who, err := a.auth.Authenticate(r.Context(), tok)
		if err != nil {
			a.fail(w, err)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), staffKey{}, tok)), who)
	}
}

func token(r *http.Request) string { t, _ := r.Context().Value(staffKey{}).(string); return t }

func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// classify 把领域错误映射成状态码和对外文字。500 只返回通用文字，细节记日志。
func classify(err error) (int, string) {
	switch {
	case errors.Is(err, factory.ErrInvalid):
		return http.StatusBadRequest, publicMsg(err)
	case errors.Is(err, factory.ErrUnauth), errors.Is(err, factory.ErrBadLogin):
		return http.StatusUnauthorized, "wrong credentials or not logged in"
	case errors.Is(err, factory.ErrForbidden):
		return http.StatusForbidden, "your role cannot do this"
	case errors.Is(err, factory.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, factory.ErrBadTransition):
		return http.StatusConflict, "not allowed in the current state"
	case errors.Is(err, factory.ErrLocked):
		return http.StatusConflict, "unit is locked"
	case errors.Is(err, factory.ErrConflict):
		return http.StatusConflict, "conflict: duplicate or changed by someone else, reload and retry"
	case errors.Is(err, factory.ErrTooManyTry):
		return http.StatusTooManyRequests, "too many failed logins, try again later"
	case errors.Is(err, factory.ErrNoPlatform):
		return http.StatusServiceUnavailable, "palpabled is not configured, this action cannot be done safely"
	case errors.Is(err, factory.ErrPlatform):
		slog.Error("platform call failed", "error", err)
		return http.StatusBadGateway, "palpabled did not accept the request; nothing was changed"
	}
	slog.Error("admin api", "error", err)
	return http.StatusInternalServerError, "internal error"
}

func (a *API) fail(w http.ResponseWriter, err error) {
	code, msg := classify(err)
	writeError(w, code, msg)
}

// publicMsg 取出 ErrInvalid 后面的说明文字（"factory: invalid input: xxx" → "xxx"）。
func publicMsg(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, factory.ErrInvalid.Error()+": "); i >= 0 {
		return msg[i+len(factory.ErrInvalid.Error())+2:]
	}
	return "invalid input"
}

func actor(who factory.Staff) factory.Actor { return factory.Actor{Name: who.Username, Role: who.Role} }

func msgOf(err error) string { _, m := classify(err); return m }

// ---- 视图 ----

type staffView struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at"`
}

func toStaff(s factory.Staff) staffView {
	return staffView{ID: s.ID, Username: s.Username, Role: string(s.Role), Disabled: s.Disabled, CreatedAt: rfc(s.CreatedAt)}
}

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type unitView struct {
	Serial         string   `json:"serial"`
	MAC            string   `json:"mac"`
	KeyFingerprint string   `json:"key_fingerprint"`
	Batch          string   `json:"batch"`
	Skin           string   `json:"skin"`
	Firmware       string   `json:"firmware"`
	State          string   `json:"state"`
	ResumeState    string   `json:"resume_state,omitempty"`
	Grade          string   `json:"grade"`
	RefurbCount    int      `json:"refurb_count"`
	Channel        string   `json:"channel"`
	OrderRef       string   `json:"order_ref"`
	SoldAt         string   `json:"sold_at"`
	WarrantyUntil  string   `json:"warranty_until"`
	InWarranty     bool     `json:"in_warranty"`
	HasOwner       bool     `json:"has_owner"`
	TransferCount  int      `json:"transfer_count"`
	Locked         bool     `json:"locked"`
	LockReason     string   `json:"lock_reason"`
	Version        int      `json:"version"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	Actions        []string `json:"actions"`
}

// toUnit 不返回公钥本身与 owner_ref：运营人员只需要指纹和"有没有主人"。
func (a *API) toUnit(u factory.Unit, role factory.Role) unitView {
	fp := sha256.Sum256(u.PublicKey)
	acts := []string{}
	for _, x := range factory.Available(u, role) {
		acts = append(acts, string(x))
	}
	return unitView{Serial: u.Serial, MAC: u.MAC, KeyFingerprint: hex.EncodeToString(fp[:8]), Batch: u.BatchCode, Skin: u.Skin,
		Firmware: u.Firmware, State: string(u.State), ResumeState: string(u.ResumeState), Grade: string(u.Grade),
		RefurbCount: u.RefurbCount, Channel: u.Channel, OrderRef: u.OrderRef, SoldAt: rfc(u.SoldAt), WarrantyUntil: rfc(u.WarrantyUntil),
		InWarranty: !u.WarrantyUntil.IsZero() && time.Now().Before(u.WarrantyUntil), HasOwner: u.OwnerRef != "",
		TransferCount: u.TransferCount, Locked: u.Locked, LockReason: string(u.LockReason), Version: u.Version,
		CreatedAt: rfc(u.CreatedAt), UpdatedAt: rfc(u.UpdatedAt), Actions: acts}
}

type eventView struct {
	ID         int64  `json:"id"`
	Serial     string `json:"serial"`
	At         string `json:"at"`
	Actor      string `json:"actor"`
	Action     string `json:"action"`
	From       string `json:"from"`
	To         string `json:"to"`
	Note       string `json:"note"`
	Anomaly    bool   `json:"anomaly"`
	ResolvedBy string `json:"resolved_by,omitempty"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

func toEvents(evs []factory.Event) []eventView {
	out := make([]eventView, 0, len(evs))
	for _, e := range evs {
		out = append(out, eventView{ID: e.ID, Serial: e.Serial, At: rfc(e.At), Actor: e.Actor, Action: e.Action, From: string(e.From),
			To: string(e.To), Note: e.Note, Anomaly: e.Anomaly, ResolvedBy: e.ResolvedBy, ResolvedAt: rfc(e.ResolvedAt)})
	}
	return out
}

// ---- 账号 ----

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if !decode(w, r, maxBody, &req) {
		return
	}
	tok, st, err := a.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"token": tok, "staff": toStaff(st)})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request, _ factory.Staff) {
	if err := a.auth.Logout(r.Context(), token(r)); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, _ *http.Request, who factory.Staff) {
	writeJSON(w, map[string]any{"staff": toStaff(who), "can_import": who.Role.CanImport(), "can_resolve": who.Role.CanResolve(),
		"is_admin": who.Role == factory.RoleAdmin})
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !decode(w, r, maxBody, &req) {
		return
	}
	if err := a.auth.ChangePassword(r.Context(), who, req.Old, req.New); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) staff(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	list, err := a.auth.List(r.Context(), who)
	if err != nil {
		a.fail(w, err)
		return
	}
	out := make([]staffView, 0, len(list))
	for _, s := range list {
		out = append(out, toStaff(s))
	}
	writeJSON(w, out)
}

func (a *API) createStaff(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decode(w, r, maxBody, &req) {
		return
	}
	s, err := a.auth.Create(r.Context(), who, req.Username, req.Password, factory.Role(req.Role))
	if err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, toStaff(s))
}

func (a *API) updateStaff(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Role     string `json:"role"`
		Disabled bool   `json:"disabled"`
	}
	if !decode(w, r, maxBody, &req) {
		return
	}
	if err := a.auth.Update(r.Context(), who, id, factory.Role(req.Role), req.Disabled); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- 台账 ----

func (a *API) stats(w http.ResponseWriter, r *http.Request, _ factory.Staff) {
	st, err := a.svc.Stats(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	by := map[string]int{}
	for _, s := range factory.States {
		by[string(s)] = st.ByState[s]
	}
	writeJSON(w, map[string]any{"total": st.Total, "by_state": by, "locked": st.Locked, "open_anomalies": st.OpenAnomalies})
}

func (a *API) listUnits(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	q := r.URL.Query()
	f := factory.Filter{State: factory.State(q.Get("state")), Q: q.Get("q"), Channel: q.Get("channel"), Batch: q.Get("batch")}
	if v := q.Get("locked"); v != "" {
		b := v == "1" || v == "true"
		f.Locked = &b
	}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Offset, _ = strconv.Atoi(q.Get("offset"))
	us, total, err := a.svc.List(r.Context(), f)
	if err != nil {
		a.fail(w, err)
		return
	}
	items := make([]unitView, 0, len(us))
	for _, u := range us {
		items = append(items, a.toUnit(u, who.Role))
	}
	writeJSON(w, map[string]any{"items": items, "total": total})
}

func (a *API) unit(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	u, evs, err := a.svc.Detail(r.Context(), r.PathValue("serial"))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"unit": a.toUnit(u, who.Role), "events": toEvents(evs)})
}

type paramsReq struct {
	Channel    string `json:"channel"`
	OrderRef   string `json:"order_ref"`
	Note       string `json:"note"`
	Grade      string `json:"grade"`
	LockReason string `json:"lock_reason"`
}

func (p paramsReq) params() factory.Params {
	return factory.Params{Channel: p.Channel, OrderRef: p.OrderRef, Note: p.Note, Grade: factory.Grade(p.Grade), LockReason: factory.LockReason(p.LockReason)}
}

func (a *API) act(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	var req struct {
		Action string `json:"action"`
		paramsReq
	}
	if !decode(w, r, maxBody, &req) {
		return
	}
	u, err := a.svc.Apply(r.Context(), actor(who), r.PathValue("serial"), factory.Action(req.Action), req.params())
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, a.toUnit(u, who.Role))
}

func (a *API) bulk(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	var req struct {
		Serials []string `json:"serials"`
		Action  string   `json:"action"`
		paramsReq
	}
	if !decode(w, r, maxBody*8, &req) {
		return
	}
	if len(req.Serials) == 0 || len(req.Serials) > maxBulk {
		writeError(w, http.StatusBadRequest, "serials must have 1-"+strconv.Itoa(maxBulk)+" items")
		return
	}
	type result struct {
		Serial string `json:"serial"`
		OK     bool   `json:"ok"`
		State  string `json:"state,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	out := make([]result, 0, len(req.Serials))
	seen := map[string]bool{}
	for _, s := range req.Serials {
		if seen[s] {
			continue
		}
		seen[s] = true
		u, err := a.svc.Apply(r.Context(), actor(who), s, factory.Action(req.Action), req.params())
		if err != nil {
			out = append(out, result{Serial: s, Error: msgOf(err)})
			continue
		}
		out = append(out, result{Serial: s, OK: true, State: string(u.State)})
	}
	writeJSON(w, map[string]any{"results": out})
}

func (a *API) batches(w http.ResponseWriter, r *http.Request, _ factory.Staff) {
	bs, err := a.svc.Batches(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	out := make([]map[string]any, 0, len(bs))
	for _, b := range bs {
		out = append(out, map[string]any{"code": b.Code, "model": b.Model, "skin": b.Skin, "firmware": b.Firmware, "note": b.Note,
			"created_by": b.CreatedBy, "created_at": rfc(b.CreatedAt), "units": b.Units})
	}
	writeJSON(w, out)
}

func (a *API) importBatch(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	var req struct {
		Code     string `json:"code"`
		Model    string `json:"model"`
		Skin     string `json:"skin"`
		Firmware string `json:"firmware"`
		Note     string `json:"note"`
		Units    []struct {
			MAC       string `json:"mac"`
			PublicKey string `json:"public_key"`
		} `json:"units"`
	}
	if !decode(w, r, maxBatchBody, &req) {
		return
	}
	in := factory.BatchInput{Code: req.Code, Model: req.Model, Skin: req.Skin, Firmware: req.Firmware, Note: req.Note}
	for _, u := range req.Units {
		in.Units = append(in.Units, factory.UnitKey{MAC: u.MAC, PublicKey: u.PublicKey})
	}
	us, err := a.svc.ImportBatch(r.Context(), actor(who), in)
	if err != nil {
		a.fail(w, err)
		return
	}
	serials := make([]string, 0, len(us))
	for _, u := range us {
		serials = append(serials, u.Serial)
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"code": in.Code, "count": len(us), "serials": serials})
}

func (a *API) channels(w http.ResponseWriter, r *http.Request, _ factory.Staff) {
	cs, err := a.svc.Channels(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	out := make([]map[string]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, map[string]string{"code": c.Code, "name": c.Name, "kind": string(c.Kind)})
	}
	writeJSON(w, out)
}

func (a *API) putChannel(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	var req struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if !decode(w, r, maxBody, &req) {
		return
	}
	c := factory.Channel{Code: r.PathValue("code"), Name: req.Name, Kind: factory.ChannelKind(req.Kind)}
	if err := a.svc.PutChannel(r.Context(), actor(who), c); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) events(w http.ResponseWriter, r *http.Request, _ factory.Staff) {
	q := r.URL.Query()
	f := factory.EventFilter{Serial: q.Get("serial"), OpenAnomaly: q.Get("anomaly") == "1"}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	evs, err := a.svc.Events(r.Context(), f)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, toEvents(evs))
}

func (a *API) resolve(w http.ResponseWriter, r *http.Request, who factory.Staff) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := a.svc.ResolveAnomaly(r.Context(), actor(who), id); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- palpabled 推来的事件 ----

func (a *API) platformEvent(w http.ResponseWriter, r *http.Request) {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(tok), []byte(a.inboundToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "bad token")
		return
	}
	var req struct {
		MAC      string `json:"mac"`
		Event    string `json:"event"`
		OwnerRef string `json:"owner_ref"`
	}
	if !decode(w, r, maxBody, &req) {
		return
	}
	err := a.svc.OnPlatformEvent(r.Context(), factory.PlatformEvent{MAC: req.MAC, Kind: req.Event, OwnerRef: req.OwnerRef})
	if err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
