import type { ActionName, Channel, Role, State } from './api'

export const STATE_LABEL: Record<State, string> = {
  produced: '已生产',
  qc_failed: '质检不合格',
  in_stock: '在库',
  in_transit: '在途',
  on_shelf: '已上架',
  sold: '已售出',
  activated: '使用中',
  returning: '退货中',
  returned: '已收退货',
  inspecting: '检测中',
  refurbishing: '翻新中',
  refurbished: '翻新品在库',
  repairing: '维修中',
  scrapped: '已报废',
}

// 概览页的漏斗顺序：正向流程在前，售后与报废在后。
export const STATE_ORDER: State[] = [
  'produced', 'in_stock', 'in_transit', 'on_shelf', 'sold', 'activated',
  'returning', 'returned', 'inspecting', 'refurbishing', 'refurbished', 'repairing', 'qc_failed', 'scrapped',
]

export const ACTION_LABEL: Record<ActionName, string> = {
  qc_pass: '质检通过',
  qc_fail: '质检不合格',
  rework: '返修完成',
  ship: '发货',
  receive: '渠道收货',
  unshelf: '下架',
  sell: '登记售出',
  return_request: '申请退货',
  return_cancel: '取消退货',
  receive_return: '收到退货',
  inspect: '开始检测',
  inspect_pass: '检测通过（无需翻新）',
  refurbish: '开始翻新',
  refurbish_done: '翻新完成',
  repair: '送修',
  repair_done: '修好寄回',
  scrap: '报废',
  lock: '挂失锁定',
  unlock: '解锁',
}

// 日志里除了动作，还有 palpabled 推来的系统事件。
export const EVENT_LABEL: Record<string, string> = {
  ...ACTION_LABEL,
  imported: '批次入库',
  activated: '家长绑定',
  unbound: '家长解绑',
  transferred: '换主人（二手）',
}

export const ACTION_HINT: Partial<Record<ActionName, string>> = {
  qc_pass: '会先清除产线测试遗留的绑定，再入库。',
  receive_return: '会清除服务端上这台设备的档案、好友、历史和声纹，并清空主人记录。清除失败则不会收货。',
  inspect_pass: '会再次清除服务端数据后才能作为翻新品入库。',
  refurbish_done: '会再次清除服务端数据后才能作为翻新品入库。',
  scrap: '会吊销这台设备的密钥，它再也连不上服务。不可撤销。',
  lock: '设备立刻被拒绝服务，直到解锁。',
  unlock: '设备恢复服务。',
}

export const DANGEROUS: ActionName[] = ['scrap', 'receive_return', 'lock']

export interface Need { channel?: boolean; order?: boolean; note?: boolean; noteOptional?: boolean; grade?: boolean; lockReason?: boolean }

// 只用来决定表单显示哪些输入框；真正的校验在服务端。
export const NEEDS: Partial<Record<ActionName, Need>> = {
  ship: { channel: true },
  sell: { order: true },
  qc_fail: { note: true },
  return_request: { note: true },
  repair: { note: true },
  scrap: { note: true },
  inspect_pass: { grade: true },
  refurbish_done: { grade: true },
  lock: { lockReason: true, noteOptional: true },
}

export const NOTE_PLACEHOLDER: Partial<Record<ActionName, string>> = {
  qc_fail: '例如：喇叭无声',
  return_request: '例如：七天无理由',
  repair: '例如：按键失灵',
  scrap: '例如：进水无法修复',
}

export const ROLE_LABEL: Record<Role, string> = {
  admin: '管理员',
  warehouse: '仓库',
  sales: '销售',
  support: '客服',
  viewer: '只读',
}

export const GRADE_LABEL: Record<string, string> = { new: '全新', A: '翻新 A（近新）', B: '翻新 B（轻微使用痕迹）' }
export const LOCK_LABEL: Record<string, string> = { lost: '丢失', stolen: '被盗', other: '其他' }
export const KIND_LABEL: Record<Channel['kind'], string> = { warehouse: '仓库', online: '线上', offline: '线下门店', distributor: '经销商' }

export function fmtTime(s: string | undefined): string {
  if (!s) return '—'
  return new Date(s).toLocaleString('zh-CN', { hour12: false })
}

export function fmtDate(s: string | undefined): string {
  if (!s) return '—'
  return new Date(s).toLocaleDateString('zh-CN')
}

// 状态徽章的底色：正向流程蓝/绿，售后黄，异常红，终态灰。
export const STATE_TONE: Record<State, string> = {
  produced: 'bg-slate-100 text-slate-700',
  qc_failed: 'bg-red-100 text-red-700',
  in_stock: 'bg-blue-100 text-blue-700',
  in_transit: 'bg-sky-100 text-sky-700',
  on_shelf: 'bg-indigo-100 text-indigo-700',
  sold: 'bg-emerald-100 text-emerald-700',
  activated: 'bg-green-100 text-green-700',
  returning: 'bg-amber-100 text-amber-800',
  returned: 'bg-amber-100 text-amber-800',
  inspecting: 'bg-amber-100 text-amber-800',
  refurbishing: 'bg-amber-100 text-amber-800',
  refurbished: 'bg-teal-100 text-teal-700',
  repairing: 'bg-orange-100 text-orange-700',
  scrapped: 'bg-zinc-200 text-zinc-600',
}
