// Package opsapi 是 palpabled 给厂家后台（palpable-admin）开放的内部接口：登记与吊销设备公钥、恢复出厂、锁定。
// 只用一个 Bearer 令牌鉴权，且不应暴露到公网。约定见 docs/factory.md §5。
package opsapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
)

// Ops 由 app 实现。所有方法幂等。
type Ops interface {
	Enroll(ctx context.Context, d id.DeviceID, pubkey []byte) error
	Revoke(ctx context.Context, d id.DeviceID) error
	// Wipe 恢复出厂：删档案、好友、群成员、历史与声纹；设备不存在返回 nil。
	Wipe(ctx context.Context, d id.DeviceID) error
	SetLock(ctx context.Context, d id.DeviceID, locked bool) error
}

var macRE = regexp.MustCompile(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)

type API struct {
	token string
	ops   Ops
	mux   *http.ServeMux
}

// New 的 token 不能为空：没有令牌就不应该注册这组路由。
func New(token string, ops Ops) *API {
	a := &API{token: token, ops: ops, mux: http.NewServeMux()}
	a.mux.HandleFunc("PUT /internal/devices/{mac}/key", a.guard(a.enroll))
	a.mux.HandleFunc("DELETE /internal/devices/{mac}/key", a.guard(a.revoke))
	a.mux.HandleFunc("POST /internal/devices/{mac}/wipe", a.guard(a.wipe))
	a.mux.HandleFunc("PUT /internal/devices/{mac}/lock", a.guard(a.lock))
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a.mux.ServeHTTP(w, r)
}

type handler func(w http.ResponseWriter, r *http.Request, d id.DeviceID)

func (a *API) guard(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if a.token == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(a.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "bad token")
			return
		}
		mac := strings.ToLower(r.PathValue("mac"))
		if !macRE.MatchString(mac) {
			writeError(w, http.StatusBadRequest, "bad device id")
			return
		}
		h(w, r, id.DeviceID(mac))
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, device.ErrBadPublicKey):
		writeError(w, http.StatusBadRequest, "public key must be ECDSA P-256 SPKI")
	case errors.Is(err, device.ErrKeyMismatch):
		writeError(w, http.StatusConflict, "a different public key is already enrolled")
	case errors.Is(err, device.ErrRevoked):
		writeError(w, http.StatusConflict, "device is revoked")
	default:
		slog.Error("ops api", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (a *API) enroll(w http.ResponseWriter, r *http.Request, d id.DeviceID) {
	var req struct {
		PublicKey string `json:"public_key"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	der, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "public_key must be base64")
		return
	}
	if err := a.ops.Enroll(r.Context(), d, der); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) revoke(w http.ResponseWriter, r *http.Request, d id.DeviceID) {
	if err := a.ops.Revoke(r.Context(), d); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) wipe(w http.ResponseWriter, r *http.Request, d id.DeviceID) {
	if err := a.ops.Wipe(r.Context(), d); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) lock(w http.ResponseWriter, r *http.Request, d id.DeviceID) {
	var req struct {
		Locked *bool `json:"locked"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || req.Locked == nil {
		writeError(w, http.StatusBadRequest, "body must be {\"locked\": true|false}")
		return
	}
	if err := a.ops.SetLock(r.Context(), d, *req.Locked); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
