package parentapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiusCraft/palpable/server/internal/account"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/store"
	"github.com/LiusCraft/palpable/server/internal/wechat"
)

// accountsRig 把真实的 account 模块接到 API 上；注销由 app 编排，这里只记录被注销的家长。
type accountsRig struct {
	*account.Accounts
	deleted []id.GuardianID
}

func (r *accountsRig) DeleteAccount(ctx context.Context, g id.GuardianID) error {
	r.deleted = append(r.deleted, g)
	return r.Accounts.Delete(ctx, g)
}

func setupAccounts(t *testing.T) (*httptest.Server, *accountsRig) {
	t.Helper()
	s := store.NewMemory()
	acc := account.New(s.Guardians(), s.Sessions(), wechat.Fake{})
	rig := &accountsRig{Accounts: acc}
	auth := func(ctx context.Context, tok string) (id.GuardianID, bool) { return acc.Verify(ctx, tok) }
	api := New(auth, nil, nil, nil, nil, nil)
	api.EnableAccounts(rig)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return srv, rig
}

func doJSON(t *testing.T, srv *httptest.Server, method, path, tok, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSignInMeAndSignOut(t *testing.T) {
	srv, _ := setupAccounts(t)
	for _, body := range []string{``, `{}`, `{"code":""}`, `{"code":"x","extra":1}`} {
		if code, _ := doJSON(t, srv, "POST", "/api/session", "", body); code != http.StatusBadRequest {
			t.Fatalf("sign in %q = %d", body, code)
		}
	}
	if code, _ := doJSON(t, srv, "POST", "/api/session", "", `{"code":"bad"}`); code != http.StatusBadRequest {
		t.Fatalf("bad code = %d", code)
	}
	code, out := doJSON(t, srv, "POST", "/api/session", "", `{"code":"mom"}`)
	tok, _ := out["token"].(string)
	if code != http.StatusCreated || tok == "" || out["expires_at"] == nil {
		t.Fatalf("sign in = %d %v", code, out)
	}
	if code, _ := doJSON(t, srv, "PUT", "/api/me", tok, `{"display_name":""}`); code != http.StatusBadRequest {
		t.Fatalf("empty name = %d", code)
	}
	if code, _ := doJSON(t, srv, "PUT", "/api/me", tok, `{"display_name":"朵朵妈妈"}`); code != http.StatusNoContent {
		t.Fatalf("put me = %d", code)
	}
	code, me := doJSON(t, srv, "GET", "/api/me", tok, "")
	if code != 200 || me["display_name"] != "朵朵妈妈" || me["id"] == "" {
		t.Fatalf("me = %d %v", code, me)
	}
	if code, _ := doJSON(t, srv, "DELETE", "/api/session", tok, ""); code != http.StatusNoContent {
		t.Fatalf("sign out = %d", code)
	}
	if code, _ := doJSON(t, srv, "GET", "/api/me", tok, ""); code != http.StatusUnauthorized {
		t.Fatalf("after sign out = %d", code)
	}
}

func TestDeleteAccount(t *testing.T) {
	srv, rig := setupAccounts(t)
	_, out := doJSON(t, srv, "POST", "/api/session", "", `{"code":"mom"}`)
	tok := out["token"].(string)
	_, other := doJSON(t, srv, "POST", "/api/session", "", `{"code":"mom"}`)
	if code, _ := doJSON(t, srv, "DELETE", "/api/me", tok, ""); code != http.StatusNoContent {
		t.Fatalf("delete me = %d", code)
	}
	if len(rig.deleted) != 1 {
		t.Fatalf("deleted = %v", rig.deleted)
	}
	// 注销后同一账号在其他手机上的会话也失效。
	if code, _ := doJSON(t, srv, "GET", "/api/me", other["token"].(string), ""); code != http.StatusUnauthorized {
		t.Fatalf("other session after delete = %d", code)
	}
}
