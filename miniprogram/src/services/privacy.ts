// 隐私同意：首次打开先请家长阅读并同意隐私政策，同意前不登录、不请求服务端。
// 同意按钮用 open-type="agreePrivacyAuthorization"，同时完成微信平台要求的隐私授权同步。
import { brand } from '../config'

const KEY = 'palpable.privacy'

export function hasAgreed(): boolean {
  try {
    return wx.getStorageSync(KEY) === brand.policyVersion
  } catch {
    return false
  }
}

export function markAgreed(): void {
  wx.setStorageSync(KEY, brand.policyVersion)
}

type Resolver = (r: { event: 'agree' | 'disagree'; buttonId?: string }) => void
type Listener = (resolve: Resolver) => void

// 调用隐私接口（如复制群号）而平台判定需要授权时，交给当前页面上的隐私弹窗处理。
const listeners: Listener[] = []

export function onNeedAuthorization(fn: Listener): () => void {
  listeners.push(fn)
  return () => {
    const i = listeners.indexOf(fn)
    if (i >= 0) listeners.splice(i, 1)
  }
}

export function installPrivacyHandler(): void {
  if (!wx.onNeedPrivacyAuthorization) return
  wx.onNeedPrivacyAuthorization((resolve) => {
    const fn = listeners[listeners.length - 1]
    if (fn) fn(resolve as unknown as Resolver)
    else (resolve as unknown as Resolver)({ event: 'disagree' })
  })
}
