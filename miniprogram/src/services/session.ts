// 家长端登录态：wx.login 拿 code，换服务端签发的会话令牌，存在本地。
// 令牌失效（401）时静默重新登录一次，家长无感；并发请求共用同一次登录。
import { env, envVersion } from '../config'
import { send } from './http'
import type { SessionToken } from './types'

const KEY = 'palpable.session.v1'
// 离过期不到一天就提前换新，避免请求半路过期。
const REFRESH_EARLY_MS = 24 * 3600 * 1000

interface Stored {
  token: string
  expiresAt: number
}

let inflight: Promise<string> | null = null

function load(): Stored | null {
  try {
    const s = wx.getStorageSync(KEY) as Stored | ''
    return s && typeof s.token === 'string' ? s : null
  } catch {
    return null
  }
}

function devToken(): string | null {
  return envVersion === 'develop' && env.devToken ? env.devToken : null
}

/** 当前可用的令牌；没有或快过期时登录。 */
export function getToken(): Promise<string> {
  const dev = devToken()
  if (dev) return Promise.resolve(dev)
  const s = load()
  if (s && s.expiresAt - Date.now() > REFRESH_EARLY_MS) return Promise.resolve(s.token)
  return signIn()
}

/** 强制重新登录（令牌被服务端拒绝时调用）。 */
export function signIn(): Promise<string> {
  const dev = devToken()
  if (dev) return Promise.resolve(dev)
  if (!inflight) {
    inflight = doSignIn().finally(() => {
      inflight = null
    })
  }
  return inflight
}

async function doSignIn(): Promise<string> {
  const code = await new Promise<string>((resolve, reject) => {
    wx.login({ success: (r) => resolve(r.code), fail: (e) => reject(new Error(e.errMsg)) })
  })
  const t = await send<SessionToken>('POST', '/api/session', null, { code })
  const stored: Stored = { token: t.token, expiresAt: Date.parse(t.expires_at) }
  wx.setStorageSync(KEY, stored)
  return t.token
}

export function clearSession(): void {
  try {
    wx.removeStorageSync(KEY)
  } catch {
    // 本地存储不可用时没有可清的
  }
}
