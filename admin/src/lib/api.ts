// 后台 API 的类型与调用。所有规则（谁能做什么、状态能否流转）都在服务端，这里只负责收发。

export type State =
  | 'produced' | 'qc_failed' | 'in_stock' | 'in_transit' | 'on_shelf' | 'sold' | 'activated'
  | 'returning' | 'returned' | 'inspecting' | 'refurbishing' | 'refurbished' | 'repairing' | 'scrapped'

export type ActionName =
  | 'qc_pass' | 'qc_fail' | 'rework' | 'ship' | 'receive' | 'unshelf' | 'sell' | 'return_request'
  | 'return_cancel' | 'receive_return' | 'inspect' | 'inspect_pass' | 'refurbish' | 'refurbish_done'
  | 'repair' | 'repair_done' | 'scrap' | 'lock' | 'unlock'

export type Role = 'admin' | 'warehouse' | 'sales' | 'support' | 'viewer'

export interface Staff { id: number; username: string; role: Role; disabled: boolean; created_at: string }

export interface Unit {
  serial: string
  mac: string
  key_fingerprint: string
  batch: string
  skin: string
  firmware: string
  state: State
  resume_state?: State
  grade: 'new' | 'A' | 'B'
  refurb_count: number
  channel: string
  order_ref: string
  sold_at: string
  warranty_until: string
  in_warranty: boolean
  has_owner: boolean
  transfer_count: number
  locked: boolean
  lock_reason: string
  version: number
  created_at: string
  updated_at: string
  actions: ActionName[]
}

export interface UnitEvent {
  id: number
  serial: string
  at: string
  actor: string
  action: string
  from: State | ''
  to: State | ''
  note: string
  anomaly: boolean
  resolved_by?: string
  resolved_at?: string
}

export interface Stats { total: number; by_state: Record<State, number>; locked: number; open_anomalies: number }
export interface Batch { code: string; model: string; skin: string; firmware: string; note: string; created_by: string; created_at: string; units: number }
export interface Channel { code: string; name: string; kind: 'warehouse' | 'online' | 'offline' | 'distributor' }
export interface Me { staff: Staff; can_import: boolean; can_resolve: boolean; is_admin: boolean }

export interface ActionParams { channel?: string; order_ref?: string; note?: string; grade?: string; lock_reason?: string }
export interface BulkResult { serial: string; ok: boolean; state?: State; error?: string }
export interface UnitFilter { state?: string; q?: string; channel?: string; batch?: string; locked?: boolean; limit?: number; offset?: number }

export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

const TOKEN_KEY = 'palpable-admin-token'

// 令牌放 sessionStorage：关掉标签页就退出，比 localStorage 更适合共用电脑的仓库和客服。
export const session = {
  get(): string {
    try { return sessionStorage.getItem(TOKEN_KEY) ?? '' } catch { return '' }
  },
  set(t: string) {
    try { sessionStorage.setItem(TOKEN_KEY, t) } catch { /* 隐私模式下不可用，只在内存里失效 */ }
  },
  clear() {
    try { sessionStorage.removeItem(TOKEN_KEY) } catch { /* 同上 */ }
  },
}

let onUnauthorized: () => void = () => {}
export function setUnauthorizedHandler(fn: () => void) { onUnauthorized = fn }

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const token = session.get()
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  let resp: Response
  try {
    resp = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) })
  } catch {
    throw new ApiError(0, '连不上服务器，请检查网络')
  }
  if (resp.status === 204) return undefined as T
  const data: unknown = await resp.json().catch(() => ({}))
  if (!resp.ok) {
    const msg = typeof data === 'object' && data !== null && 'error' in data ? String((data as { error: unknown }).error) : `请求失败（${resp.status}）`
    if (resp.status === 401 && token) onUnauthorized()
    throw new ApiError(resp.status, msg)
  }
  return data as T
}

function qs(params: Record<string, string | number | boolean | undefined>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') p.set(k, String(v))
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  login: (username: string, password: string) => call<{ token: string; staff: Staff }>('POST', '/api/login', { username, password }),
  logout: () => call<void>('POST', '/api/logout'),
  me: () => call<Me>('GET', '/api/me'),
  changePassword: (oldPw: string, newPw: string) => call<void>('POST', '/api/me/password', { old: oldPw, new: newPw }),

  stats: () => call<Stats>('GET', '/api/stats'),
  units: (f: UnitFilter) => call<{ items: Unit[]; total: number }>('GET', `/api/units${qs({ ...f })}`),
  unit: (serial: string) => call<{ unit: Unit; events: UnitEvent[] }>('GET', `/api/units/${encodeURIComponent(serial)}`),
  act: (serial: string, action: ActionName, p: ActionParams) =>
    call<Unit>('POST', `/api/units/${encodeURIComponent(serial)}/actions`, { action, ...p }),
  bulk: (serials: string[], action: ActionName, p: ActionParams) =>
    call<{ results: BulkResult[] }>('POST', '/api/bulk-actions', { serials, action, ...p }),

  batches: () => call<Batch[]>('GET', '/api/batches'),
  importBatch: (b: { code: string; model: string; skin: string; firmware: string; note: string; units: { mac: string; public_key: string }[] }) =>
    call<{ code: string; count: number; serials: string[] }>('POST', '/api/batches', b),
  channels: () => call<Channel[]>('GET', '/api/channels'),
  putChannel: (code: string, name: string, kind: Channel['kind']) => call<void>('PUT', `/api/channels/${encodeURIComponent(code)}`, { name, kind }),

  events: (f: { anomaly?: boolean; serial?: string; limit?: number }) =>
    call<UnitEvent[]>('GET', `/api/events${qs({ anomaly: f.anomaly ? 1 : undefined, serial: f.serial, limit: f.limit })}`),
  resolve: (id: number) => call<void>('POST', `/api/events/${id}/resolve`),

  staff: () => call<Staff[]>('GET', '/api/staff'),
  createStaff: (username: string, password: string, role: Role) => call<Staff>('POST', '/api/staff', { username, password, role }),
  updateStaff: (id: number, role: Role, disabled: boolean) => call<void>('PATCH', `/api/staff/${id}`, { role, disabled }),
}
