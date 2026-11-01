// 家长端接口。所有业务规则在服务端；这里只发请求，遇到 401 重新登录后重试一次。
import { ApiError, send, type Method } from './http'
import { clearSession, getToken, signIn } from './session'
import type { Digest, Friend, Group, Kid, Me, Settings, SettingsUpdate } from './types'

async function call<T>(method: Method, path: string, body?: object): Promise<T> {
  const token = await getToken()
  try {
    return await send<T>(method, path, token, body)
  } catch (e) {
    if (!(e instanceof ApiError) || e.status !== 401) throw e
    clearSession()
    return send<T>(method, path, await signIn(), body)
  }
}

const kid = (id: string) => `/api/kids/${encodeURIComponent(id)}`

export const api = {
  kids: () => call<Kid[]>('GET', '/api/kids'),
  friends: (k: string) => call<Friend[]>('GET', `${kid(k)}/friends`),
  groups: (k: string) => call<Group[]>('GET', `${kid(k)}/groups`),
  digest: (k: string) => call<Digest>('GET', `${kid(k)}/digest?date=today`),
  confirm: (requestId: string) => call<void>('POST', `/api/friend-requests/${encodeURIComponent(requestId)}/confirm`),
  removeFriend: (k: string, friend: string) => call<void>('DELETE', `${kid(k)}/friends/${encodeURIComponent(friend)}`),
  blockAgent: (k: string, friend: string, blocked: boolean) =>
    call<void>('PUT', `${kid(k)}/friends/${encodeURIComponent(friend)}/agent`, { blocked }),
  settings: (k: string) => call<Settings>('GET', `${kid(k)}/settings`),
  saveSettings: (k: string, s: SettingsUpdate) => call<void>('PUT', `${kid(k)}/settings`, s),
  bind: (code: string, voiceConsent: boolean) =>
    call<{ id: string }>('POST', '/api/bindings', { code, ai_companion: true, voice_consent: voiceConsent }),
  unbind: (k: string) => call<void>('DELETE', kid(k)),
  me: () => call<Me>('GET', '/api/me'),
  setDisplayName: (name: string) => call<void>('PUT', '/api/me', { display_name: name }),
  signOut: async () => {
    try {
      await call<void>('DELETE', '/api/session')
    } finally {
      clearSession()
    }
  },
  deleteAccount: async () => {
    await call<void>('DELETE', '/api/me')
    clearSession()
  },
}

export { ApiError }
