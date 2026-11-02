// Package factory 是厂家后台的领域层：设备从出厂到报废的生命周期、事件日志、运营人员与权限。
// 规则见 docs/factory.md。本包不含任何儿童数据，对 palpabled 的调用只经 Platform 端口。
package factory

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("factory: not found")
	ErrConflict      = errors.New("factory: conflict")
	ErrForbidden     = errors.New("factory: action not allowed for this role")
	ErrBadTransition = errors.New("factory: action not allowed in this state")
	ErrLocked        = errors.New("factory: unit is locked")
	ErrInvalid       = errors.New("factory: invalid input")
	ErrNoPlatform    = errors.New("factory: platform not available")
	ErrPlatform      = errors.New("factory: platform call failed")
)

type State string

const (
	Produced     State = "produced"
	QCFailed     State = "qc_failed"
	InStock      State = "in_stock"
	InTransit    State = "in_transit"
	OnShelf      State = "on_shelf"
	Sold         State = "sold"
	Activated    State = "activated"
	Returning    State = "returning"
	Returned     State = "returned"
	Inspecting   State = "inspecting"
	Refurbishing State = "refurbishing"
	Refurbished  State = "refurbished"
	Repairing    State = "repairing"
	Scrapped     State = "scrapped"
)

// States 是展示顺序，也是合法状态的全集。
var States = []State{Produced, QCFailed, InStock, InTransit, OnShelf, Sold, Activated, Returning, Returned,
	Inspecting, Refurbishing, Refurbished, Repairing, Scrapped}

func (s State) Valid() bool {
	for _, v := range States {
		if v == s {
			return true
		}
	}
	return false
}

type Grade string

const (
	GradeNew Grade = "new"
	GradeA   Grade = "A"
	GradeB   Grade = "B"
)

type LockReason string

const (
	LockLost   LockReason = "lost"
	LockStolen LockReason = "stolen"
	LockOther  LockReason = "other"
)

// Unit 是一台设备。公钥是 palpabled 出厂清单的内容；OwnerRef 是 palpabled 对家长标识做 HMAC 后的值，
// 后台只用它判断"是不是同一个人"，无法还原。
type Unit struct {
	Serial    string
	MAC       string
	PublicKey []byte
	BatchCode string
	Skin      string
	Firmware  string

	State       State
	ResumeState State // 退货申请、维修结束或取消时回到的状态
	Grade       Grade
	RefurbCount int

	Channel       string
	OrderRef      string
	SoldAt        time.Time
	WarrantyUntil time.Time

	OwnerRef      string
	PrevOwnerRef  string
	TransferCount int

	Locked     bool
	LockReason LockReason

	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Event 是只追加的日志。Action 除了 Action 常量，还有系统事件 transferred、anomaly。
type Event struct {
	ID         int64
	Serial     string
	At         time.Time
	Actor      string
	Action     string
	From, To   State
	Note       string
	Anomaly    bool
	ResolvedBy string
	ResolvedAt time.Time
}

type Batch struct {
	Code      string
	Model     string
	Skin      string
	Firmware  string
	Note      string
	CreatedBy string
	CreatedAt time.Time
}

type BatchSummary struct {
	Batch
	Units int
}

type ChannelKind string

const (
	ChannelWarehouse   ChannelKind = "warehouse"
	ChannelOnline      ChannelKind = "online"
	ChannelOffline     ChannelKind = "offline"
	ChannelDistributor ChannelKind = "distributor"
)

func (k ChannelKind) Valid() bool {
	switch k {
	case ChannelWarehouse, ChannelOnline, ChannelOffline, ChannelDistributor:
		return true
	}
	return false
}

type Channel struct {
	Code      string
	Name      string
	Kind      ChannelKind
	CreatedAt time.Time
}

// Filter 列表筛选。Limit 为 0 取默认值。
type Filter struct {
	State   State
	Q       string // 序列号或 MAC 的片段
	Channel string
	Batch   string
	Locked  *bool
	Limit   int
	Offset  int
}

type EventFilter struct {
	Serial      string
	OpenAnomaly bool // 只要待核对的异常
	Limit       int
}

type Stats struct {
	Total         int
	ByState       map[State]int
	Locked        int
	OpenAnomalies int
}

// Repo 是设备与事件的存储端口。
type Repo interface {
	// CreateBatch 在一个事务里写批次、设备和初始事件；序列号或 MAC 重复返回 ErrConflict。
	CreateBatch(ctx context.Context, b Batch, units []Unit, events []Event) error
	Batches(ctx context.Context) ([]BatchSummary, error)
	Unit(ctx context.Context, serial string) (Unit, error)
	UnitByMAC(ctx context.Context, mac string) (Unit, error)
	// SaveUnit 要求 u.Version 等于库里的版本，成功后库里版本 +1；否则 ErrConflict。事件与设备同一事务。
	SaveUnit(ctx context.Context, u Unit, events ...Event) error
	ListUnits(ctx context.Context, f Filter) ([]Unit, int, error)
	Events(ctx context.Context, serial string) ([]Event, error)
	RecentEvents(ctx context.Context, f EventFilter) ([]Event, error)
	ResolveEvent(ctx context.Context, id int64, by string, at time.Time) error
	Stats(ctx context.Context) (Stats, error)
	Channels(ctx context.Context) ([]Channel, error)
	Channel(ctx context.Context, code string) (Channel, error)
	PutChannel(ctx context.Context, c Channel) error
}
