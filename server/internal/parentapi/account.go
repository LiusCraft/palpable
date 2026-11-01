package parentapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiusCraft/palpable/server/internal/account"
	"github.com/LiusCraft/palpable/server/internal/id"
)

// Accounts 是家长账号：微信登录、会话与注销。注销要先解绑名下设备，由 app 实现。
type Accounts interface {
	SignInWeChat(ctx context.Context, code string) (account.Token, error)
	SignOut(ctx context.Context, token string) error
	Profile(ctx context.Context, g id.GuardianID) (account.Guardian, error)
	SetDisplayName(ctx context.Context, g id.GuardianID, name string) error
	DeleteAccount(ctx context.Context, g id.GuardianID) error
}

// EnableAccounts 开放登录与账号接口。
func (a *API) EnableAccounts(acc Accounts) {
	a.accounts = acc
	a.mux.HandleFunc("POST /api/session", a.signIn)
	a.mux.HandleFunc("DELETE /api/session", a.guard(a.signOut))
	a.mux.HandleFunc("GET /api/me", a.guard(a.me))
	a.mux.HandleFunc("PUT /api/me", a.guard(a.putMe))
	a.mux.HandleFunc("DELETE /api/me", a.guard(a.deleteMe))
}

type Me struct {
	ID          id.GuardianID `json:"id"`
	DisplayName string        `json:"display_name"`
	CreatedAt   time.Time     `json:"created_at"`
}

func (a *API) signIn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, `body must be {"code": "<wx.login code>"}`)
		return
	}
	t, err := a.accounts.SignInWeChat(r.Context(), req.Code)
	switch {
	case errors.Is(err, account.ErrBadCode):
		writeError(w, http.StatusBadRequest, "invalid login code")
	case errors.Is(err, account.ErrRejected):
		writeError(w, http.StatusForbidden, "login rejected")
	case errors.Is(err, account.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
	case err != nil:
		internal(w, err)
	default:
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": t.Value, "expires_at": t.ExpiresAt})
	}
}

func (a *API) signOut(w http.ResponseWriter, r *http.Request, _ id.GuardianID) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := a.accounts.SignOut(r.Context(), token); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request, g id.GuardianID) {
	p, err := a.accounts.Profile(r.Context(), g)
	switch {
	case errors.Is(err, account.ErrNotFound):
		// 演示家长没有账号记录，按空称呼返回。
		writeJSON(w, Me{ID: g})
	case err != nil:
		internal(w, err)
	default:
		writeJSON(w, Me{ID: p.ID, DisplayName: p.DisplayName, CreatedAt: p.CreatedAt})
	}
}

func (a *API) putMe(w http.ResponseWriter, r *http.Request, g id.GuardianID) {
	var req struct {
		DisplayName string `json:"display_name"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `body must be {"display_name": string}`)
		return
	}
	switch err := a.accounts.SetDisplayName(r.Context(), g, req.DisplayName); {
	case errors.Is(err, account.ErrBadName):
		writeError(w, http.StatusBadRequest, "display_name must be 1-20 characters")
	case errors.Is(err, account.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internal(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteMe 注销账号：名下所有设备解绑并恢复出厂，然后删除账号与全部会话。
func (a *API) deleteMe(w http.ResponseWriter, r *http.Request, g id.GuardianID) {
	switch err := a.accounts.DeleteAccount(r.Context(), g); {
	case errors.Is(err, account.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internal(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
