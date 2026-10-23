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

export interface Digest {
  met: { friend: string; name: string; channel: string; at: string }[]
  shared: { from: string; name: string; text: string; channel: string; at: string }[]
  chats: { role: 'user' | 'assistant'; text: string; at: string }[]
}

async function call<T>(method: string, path: string): Promise<T> {
  const resp = await fetch(path, { method, headers: { Authorization: `Bearer ${token}` } })
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
}
