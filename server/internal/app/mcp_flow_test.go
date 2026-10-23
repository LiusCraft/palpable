package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiusCraft/palpable/server/internal/linxapi"
	"github.com/LiusCraft/palpable/server/internal/store"
)

// 设备声明 mcp 时，灵矽的 tools/list 经代理问到设备，合并业务工具后回灵矽。
func TestMCPRoundTripThroughDevice(t *testing.T) {
	f := &fakeAI{calls: make(chan string, 32)}
	a := mustApp(t, f)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	hello := strings.Replace(helloJSON, `"capabilities":["mic","speaker"]`, `"capabilities":["mic","speaker","mcp"]`, 1)
	_ = ws.WriteMessage(websocket.TextMessage, []byte(hello))
	readText(t, ws) // hello.ack
	f.wantPrefix(t, "open ")
	f.wantPrefix(t, "say ")

	go f.onMCP(mac, json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	got := readText(t, ws)
	var msg struct {
		Type    string `json:"type"`
		Payload struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(got), &msg); err != nil || msg.Type != "mcp" || msg.Payload.Method != "tools/list" {
		t.Fatalf("device got %s", got)
	}
	reply := `{"type":"mcp","payload":{"jsonrpc":"2.0","id":"` + msg.Payload.ID + `","result":{"tools":[{"name":"self.light.set","description":"灯","inputSchema":{}}]}}}`
	_ = ws.WriteMessage(websocket.TextMessage, []byte(reply))
	select {
	case got := <-f.calls:
		if !strings.HasPrefix(got, `mcp 02:00:00:00:00:01 {"jsonrpc":"2.0","id":2`) || !strings.Contains(got, `"self.light.set"`) || !strings.Contains(got, `"self.zone.join"`) {
			t.Fatalf("merged list = %s", got)
		}
		if strings.Contains(got, `"required":null`) {
			t.Fatal(`schemas must not contain "required":null`)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no reply to linx")
	}
}

// 开通失败：设备收到 error 与 muted 灯，仍保持连接。
func TestProvisioningFailureKeepsDeviceConnected(t *testing.T) {
	ctx := context.Background()
	fake := linxapi.NewFake()
	a, err := newApp(ctx, deps{store: store.NewMemory(), linx: fake})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.catalog.EnsureAll(ctx); err != nil {
		t.Fatal(err)
	}
	fake.FailPrereg = errors.New("linx down")
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	ws := connect(t, srv, mac)
	if got := readText(t, ws); !strings.Contains(got, `"provisioning failed"`) {
		t.Fatalf("error = %s", got)
	}
	if got := readText(t, ws); !strings.Contains(got, `"muted"`) {
		t.Fatalf("light = %s", got)
	}
	if _, ok := a.gw.Session(mac); !ok {
		t.Fatal("device must stay connected")
	}
	if err := a.toLinx(mac, nil); err == nil {
		t.Fatal("toLinx without ai must error")
	}
}
