package factory

import "slices"

type Action string

const (
	ActQCPass        Action = "qc_pass"
	ActQCFail        Action = "qc_fail"
	ActRework        Action = "rework"
	ActShip          Action = "ship"
	ActReceive       Action = "receive"
	ActUnshelf       Action = "unshelf"
	ActSell          Action = "sell"
	ActReturnRequest Action = "return_request"
	ActReturnCancel  Action = "return_cancel"
	ActReceiveReturn Action = "receive_return"
	ActInspect       Action = "inspect"
	ActInspectPass   Action = "inspect_pass"
	ActRefurbish     Action = "refurbish"
	ActRefurbishDone Action = "refurbish_done"
	ActRepair        Action = "repair"
	ActRepairDone    Action = "repair_done"
	ActScrap         Action = "scrap"
	ActLock          Action = "lock"
	ActUnlock        Action = "unlock"
)

// 系统事件的名字，只出现在事件日志里。
const (
	evActivated   = "activated"
	evUnbound     = "unbound"
	evTransferred = "transferred"
	evAnomaly     = "anomaly"
	evResolved    = "resolved"
	evImported    = "imported"
)

// 动作需要的输入，rule.need 按位组合。
type need uint8

const (
	needChannel need = 1 << iota
	needOrder
	needNote
	needGrade
	needLockReason
)

// rule 描述一个动作：从哪些状态出发、到哪、需要什么输入、有什么副作用。
// to 为 nil 表示状态不变（锁定类动作）；to 返回空串表示回到 ResumeState。
type rule struct {
	from   []State
	to     func(Unit) State
	need   need
	wipe   bool // 进入可再销售状态前，必须先擦除 palpabled 里的档案
	revoke bool // 吊销公钥
}

func to(s State) func(Unit) State { return func(Unit) State { return s } }

// resume 回到进入前的状态。
func resume(Unit) State { return "" }

// 下架：新品回到 in_stock，翻新品回到 refurbished。
func backToStock(u Unit) State {
	if u.Grade == GradeNew || u.Grade == "" {
		return InStock
	}
	return Refurbished
}

var anyState = func() []State {
	var out []State
	for _, s := range States {
		if s != Scrapped {
			out = append(out, s)
		}
	}
	return out
}()

var rules = map[Action]rule{
	ActQCPass:        {from: []State{Produced}, to: to(InStock), wipe: true},
	ActQCFail:        {from: []State{Produced}, to: to(QCFailed), need: needNote},
	ActRework:        {from: []State{QCFailed}, to: to(Produced)},
	ActShip:          {from: []State{InStock, Refurbished}, to: to(InTransit), need: needChannel},
	ActReceive:       {from: []State{InTransit}, to: to(OnShelf)},
	ActUnshelf:       {from: []State{OnShelf}, to: backToStock},
	ActSell:          {from: []State{OnShelf}, to: to(Sold), need: needOrder},
	ActReturnRequest: {from: []State{Sold, Activated}, to: to(Returning), need: needNote},
	ActReturnCancel:  {from: []State{Returning}, to: resume},
	ActReceiveReturn: {from: []State{Returning}, to: to(Returned), wipe: true},
	ActInspect:       {from: []State{Returned}, to: to(Inspecting)},
	ActInspectPass:   {from: []State{Inspecting}, to: to(Refurbished), need: needGrade, wipe: true},
	ActRefurbish:     {from: []State{Inspecting}, to: to(Refurbishing)},
	ActRefurbishDone: {from: []State{Refurbishing}, to: to(Refurbished), need: needGrade, wipe: true},
	ActRepair:        {from: []State{Sold, Activated}, to: to(Repairing), need: needNote},
	ActRepairDone:    {from: []State{Repairing}, to: resume},
	ActScrap:         {from: []State{QCFailed, Returned, Inspecting, Refurbishing, Repairing}, to: to(Scrapped), need: needNote, revoke: true},
	ActLock:          {from: anyState, need: needLockReason},
	ActUnlock:        {from: anyState},
}

type Role string

const (
	RoleAdmin     Role = "admin"
	RoleWarehouse Role = "warehouse"
	RoleSales     Role = "sales"
	RoleSupport   Role = "support"
	RoleViewer    Role = "viewer"
)

var Roles = []Role{RoleAdmin, RoleWarehouse, RoleSales, RoleSupport, RoleViewer}

func (r Role) Valid() bool { return slices.Contains(Roles, r) }

var rolePerms = map[Role][]Action{
	RoleWarehouse: {ActQCPass, ActQCFail, ActRework, ActShip, ActReceive, ActUnshelf, ActReceiveReturn,
		ActInspect, ActInspectPass, ActRefurbish, ActRefurbishDone, ActScrap},
	RoleSales:   {ActReceive, ActUnshelf, ActSell},
	RoleSupport: {ActReturnRequest, ActReturnCancel, ActRepair, ActRepairDone, ActLock, ActUnlock},
}

// Can 报告这个角色能否执行该动作。admin 可以执行全部动作。
func (r Role) Can(a Action) bool {
	if r == RoleAdmin {
		_, ok := rules[a]
		return ok
	}
	return slices.Contains(rolePerms[r], a)
}

// CanImport 导入批次：admin 与 warehouse。
func (r Role) CanImport() bool { return r == RoleAdmin || r == RoleWarehouse }

// CanResolve 核对异常：admin、sales、support。
func (r Role) CanResolve() bool { return r == RoleAdmin || r == RoleSales || r == RoleSupport }

// Available 列出这台设备此刻、这个角色可以做的动作，供界面显示按钮；执行时仍会再校验。
func Available(u Unit, role Role) []Action {
	var out []Action
	for _, a := range actionOrder {
		if !role.Can(a) || !allowed(u, a) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// actionOrder 固定展示顺序（map 无序）。
var actionOrder = []Action{ActQCPass, ActQCFail, ActRework, ActShip, ActReceive, ActUnshelf, ActSell,
	ActReturnRequest, ActReturnCancel, ActReceiveReturn, ActInspect, ActInspectPass, ActRefurbish,
	ActRefurbishDone, ActRepair, ActRepairDone, ActScrap, ActLock, ActUnlock}

// allowed 检查状态与锁定，不检查角色与输入。
func allowed(u Unit, a Action) bool {
	r, ok := rules[a]
	if !ok || !slices.Contains(r.from, u.State) {
		return false
	}
	switch a {
	case ActLock:
		return !u.Locked
	case ActUnlock:
		return u.Locked
	case ActScrap:
		return true // 锁定的设备也可以报废
	}
	return !u.Locked
}
