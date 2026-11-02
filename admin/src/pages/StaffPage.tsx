import { useState } from 'react'
import { Navigate } from 'react-router-dom'
import { toast } from 'sonner'
import { useLoad } from '@/hooks/useLoad'
import { api, type Role } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { fmtDate, ROLE_LABEL } from '@/lib/labels'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const ROLES = Object.keys(ROLE_LABEL) as Role[]

export function StaffPage() {
  const { me } = useAuth()
  const list = useLoad(() => (me?.is_admin ? api.staff() : Promise.resolve([])), [me?.is_admin])
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<Role>('viewer')
  if (me && !me.is_admin) return <Navigate to="/" replace />

  async function run(fn: () => Promise<unknown>, ok: string) {
    try {
      await fn()
      toast.success(ok)
      list.reload()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div className="grid gap-4">
      <div>
        <h1 className="text-2xl font-semibold">人员</h1>
        <p className="text-sm text-muted-foreground">停用账号后，对方的登录会立即失效。不能停用或降级自己。</p>
      </div>
      {list.error && <p role="alert" className="text-sm text-destructive">{list.error}</p>}
      <div className="rounded-md border">
        <Table>
          <TableHeader><TableRow><TableHead>用户名</TableHead><TableHead>角色</TableHead><TableHead className="hidden md:table-cell">创建</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
          <TableBody>
            {list.data?.map((s) => (
              <TableRow key={s.id} className={s.disabled ? 'opacity-50' : ''}>
                <TableCell className="font-medium">{s.username}{s.disabled ? '（已停用）' : ''}</TableCell>
                <TableCell>
                  <Select value={s.role} disabled={s.id === me?.staff.id} onValueChange={(v) => void run(() => api.updateStaff(s.id, v as Role, s.disabled), '角色已更新')}>
                    <SelectTrigger className="w-28"><SelectValue /></SelectTrigger>
                    <SelectContent>{ROLES.map((r) => <SelectItem key={r} value={r}>{ROLE_LABEL[r]}</SelectItem>)}</SelectContent>
                  </Select>
                </TableCell>
                <TableCell className="hidden text-sm text-muted-foreground md:table-cell">{fmtDate(s.created_at)}</TableCell>
                <TableCell className="text-right">
                  {s.id !== me?.staff.id && (
                    <Button size="sm" variant="outline" onClick={() => void run(() => api.updateStaff(s.id, s.role, !s.disabled), s.disabled ? '已启用' : '已停用')}>
                      {s.disabled ? '启用' : '停用'}
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <Card>
        <CardHeader><CardTitle>新建账号</CardTitle></CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-4 sm:items-end">
          <div className="grid gap-2"><Label htmlFor="nu">用户名（小写字母、数字、. _ -）</Label><Input id="nu" autoComplete="off" value={username} onChange={(e) => setUsername(e.target.value)} /></div>
          <div className="grid gap-2"><Label htmlFor="np">初始口令（至少 10 位）</Label><Input id="np" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} /></div>
          <div className="grid gap-2">
            <Label>角色</Label>
            <Select value={role} onValueChange={(v) => setRole(v as Role)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>{ROLES.map((r) => <SelectItem key={r} value={r}>{ROLE_LABEL[r]}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <Button disabled={!username || password.length < 10}
            onClick={() => void run(async () => { await api.createStaff(username, password, role); setUsername(''); setPassword('') }, '账号已创建')}>创建</Button>
        </CardContent>
      </Card>
    </div>
  )
}
