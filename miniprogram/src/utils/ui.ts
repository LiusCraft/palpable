// 页面反馈：TDesign Toast 与把接口错误翻成家长看得懂的话。
import Toast from 'tdesign-miniprogram/toast/index'
import { ApiError } from '../services/http'
import { copy } from './copy'

type Ctx = WechatMiniprogram.Page.TrivialInstance | WechatMiniprogram.Component.TrivialInstance

export function toast(ctx: Ctx, message: string, theme: 'success' | 'error' | 'warning' | 'loading' = 'success'): void {
  Toast({ context: ctx, selector: '#t-toast', message, theme, direction: 'column' })
}

/** overrides 按状态码给出这个场景下更具体的说法。 */
export function errorText(e: unknown, overrides: Record<number, string> = {}): string {
  if (!(e instanceof ApiError)) return copy.errors.unknown
  if (overrides[e.status]) return overrides[e.status]
  if (e.status === 0) return copy.errors.network
  if (e.status === 401) return copy.errors.auth
  if (e.status === 404) return copy.errors.notFound
  if (e.status === 409) return copy.errors.conflict
  if (e.status === 429) return copy.errors.tooMany
  if (e.status >= 500) return copy.errors.server
  return copy.errors.unknown
}
