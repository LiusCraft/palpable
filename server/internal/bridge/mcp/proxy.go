// Package mcp 代设备做 MCP：解析灵矽与设备之间的 JSON-RPC，把设备工具与业务工具合并成一份列表，
// 按工具名路由调用。调用身份来自连接，参数里没有设备 ID；业务规则交给领域模块。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
)

const (
	protocolVersion = "2024-11-05"
	deviceTimeout   = 5 * time.Second
	callTimeout     = 20 * time.Second
	maxPages        = 20
	idPrefix        = "palpable-"

	codeMethodNotFound = -32601
	codeInternal       = -32603
)

type Tool struct {
	Name        string // self.<模块>.<功能>
	Description string // 儿童友好、不引导说出个人信息
	InputSchema json.RawMessage
	Call        func(ctx context.Context, d id.DeviceID, args json.RawMessage) (ToolResult, error)
}

type ToolResult struct {
	Text    string
	IsError bool
}

// Ports：由 app 注入。
type (
	// ToLinx 把 JSON-RPC 发回灵矽（bridge/linx.SendMCP）。
	ToLinx func(d id.DeviceID, payload json.RawMessage) error
	// ToDevice 把 JSON-RPC 发给设备（gateway 会话的 mcp 消息）。
	ToDevice func(d id.DeviceID, payload json.RawMessage) error
	// HasMCP 设备是否在 capabilities 里声明了 mcp。
	HasMCP func(d id.DeviceID) bool
)

type rpc struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDesc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type Proxy struct {
	toLinx   ToLinx
	toDevice ToDevice
	hasMCP   HasMCP

	tools  []Tool
	byName map[string]Tool

	seq      atomic.Int64
	mu       sync.Mutex
	pending  map[string]chan rpc             // 代理发给设备的请求，等回复
	devTools map[id.DeviceID]map[string]bool // 设备自带的工具名，重名时以设备为准
}

func New(toLinx ToLinx, toDevice ToDevice, hasMCP HasMCP) *Proxy {
	return &Proxy{toLinx: toLinx, toDevice: toDevice, hasMCP: hasMCP, byName: map[string]Tool{},
		pending: map[string]chan rpc{}, devTools: map[id.DeviceID]map[string]bool{}}
}

func (p *Proxy) Register(t Tool) {
	p.tools = append(p.tools, t)
	p.byName[t.Name] = t
}

// Call 直接执行一个业务工具，供演示遥控代替孩子说出指令；与 AI 调用走同一段业务代码。
func (p *Proxy) Call(ctx context.Context, d id.DeviceID, name string, args json.RawMessage) (ToolResult, error) {
	t, ok := p.byName[name]
	if !ok {
		return ToolResult{}, fmt.Errorf("unknown business tool %q", name)
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	return t.Call(ctx, d, args)
}

// Forget 在设备断线时清理。
func (p *Proxy) Forget(d id.DeviceID) {
	p.mu.Lock()
	delete(p.devTools, d)
	p.mu.Unlock()
}

// OnDeviceMCP 设备 → 灵矽：代理自己发起的请求的回复留下，其余原样转给灵矽。
func (p *Proxy) OnDeviceMCP(d id.DeviceID, payload json.RawMessage) {
	var m rpc
	if err := json.Unmarshal(payload, &m); err != nil {
		slog.Debug("bad device mcp", "device_id", d, "error", err)
		return
	}
	if m.Method == "" && len(m.ID) > 0 {
		p.mu.Lock()
		ch, ok := p.pending[string(m.ID)]
		delete(p.pending, string(m.ID))
		p.mu.Unlock()
		if ok {
			ch <- m
			return
		}
	}
	if err := p.toLinx(d, payload); err != nil {
		slog.Debug("forward device mcp to linx", "device_id", d, "error", err)
	}
}

// OnLinxMCP 灵矽 → 设备。在会话读循环里被调用，不阻塞：需要等待的处理放到 goroutine。
func (p *Proxy) OnLinxMCP(d id.DeviceID, payload json.RawMessage) {
	var m rpc
	if err := json.Unmarshal(payload, &m); err != nil {
		slog.Debug("bad linx mcp", "device_id", d, "error", err)
		return
	}
	switch m.Method {
	case "initialize":
		go p.initialize(d, m)
	case "tools/list":
		go p.list(d, m)
	case "tools/call":
		go p.call(d, m, payload)
	default:
		if p.hasMCP(d) {
			_ = p.toDevice(d, payload)
		} else if len(m.ID) > 0 && m.Method != "" {
			p.replyError(d, m.ID, codeMethodNotFound, "method not found")
		}
	}
}

func (p *Proxy) initialize(d id.DeviceID, m rpc) {
	if p.hasMCP(d) {
		// 转给设备：视觉接口的地址与令牌在 params 里，拍照工具要用。
		if res, err := p.ask(d, "initialize", m.Params); err == nil && res.Error == nil {
			p.reply(d, m.ID, res.Result)
			return
		}
	}
	result, _ := json.Marshal(map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "palpable", "version": "1.0.0"},
	})
	p.reply(d, m.ID, result)
}

