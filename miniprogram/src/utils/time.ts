// 时间显示：服务端给 RFC 3339 时间，这里按手机本地时区显示。

const pad = (n: number) => (n < 10 ? `0${n}` : `${n}`)

export function clock(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** "今天" / "昨天" / "3 天前" / "10月12日" */
export function relativeDay(iso: string, now = new Date()): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const start = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const days = Math.round((start(now) - start(d)) / 86400000)
  if (days <= 0) return '今天'
  if (days === 1) return '昨天'
  if (days < 7) return `${days} 天前`
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

export function greeting(now = new Date()): string {
  const h = now.getHours()
  if (h < 6) return '夜深啦'
  if (h < 11) return '早上好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
}
