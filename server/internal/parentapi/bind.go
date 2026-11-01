package parentapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/LiusCraft/palpable/server/internal/device"
	"github.com/LiusCraft/palpable/server/internal/id"
)

// Bindings 是绑定与解绑，由 app 实现（解绑要同时清理好友与频道），规则见 docs/trust.md §3。
type Bindings interface {
	Claim(ctx context.Context, g id.GuardianID, code string, voiceConsent bool) (id.DeviceID, error)
	Unbind(ctx context.Context, g id.GuardianID, d id.DeviceID) error
}

// EnableBinding 开放绑定与解绑接口。
func (a *API) EnableBinding(b Bindings) {
	a.bindings = b
	a.mux.HandleFunc("POST /api/bindings", a.guard(a.bind))
	a.mux.HandleFunc("DELETE /api/kids/{id}", a.guard(a.kid(a.unbind)))
}

type bindRequest struct {
	Code string `json:"code"`
	// AICompanion 是对 AI 陪伴服务的同意（《拟人化互动服务管理暂行办法》第十四条），必须为 true。
	AICompanion bool `json:"ai_companion"`
	// VoiceConsent 是声纹登记的单独同意，可以不勾。
	VoiceConsent bool `json:"voice_consent"`
}

func (a *API) bind(w http.ResponseWriter, r *http.Request, g id.GuardianID) {
	var req bindRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || len(req.Code) != 6 {
		writeError(w, http.StatusBadRequest, "code must be 6 digits")
		return
	}
	if !req.AICompanion {
		writeError(w, http.StatusBadRequest, "guardian consent to the AI companion is required")
		return
	}
	d, err := a.bindings.Claim(r.Context(), g, req.Code, req.VoiceConsent)
	switch {
	case errors.Is(err, device.ErrBadBindCode):
		writeError(w, http.StatusBadRequest, "invalid or expired code")
	case errors.Is(err, device.ErrTooManyTries):
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
	case errors.Is(err, device.ErrAlreadyBound):
		writeError(w, http.StatusConflict, "device already bound")
	case err != nil:
		internal(w, err)
	default:
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]id.DeviceID{"id": d})
	}
}

func (a *API) unbind(w http.ResponseWriter, r *http.Request, g id.GuardianID, kid device.Profile) {
	switch err := a.bindings.Unbind(r.Context(), g, kid.ID); {
	case errors.Is(err, device.ErrNotFound), errors.Is(err, device.ErrNotYourDevice):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internal(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
