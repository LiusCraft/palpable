import { useCallback, useEffect, useState } from 'react'

export interface Loaded<T> {
  data: T | null
  error: string
  loading: boolean
  reload: () => void
}

// 最小的取数 hook：依赖变化时重新请求，丢弃过期响应。
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[]): Loaded<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [tick, setTick] = useState(0)

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const run = useCallback(fn, deps)

  useEffect(() => {
    let stale = false
    setLoading(true)
    run().then(
      (d) => { if (!stale) { setData(d); setError(''); setLoading(false) } },
      (e: unknown) => { if (!stale) { setError(e instanceof Error ? e.message : String(e)); setLoading(false) } },
    )
    return () => { stale = true }
  }, [run, tick])

  return { data, error, loading, reload: () => setTick((n) => n + 1) }
}
