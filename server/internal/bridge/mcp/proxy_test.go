package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
)

const dev id.DeviceID = "02:00:00:00:00:0a"

type wire struct {
	toLinx   chan string
	toDevice chan string
	mcp      bool
}

func newProxy(t *testing.T, mcp bool) (*Proxy, *wire) {
	t.Helper()
	w := &wire{toLinx: make(chan string, 16), toDevice: make(chan string, 16), mcp: mcp}
	p := New(
		func(_ id.DeviceID, b json.RawMessage) error { w.toLinx <- string(b); return nil },
		func(_ id.DeviceID, b json.RawMessage) error { w.toDevice <- string(b); return nil },
		func(id.DeviceID) bool { return w.mcp },
	)
	p.Register(Tool{Name: "self.zone.join", Description: "加入区域", InputSchema: noArgs(),
		Call: func(_ context.Context, d id.DeviceID, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Text: "joined " + string(d)}, nil
		}})
	p.Register(Tool{Name: "self.light.set", Description: "业务里也有同名工具", InputSchema: noArgs(),
		Call: func(context.Context, id.DeviceID, json.RawMessage) (ToolResult, error) {
			t.Error("device tool must win over business tool with the same name")
			return ToolResult{}, nil
		}})
	return p, w
}

func recv(t *testing.T, ch <-chan string) map[string]any {
	t.Helper()
	select {
	case s := <-ch:
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
		return nil
	}
}

func toolNames(t *testing.T, reply map[string]any) []string {
	t.Helper()
	var out []string
	for _, x := range reply["result"].(map[string]any)["tools"].([]any) {
		out = append(out, x.(map[string]any)["name"].(string))
	}
	return out
}

func TestListMergesAllDevicePagesWithBusinessTools(t *testing.T) {
	p, w := newProxy(t, true)
	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))

	req := recv(t, w.toDevice)
	if req["method"] != "tools/list" || !strings.HasPrefix(req["id"].(string), idPrefix) {
		t.Fatalf("first page request = %v", req)
	}
	p.OnDeviceMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":"`+req["id"].(string)+`","result":{"tools":[{"name":"self.light.set","description":"灯","inputSchema":{}}],"nextCursor":"p2"}}`))

	req = recv(t, w.toDevice)
	if req["params"].(map[string]any)["cursor"] != "p2" {
		t.Fatalf("second page request = %v", req)
	}
	p.OnDeviceMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":"`+req["id"].(string)+`","result":{"tools":[{"name":"self.camera.take_photo","description":"拍照","inputSchema":{}}],"nextCursor":""}}`))

	reply := recv(t, w.toLinx)
	if reply["id"] != float64(2) {
		t.Fatalf("reply id = %v", reply["id"])
	}
	got := strings.Join(toolNames(t, reply), ",")
	if got != "self.light.set,self.camera.take_photo,self.zone.join" {
		t.Fatalf("merged tools = %s", got)
	}
	if reply["result"].(map[string]any)["nextCursor"] != "" {
		t.Fatal("merged list must be a single page")
	}
}

func TestCallRoutesByName(t *testing.T) {
	p, w := newProxy(t, true)
	// 先列一次，记下设备自带的工具名。
	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	req := recv(t, w.toDevice)
	p.OnDeviceMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":"`+req["id"].(string)+`","result":{"tools":[{"name":"self.light.set"}]}}`))
	recv(t, w.toLinx)

	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"self.zone.join","arguments":{}}}`))
	reply := recv(t, w.toLinx)
	content := reply["result"].(map[string]any)["content"].([]any)[0].(map[string]any)
	if reply["id"] != float64(3) || content["text"] != "joined "+string(dev) {
		t.Fatalf("business call reply = %v", reply)
	}

	call := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"self.light.set","arguments":{"color":"red"}}}`
	p.OnLinxMCP(dev, json.RawMessage(call))
	if fwd := recv(t, w.toDevice); fwd["id"] != float64(4) {
		t.Fatalf("device call must be forwarded as is: %v", fwd)
	}
	p.OnDeviceMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":4,"result":{"content":[{"type":"text","text":"ok"}],"isError":false}}`))
	if back := recv(t, w.toLinx); back["id"] != float64(4) {
		t.Fatalf("device reply must go back to linx: %v", back)
	}
}

func TestDeviceWithoutMCPStillGetsBusinessTools(t *testing.T) {
	p, w := newProxy(t, false)
	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	init := recv(t, w.toLinx)
	if init["result"].(map[string]any)["protocolVersion"] != protocolVersion {
		t.Fatalf("initialize = %v", init)
	}
	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if got := strings.Join(toolNames(t, recv(t, w.toLinx)), ","); got != "self.zone.join,self.light.set" {
		t.Fatalf("tools = %s", got)
	}
	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"self.camera.take_photo"}}`))
	if e := recv(t, w.toLinx); e["error"] == nil {
		t.Fatalf("unknown tool must error: %v", e)
	}
	select {
	case s := <-w.toDevice:
		t.Fatalf("nothing should go to a device without mcp: %s", s)
	default:
	}
}

func TestInitializeForwardedToDevice(t *testing.T) {
	p, w := newProxy(t, true)
	p.OnLinxMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{"vision":{"url":"https://v","token":"t"}}}}`))
	req := recv(t, w.toDevice)
	if req["params"].(map[string]any)["capabilities"].(map[string]any)["vision"] == nil {
		t.Fatalf("vision params must reach the device: %v", req)
	}
	p.OnDeviceMCP(dev, json.RawMessage(`{"jsonrpc":"2.0","id":"`+req["id"].(string)+`","result":{"protocolVersion":"2024-11-05","serverInfo":{"name":"web-sim"}}}`))
	if r := recv(t, w.toLinx); r["id"] != float64(1) || r["result"].(map[string]any)["serverInfo"].(map[string]any)["name"] != "web-sim" {
		t.Fatalf("initialize reply = %v", r)
	}
}
