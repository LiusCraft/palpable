// wx.request 的 Promise 封装。只负责收发与把失败统一成 ApiError，不含登录逻辑。
import { env } from '../config'

export class ApiError extends Error {
  /** HTTP 状态码；0 表示网络不通或超时 */
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export type Method = 'GET' | 'POST' | 'PUT' | 'DELETE'

export function send<T>(method: Method, path: string, token: string | null, body?: object): Promise<T> {
  return new Promise((resolve, reject) => {
    const header: Record<string, string> = {}
    if (token) header.Authorization = `Bearer ${token}`
    if (body) header['Content-Type'] = 'application/json'
    wx.request({
      url: env.apiBase + path,
      method,
      header,
      data: body,
      timeout: 10000,
      dataType: 'json',
      success: (res) => {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve((res.statusCode === 204 ? undefined : res.data) as T)
          return
        }
        const data = res.data as { error?: unknown } | undefined
        const msg = data && typeof data.error === 'string' ? data.error : `http ${res.statusCode}`
        reject(new ApiError(res.statusCode, msg))
      },
      fail: (err) => reject(new ApiError(0, err.errMsg)),
    })
  })
}
