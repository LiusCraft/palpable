package adminhook_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/adminhook"
)

type sink struct {
	mu     sync.Mutex
	got    []adminhook.Event
	status []int // 依次返回的状态码，用完后返回 204
	auth   string
	calls  int
}

func (s *sink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.auth = r.Header.Get("Authorization")
	if len(s.status) > 0 {
		code := s.status[0]
		s.status = s.status[1:]
		if code >= 300 {
			w.WriteHeader(code)
			return
		}
	}
	var e adminhook.Event
	_ = json.NewDecoder(r.Body).Decode(&e)
	s.got = append(s.got, e)
	w.WriteHeader(http.StatusNoContent)
}

func (s *sink) events() []adminhook.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]adminhook.Event(nil), s.got...)
}

func wait(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func start(t *testing.T, s *sink) *adminhook.Client {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	c := adminhook.New(srv.URL+"/", "tok")
	c.FastRetry()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go c.Run(ctx)
	return c
}

func TestDeliversInOrderWithToken(t *testing.T) {
	s := &sink{}
	c := start(t, s)
	c.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:01", Event: "activated", OwnerRef: "ref-1"})
	c.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:01", Event: "unbound"})
	wait(t, func() bool { return len(s.events()) == 2 })
	got := s.events()
	if got[0].Event != "activated" || got[0].OwnerRef != "ref-1" || got[1].Event != "unbound" {
		t.Fatalf("order/content: %+v", got)
	}
	if s.auth != "Bearer tok" {
		t.Fatalf("auth header %q", s.auth)
	}
}

func TestRetriesServerErrorsButNotRejections(t *testing.T) {
	s := &sink{status: []int{503, 500, 429}}
	c := start(t, s)
	c.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:01", Event: "activated", OwnerRef: "r"})
	wait(t, func() bool { return len(s.events()) == 1 })
	if s.calls != 4 {
		t.Fatalf("calls = %d, want 3 failures + 1 success", s.calls)
	}

	s2 := &sink{status: []int{404}}
	c2 := start(t, s2)
	c2.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:02", Event: "unbound"})
	c2.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:03", Event: "unbound"})
	wait(t, func() bool { return len(s2.events()) == 1 })
	if s2.events()[0].MAC != "aa:bb:cc:dd:ee:03" || s2.calls != 2 {
		t.Fatalf("a rejected event must be dropped without retry: calls=%d %+v", s2.calls, s2.events())
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	s := &sink{status: []int{500, 500, 500, 500, 500, 500}}
	c := start(t, s)
	c.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:01", Event: "unbound"})
	c.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:02", Event: "unbound"})
	wait(t, func() bool { return len(s.events()) == 1 })
	if s.events()[0].MAC != "aa:bb:cc:dd:ee:02" {
		t.Fatalf("after giving up the next event should still be sent: %+v", s.events())
	}
}

func TestUnreachableAndStop(t *testing.T) {
	c := adminhook.New("http://127.0.0.1:1", "tok")
	c.FastRetry()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	c.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:01", Event: "unbound"})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestNotifyNeverBlocksWhenFull(t *testing.T) {
	c := adminhook.New("http://127.0.0.1:1", "tok") // 没有 Run，队列只进不出
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			c.Notify(adminhook.Event{MAC: "aa:bb:cc:dd:ee:01", Event: "unbound"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Notify blocked on a full queue")
	}
}
