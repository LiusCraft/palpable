import { Boxes, ClipboardList, LayoutDashboard, LogOut, PackagePlus, Store, Users } from 'lucide-react'
import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { ROLE_LABEL } from '@/lib/labels'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const NAV = [
  { to: '/', label: '概览', icon: LayoutDashboard, end: true },
  { to: '/units', label: '设备台账', icon: Boxes },
  { to: '/batches', label: '生产批次', icon: PackagePlus },
  { to: '/channels', label: '渠道', icon: Store },
  { to: '/staff', label: '人员', icon: Users, adminOnly: true },
]

export function Layout() {
  const { me, logout } = useAuth()
  const [pwOpen, setPwOpen] = useState(false)
  if (!me) return null
  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-56 shrink-0 flex-col border-r bg-card md:flex">
        <div className="flex items-center gap-2 px-5 py-4 font-semibold"><ClipboardList className="size-5 text-primary" />可感 · 厂家后台</div>
        <nav className="flex flex-1 flex-col gap-1 px-3">
          {NAV.filter((n) => !n.adminOnly || me.is_admin).map((n) => (
            <NavLink key={n.to} to={n.to} end={n.end}
              className={({ isActive }) => cn('flex items-center gap-2 rounded-md px-3 py-2 text-sm', isActive ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-accent')}>
              <n.icon className="size-4" />{n.label}
            </NavLink>
          ))}
        </nav>
        <div className="border-t p-3 text-sm">
          <p className="font-medium">{me.staff.username}</p>
          <p className="mb-2 text-xs text-muted-foreground">{ROLE_LABEL[me.staff.role]}</p>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" onClick={() => setPwOpen(true)}>改口令</Button>
            <Button size="sm" variant="ghost" onClick={() => void logout()}><LogOut className="size-4" />退出</Button>
          </div>
        </div>
      </aside>
      <main className="min-w-0 flex-1 p-4 md:p-8">
        <nav className="mb-4 flex gap-1 overflow-x-auto md:hidden">
          {NAV.filter((n) => !n.adminOnly || me.is_admin).map((n) => (
            <NavLink key={n.to} to={n.to} end={n.end}
              className={({ isActive }) => cn('shrink-0 rounded-md px-3 py-1.5 text-sm', isActive ? 'bg-primary/10 text-primary' : 'text-muted-foreground')}>{n.label}</NavLink>
          ))}
          <button className="ml-auto shrink-0 px-3 text-sm text-muted-foreground" onClick={() => void logout()}>退出</button>
        </nav>
        <Outlet />
      </main>
      <PasswordDialog open={pwOpen} onClose={() => setPwOpen(false)} />
    </div>
  )
}

function PasswordDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { logout } = useAuth()
  const [oldPw, setOld] = useState('')
  const [newPw, setNew] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    try {
      await api.changePassword(oldPw, newPw)
      toast.success('口令已修改，请重新登录')
      await logout()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader><DialogTitle>修改口令</DialogTitle></DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2"><Label htmlFor="old">当前口令</Label><Input id="old" type="password" autoComplete="current-password" value={oldPw} onChange={(e) => setOld(e.target.value)} /></div>
          <div className="grid gap-2"><Label htmlFor="new">新口令（至少 10 位）</Label><Input id="new" type="password" autoComplete="new-password" value={newPw} onChange={(e) => setNew(e.target.value)} /></div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button disabled={busy || newPw.length < 10 || !oldPw} onClick={() => void submit()}>修改</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
