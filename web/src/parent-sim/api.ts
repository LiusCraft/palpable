// 家长端 REST 客户端：Web 手机窗口与将来的小程序调用同一套 /api。
// 演示令牌只代表演示家长，不是灵矽令牌；可用 ?parent_token= 覆盖。

const token = new URLSearchParams(location.search).get('parent_token') ?? 'palpable-demo'

export interface Kid {
  id: string
  doll_name: string
  kid_name: string
  skin: string
  stage: 'fresh' | 'named' | 'ready'
  online: boolean
}

export interface Friend {
  request_id: string
  device_id: string
  name: string
  status: 'pending' | 'confirmed'
  /** pending 时：you 等你同意；other 你已同意（或自动放行），在等对方家长 */
  waiting_for?: 'you' | 'other'
  auto: boolean
  /** 你屏蔽了这位朋友的小伙伴：它在频道里说话时你家孩子听不到 */
  agent_blocked: boolean
  online: boolean
  since: string
}

export interface Group {
  id: string
  kind: 'zone' | 'group'
  name: string
  code?: string
  expires_at?: string
  foreground: boolean
  members: number
}

export type FriendPolicy = 'approve' | 'notify'

export interface Settings {
  friend_policy: FriendPolicy
  /** 自家小伙伴被孩子点名时，能否在频道里对好朋友说话 */
  agent_in_channel: boolean
}

export interface Digest {
  met: { friend: string; name: string; channel: string; at: string }[]
  shared: { from: string; name: string; text: string; channel: string; at: string }[]
  chats: { role: 'user' | 'assistant'; text: string; at: string }[]
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(path, {
    method,
    headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!resp.ok) throw new Error(`${resp.status}`)
  return resp.status === 204 ? (undefined as T) : ((await resp.json()) as T)
}

export const parentApi = {
  kids: () => call<Kid[]>('GET', '/api/kids'),
  friends: (kid: string) => call<Friend[]>('GET', `/api/kids/${kid}/friends`),
  groups: (kid: string) => call<Group[]>('GET', `/api/kids/${kid}/groups`),
  digest: (kid: string) => call<Digest>('GET', `/api/kids/${kid}/digest?date=today`),
  confirm: (requestId: string) => call<void>('POST', `/api/friend-requests/${requestId}/confirm`),
  remove: (kid: string, friend: string) => call<void>('DELETE', `/api/kids/${kid}/friends/${friend}`),
  bind: (code: string, voiceConsent: boolean) =>
    call<{ id: string }>('POST', '/api/bindings', { code, ai_companion: true, voice_consent: voiceConsent }),
  unbind: (kid: string) => call<void>('DELETE', `/api/kids/${kid}`),
  settings: (kid: string) => call<Settings>('GET', `/api/kids/${kid}/settings`),
  saveSettings: (kid: string, s: Settings) => call<void>('PUT', `/api/kids/${kid}/settings`, s),
  blockAgent: (kid: string, friend: string, blocked: boolean) =>
    call<void>('PUT', `/api/kids/${kid}/friends/${friend}/agent`, { blocked }),
}
