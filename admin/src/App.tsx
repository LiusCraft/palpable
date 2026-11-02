import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from '@/components/Layout'
import { Toaster } from '@/components/ui/sonner'
import { AuthProvider, useAuth } from '@/lib/auth'
import { Batches } from '@/pages/Batches'
import { Channels } from '@/pages/Channels'
import { Dashboard } from '@/pages/Dashboard'
import { Login } from '@/pages/Login'
import { StaffPage } from '@/pages/StaffPage'
import { UnitDetail } from '@/pages/UnitDetail'
import { Units } from '@/pages/Units'

function Protected() {
  const { me, loading } = useAuth()
  if (loading) return <div className="p-8 text-sm text-muted-foreground">加载中…</div>
  if (!me) return <Navigate to="/login" replace />
  return <Layout />
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route element={<Protected />}>
            <Route index element={<Dashboard />} />
            <Route path="units" element={<Units />} />
            <Route path="units/:serial" element={<UnitDetail />} />
            <Route path="batches" element={<Batches />} />
            <Route path="channels" element={<Channels />} />
            <Route path="staff" element={<StaffPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
      <Toaster richColors position="top-right" />
    </AuthProvider>
  )
}
