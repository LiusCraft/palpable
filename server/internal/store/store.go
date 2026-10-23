// Package store 是持久化层：每个领域模块一个仓储接口，SQLite 实现与内存假实现。
//
// 这里只存取记录，不含业务规则；状态、阶段等枚举以整数存放，含义由所属领域模块定义。
// 声纹特征与原始音频不入库，记忆本体在灵矽，也不在这里重复保存。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/LiusCraft/palpable/server/internal/id"
)

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: already exists")
)

// Store 汇总各模块的仓储；业务模块只依赖自己那一个接口。
type Store interface {
	Devices() DeviceRepo
	Styles() StyleRepo
	Guardians() GuardianRepo
	Friendships() FriendshipRepo
	Groups() GroupRepo
	History() HistoryRepo
	Close() error
}

type Device struct {
	ID           id.DeviceID
	Skin         string
	StyleID      string
	AgentID      string
	DollName     string
	KidName      string
	Stage        int
	Guardian     id.GuardianID
	VoiceConsent bool
	SpeakerID    string
	CreatedAt    time.Time
}

type DeviceRepo interface {
	Get(ctx context.Context, d id.DeviceID) (Device, error)
	Create(ctx context.Context, dev Device) error // 已存在返回 ErrConflict，供并发开通时判重
	Update(ctx context.Context, dev Device) error // 不存在返回 ErrNotFound；CreatedAt 不变
	// Delete 在同一事务里删除设备及其好友关系、群成员身份与历史。
	Delete(ctx context.Context, d id.DeviceID) error
	ListByGuardian(ctx context.Context, g id.GuardianID) ([]Device, error) // 按建档时间升序
}

// Style 记录风格到灵矽智能体的映射；引导智能体的 ID 为 "welcome"，Skin 为空。
type Style struct {
	ID      string
	Skin    string
	AgentID string
}

type StyleRepo interface {
	Get(ctx context.Context, styleID string) (Style, error)
	Put(ctx context.Context, s Style) error // 存在则覆盖
	List(ctx context.Context) ([]Style, error)
}

type Guardian struct {
	ID          id.GuardianID
	DisplayName string
}

type GuardianRepo interface {
	Get(ctx context.Context, g id.GuardianID) (Guardian, error)
	Put(ctx context.Context, g Guardian) error // 存在则覆盖
}

// Friendship 的 A、B 由仓储规范为 A < B，同一对设备只有一条记录。
type Friendship struct {
	ID          string
	A, B        id.DeviceID
	Status      int
	ConfirmedBy id.GuardianID
	CreatedAt   time.Time
}

type FriendshipRepo interface {
	Get(ctx context.Context, friendshipID string) (Friendship, error)
	ByPair(ctx context.Context, a, b id.DeviceID) (Friendship, error) // 参数顺序无关
	Create(ctx context.Context, f Friendship) error                   // ID 或这一对已存在返回 ErrConflict
	Update(ctx context.Context, f Friendship) error                   // 只改 Status 与 ConfirmedBy
	ListByDevice(ctx context.Context, d id.DeviceID) ([]Friendship, error)
	ListByStatus(ctx context.Context, status int) ([]Friendship, error) // 启动时装载好友缓存
}

// Group 是持久化的群频道；区域频道成员只在内存，不经过这里。
type Group struct {
	ID        id.ChannelID
	Name      string
	Owner     id.DeviceID
	Code      string
	ExpiresAt time.Time // 零值表示长期
}

func (g Group) Active(now time.Time) bool { return g.ExpiresAt.IsZero() || now.Before(g.ExpiresAt) }

type Member struct {
	Participant id.ParticipantID
	JoinedAt    time.Time
}

type GroupRepo interface {
	Create(ctx context.Context, g Group) error // ID 已存在返回 ErrConflict
	Get(ctx context.Context, ch id.ChannelID) (Group, error)
	// ByCode 返回在 now 时仍有效、使用该码的群；码的唯一性由 channel 保证。
	ByCode(ctx context.Context, code string, now time.Time) (Group, error)
	List(ctx context.Context) ([]Group, error)
	Delete(ctx context.Context, ch id.ChannelID) error // 连同成员
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
	AddMember(ctx context.Context, ch id.ChannelID, m Member) error // 已是成员时不改 JoinedAt；群不存在返回 ErrNotFound
	RemoveMember(ctx context.Context, ch id.ChannelID, p id.ParticipantID) error
	Members(ctx context.Context, ch id.ChannelID) ([]Member, error)
	ListByMember(ctx context.Context, p id.ParticipantID) ([]Group, error)
}

type HistoryEvent struct {
	ID      string
	Device  id.DeviceID
	Kind    int
	Payload json.RawMessage
	At      time.Time
}

type HistoryRepo interface {
	Append(ctx context.Context, e HistoryEvent) error // ID 已存在返回 ErrConflict
	// ListByDevice 返回 [from, to) 内的事件，按时间升序。
	ListByDevice(ctx context.Context, d id.DeviceID, from, to time.Time) ([]HistoryEvent, error)
}

var errSelfFriendship = errors.New("store: a device cannot befriend itself")

func orderPair(a, b id.DeviceID) (id.DeviceID, id.DeviceID) {
	if a > b {
		return b, a
	}
	return a, b
}

// 库里时间精度为毫秒；两种实现都经这两个函数换算，读出的值才一致。
func toMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func roundMillis(t time.Time) time.Time { return fromMillis(toMillis(t)) }
