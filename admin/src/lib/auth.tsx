import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, session, setUnauthorizedHandler, type Me } from './api'

interface AuthState {
  me: Me | null
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

const Ctx = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(Boolean(session.get()))

  const refresh = useCallback(async () => {
    if (!session.get()) {
      setMe(null)
      setLoading(false)
      return
    }
    try {
      setMe(await api.me())
    } catch {
      session.clear()
      setMe(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    setUnauthorizedHandler(() => {
      session.clear()
      setMe(null)
    })
    void refresh()
  }, [refresh])

  const login = useCallback(async (username: string, password: string) => {
    const r = await api.login(username, password)
    session.set(r.token)
    await refresh()
  }, [refresh])

  const logout = useCallback(async () => {
    try { await api.logout() } finally {
      session.clear()
      setMe(null)
    }
  }, [])

  const value = useMemo(() => ({ me, loading, login, logout, refresh }), [me, loading, login, logout, refresh])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useAuth(): AuthState {
  const v = useContext(Ctx)
  if (!v) throw new Error('useAuth must be used inside AuthProvider')
  return v
}