// list 取完设备的全部分页，再合并业务工具，一次回给灵矽。
func (p *Proxy) list(d id.DeviceID, m rpc) {
	var tools []toolDesc
	names := map[string]bool{}
	if p.hasMCP(d) {
		cursor := ""
		for range maxPages {
			var params json.RawMessage
			if cursor != "" {
				params, _ = json.Marshal(map[string]string{"cursor": cursor})
			}
			res, err := p.ask(d, "tools/list", params)
			if err != nil || res.Error != nil {
				slog.Warn("device tools/list", "device_id", d, "error", err)
				break
			}
			var page struct {
				Tools      []toolDesc `json:"tools"`
				NextCursor string     `json:"nextCursor"`
			}
			if err := json.Unmarshal(res.Result, &page); err != nil {
				break
			}
			for _, t := range page.Tools {
				if !names[t.Name] {
					names[t.Name] = true
					tools = append(tools, t)
				}
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
	}
	p.mu.Lock()
	p.devTools[d] = names
	p.mu.Unlock()

	for _, t := range p.tools {
		if names[t.Name] {
			slog.Warn("device tool shadows business tool", "device_id", d, "tool", t.Name)
			continue
		}
		tools = append(tools, toolDesc{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	if tools == nil {
		tools = []toolDesc{}
	}
	result, _ := json.Marshal(map[string]any{"tools": tools, "nextCursor": ""})
	p.reply(d, m.ID, result)
}

func (p *Proxy) call(d id.DeviceID, m rpc, raw json.RawMessage) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(m.Params, &params)

	p.mu.Lock()
	deviceOwns := p.devTools[d][params.Name]
	p.mu.Unlock()
	t, business := p.byName[params.Name]
	if business && !deviceOwns {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		res, err := t.Call(ctx, d, params.Arguments)
		if err != nil {
			slog.Warn("business tool failed", "device_id", d, "tool", params.Name, "error", err)
			res = ToolResult{Text: "这件事暂时没做成，等一下再试试吧。", IsError: true}
		}
		result, _ := json.Marshal(map[string]any{
			"content": []map[string]string{{"type": "text", "text": res.Text}},
			"isError": res.IsError,
		})
		p.reply(d, m.ID, result)
		return
	}
	if p.hasMCP(d) {
		// 设备工具：原样转给设备，回复经 OnDeviceMCP 原样回灵矽。
		if err := p.toDevice(d, raw); err != nil {
			p.replyError(d, m.ID, codeInternal, "device unavailable")
		}
		return
	}
	p.replyError(d, m.ID, codeMethodNotFound, "unknown tool "+params.Name)
}

// ask 代理以自己的 ID 向设备发请求并等待回复。
func (p *Proxy) ask(d id.DeviceID, method string, params json.RawMessage) (rpc, error) {
	reqID := fmt.Sprintf("%q", fmt.Sprintf("%s%d", idPrefix, p.seq.Add(1)))
	ch := make(chan rpc, 1)
	p.mu.Lock()
	p.pending[reqID] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.pending, reqID)
		p.mu.Unlock()
	}()

	req := rpc{JSONRPC: "2.0", ID: json.RawMessage(reqID), Method: method, Params: params}
	raw, _ := json.Marshal(req)
	if err := p.toDevice(d, raw); err != nil {
		return rpc{}, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(deviceTimeout):
		return rpc{}, fmt.Errorf("device %s did not answer %s", d, method)
	}
}

func (p *Proxy) reply(d id.DeviceID, reqID json.RawMessage, result json.RawMessage) {
	raw, _ := json.Marshal(rpc{JSONRPC: "2.0", ID: reqID, Result: result})
	if err := p.toLinx(d, raw); err != nil {
		slog.Debug("reply mcp to linx", "device_id", d, "error", err)
	}
}

func (p *Proxy) replyError(d id.DeviceID, reqID json.RawMessage, code int, msg string) {
	raw, _ := json.Marshal(rpc{JSONRPC: "2.0", ID: reqID, Error: &rpcError{Code: code, Message: msg}})
	_ = p.toLinx(d, raw)
}
