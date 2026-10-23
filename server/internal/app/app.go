// Package app 是编排层：创建各模块、组装端口，把设备消息分发到对应模块。
// 它只做"收到 X 就调用 Y"，不持有业务状态；出现业务判断说明规则放错了地方。
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/LiusCraft/palpable/server/internal/bridge/linx"
	"github.com/LiusCraft/palpable/server/internal/channel"
	"github.com/LiusCraft/palpable/server/internal/config"
	"github.com/LiusCraft/palpable/server/internal/gateway"
	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
	"github.com/LiusCraft/palpable/server/internal/relation"
	"github.com/LiusCraft/palpable/server/internal/store"
)

// ai 是 app 用到的 bridge/linx 能力。voice（S5）落地后，音频与收音改由 voice 调用。
type ai interface {
	Open(ctx context.Context, dev linx.DeviceLink, p linx.Persona) error
	Close(d id.DeviceID)
	Forward(d id.DeviceID, frame []byte)
	Listen(d id.DeviceID, on bool)
}

type App struct {
	store store.Store
	ai    ai // 未配置灵矽时为 nil
	rel   *relation.Relations
	chans *channel.Channels
	gw    *gateway.Gateway
}

var _ gateway.Handler = (*App)(nil)

// deps 是外部依赖；测试用假实现替换。
type deps struct {
	store store.Store
	ai    ai
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, err
	}
	d := deps{store: db}
	if cfg.Linx.Enabled() {
		d.ai = linx.New(linx.Config{URL: cfg.Linx.WSURL, Token: cfg.Linx.Token})
		slog.Info("linx bridge enabled")
	} else {
		slog.Warn("linx.ws_url not set, AI conversation disabled")
	}
	a, err := newApp(ctx, d)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return a, nil
}

func newApp(ctx context.Context, d deps) (*App, error) {
	rel, err := relation.New(ctx, d.store.Friendships())
	if err != nil {
		return nil, err
	}
	chans, err := channel.New(ctx, d.store.Groups())
	if err != nil {
		return nil, err
	}
	a := &App{store: d.store, ai: d.ai, rel: rel, chans: chans}
	a.gw = gateway.New(a, allowDevOrigin)
	return a, nil
}

// Start 订阅领域事件并翻译成下行消息，直到 ctx 结束。
func (a *App) Start(ctx context.Context) {
	go a.relationEvents(ctx)
	go a.channelEvents(ctx)
	go a.sweepGroups(ctx)
}

func (a *App) Close() error { return a.store.Close() }

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/ws", a.gw)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// Run 组装并运行 palpabled，直到 ctx 结束。
func Run(ctx context.Context, cfg config.Config) error {
	a, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	a.Start(ctx)

	srv := &http.Server{Addr: cfg.Addr, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("palpabled starting", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (a *App) OnConnect(s *gateway.Session) {
	if a.ai == nil {
		return
	}
	if err := a.ai.Open(s.Context(), s, persona(s)); err != nil {
		slog.Warn("open ai session", "device_id", s.DeviceID(), "error", err)
		_ = s.Send(&protocol.Error{Type: protocol.TypeError, Code: protocol.CodeInternal, Message: "ai service unavailable"})
	}
}

// persona 在 device（S6）落地前没有档案可读：不指定智能体，
// 由灵矽按平台上该 MAC 已有的绑定选择。S6 起改为 device.Ensure 的结果。
func persona(*gateway.Session) linx.Persona { return linx.Persona{} }

func (a *App) OnMessage(s *gateway.Session, msg protocol.Message) {
	switch m := msg.(type) {
	case *protocol.MicState:
		if a.ai != nil {
			a.ai.Listen(s.DeviceID(), m.State == "start")
		}
	case *protocol.Wake:
		if a.ai != nil {
			a.ai.Listen(s.DeviceID(), true)
		}
	case *protocol.Proximity:
		peers := make([]relation.Peer, 0, len(m.Peers))
		for _, p := range m.Peers {
			peers = append(peers, relation.Peer{Device: id.DeviceID(p.DeviceID), Strength: p.Strength})
		}
		if err := a.rel.OnProximity(s.Context(), s.DeviceID(), peers); err != nil {
			slog.Warn("proximity", "device_id", s.DeviceID(), "error", err)
		}
	default:
		// location → zone（S4）。
		slog.Debug("device message not handled yet", "device_id", s.DeviceID(), "type", msg.MsgType())
	}
}

func (a *App) OnAudio(s *gateway.Session, frame []byte) {
	if a.ai != nil {
		a.ai.Forward(s.DeviceID(), frame)
	}
}

func (a *App) OnDisconnect(s *gateway.Session) {
	if a.ai != nil {
		a.ai.Close(s.DeviceID())
	}
	// 区域频道成员只在线时有效；群成员身份保留。
	_ = a.chans.LeaveZone(context.WithoutCancel(s.Context()), s.DeviceID())
}

// allowDevOrigin 开发期只放行本机前端；上线前改为配置白名单。
func allowDevOrigin(r *http.Request) bool {
	switch r.Header.Get("Origin") {
	case "", "http://localhost:5173", "http://127.0.0.1:5173":
		return true
	}
	return false
}

// send 向在线设备下发消息；设备不在线时丢弃，它上线后会从档案与频道状态重新得到所需信息。
func (a *App) send(d id.DeviceID, msgs ...protocol.Message) {
	sess, ok := a.gw.Session(d)
	if !ok {
		return
	}
	for _, m := range msgs {
		if err := sess.Send(m); err != nil {
			return
		}
	}
}

// dollName 返回设备小伙伴的名字，供提示与展示；device（S6）落地前为空。
func (a *App) dollName(context.Context, id.DeviceID) string { return "" }

func (a *App) relationEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.rel.Subscribe():
			a.onRelation(ctx, e)
		}
	}
}

