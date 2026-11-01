package wechat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, body string, check func(r *http.Request)) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New("wxapp", "s3cret", srv.URL)
}

func TestCode2Session(t *testing.T) {
	c := serve(t, `{"openid":"o1","session_key":"k","unionid":"u1"}`, func(r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/sns/jscode2session" || q.Get("appid") != "wxapp" || q.Get("secret") != "s3cret" ||
			q.Get("js_code") != "c1" || q.Get("grant_type") != "authorization_code" {
			t.Errorf("unexpected request %s", r.URL)
		}
	})
	got, err := c.Code2Session(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Identity{OpenID: "o1", UnionID: "u1"}) {
		t.Fatalf("identity = %+v", got)
	}
}

func TestCode2SessionErrors(t *testing.T) {
	cases := []struct {
		body string
		want error
	}{
		{`{"errcode":40029,"errmsg":"invalid code"}`, ErrInvalidCode},
		{`{"errcode":40163,"errmsg":"code been used"}`, ErrInvalidCode},
		{`{"errcode":40226,"errmsg":"high risk"}`, ErrRiskyUser},
		{`{"errcode":45011,"errmsg":"freq limit"}`, ErrRateLimited},
	}
	for _, c := range cases {
		_, err := serve(t, c.body, nil).Code2Session(context.Background(), "c")
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.body, err, c.want)
		}
	}
	_, err := serve(t, `{"errcode":-1,"errmsg":"busy"}`, nil).Code2Session(context.Background(), "c")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != -1 {
		t.Fatalf("err = %v, want APIError -1", err)
	}
	if _, err := serve(t, `{}`, nil).Code2Session(context.Background(), "c"); err == nil {
		t.Fatal("empty openid accepted")
	}
}

func TestTransportErrorHidesSecret(t *testing.T) {
	c := New("wxapp", "s3cret", "http://127.0.0.1:1")
	_, err := c.Code2Session(context.Background(), "c")
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err = %v", err)
	}
}