func (a *App) onRelation(ctx context.Context, e relation.Event) {
	f := e.Friendship
	for _, d := range []id.DeviceID{f.A, f.B} {
		other := f.Other(d)
		state := &protocol.FriendState{Type: protocol.TypeFriendState, DeviceID: other, Name: a.dollName(ctx, other)}
		switch e.Kind {
		case relation.Proposed:
			state.Event = "pending"
			a.send(d, state,
				&protocol.Haptic{Type: protocol.TypeHaptic, Pattern: "double_tap"},
				&protocol.Sound{Type: protocol.TypeSound, Name: "friend"})
		case relation.Confirmed:
			state.Event = "confirmed"
			a.send(d, state, &protocol.Haptic{Type: protocol.TypeHaptic, Pattern: "long"})
		case relation.Removed:
			state.Event = "removed"
			a.send(d, state)
		}
	}
}

func (a *App) sweepGroups(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.chans.Sweep(ctx); err != nil {
				slog.Warn("sweep groups", "error", err)
			}
		}
	}
}

func (a *App) channelEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-a.chans.Subscribe():
			a.onChannel(ctx, e)
		}
	}
}

// onChannel 告诉本设备前台频道的变化，并只让同频道的好友知道它来了或走了。
func (a *App) onChannel(ctx context.Context, e channel.Event) {
	d := id.OwnerDevice(e.Participant)
	ch := e.Channel
	state := &protocol.ChannelState{Type: protocol.TypeChannelState, Channel: string(ch.ID), Name: ch.Name, Color: channelColor(ch)}
	switch e.Kind {
	case channel.Joined:
		state.Event = "joined"
		a.send(d, state, &protocol.Sound{Type: protocol.TypeSound, Name: "joined"})
		others := a.rel.FilterFriends(ctx, e.Participant, a.chans.Members(ch.ID))
		for _, f := range others {
			fd := id.OwnerDevice(f)
			a.send(fd, a.presence("join", d, ch))
			a.send(d, a.presence("join", fd, ch)) // 新来的也知道哪些好友已经在
		}
	case channel.Left:
		state.Event = "left"
		a.send(d, state, &protocol.Sound{Type: protocol.TypeSound, Name: "left"})
		for _, f := range a.rel.FilterFriends(ctx, e.Participant, a.chans.Members(ch.ID)) {
			a.send(id.OwnerDevice(f), a.presence("leave", d, ch))
		}
	case channel.Switched:
		state.Event = "switched"
		a.send(d, state, &protocol.Sound{Type: protocol.TypeSound, Name: "switched"})
	}
}

func (a *App) presence(event string, d id.DeviceID, ch channel.Channel) *protocol.Presence {
	return &protocol.Presence{Type: protocol.TypePresence, Event: event, DeviceID: string(d), Name: a.dollName(context.Background(), d), Channel: string(ch.ID)}
}

var groupColors = []string{"#ff9f43", "#54a0ff", "#ff6b9d", "#a55eea", "#26c6da", "#feca57"}

// channelColor 给每个频道一个稳定的灯色：区域为绿色，群按 ID 取色。
func channelColor(ch channel.Channel) string {
	if ch.Kind == channel.KindZone {
		return "#4caf50"
	}
	var h uint32
	for _, r := range ch.ID {
		h = h*31 + uint32(r)
	}
	return groupColors[h%uint32(len(groupColors))]
}
