import { useState } from 'react'
import { toast } from 'sonner'
import { useLoad } from '@/hooks/useLoad'
import { api, type Channel } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { KIND_LABEL } from '@/lib/labels'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

export function Channels() {
  const { me } = useAuth()
  const list = useLoad(() => api.channels(), [])
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [kind, setKind] = useState<Channel['kind']>('online')

  async function save() {
    try {
      await api.putChannel(code, name, kind)
      toast.success('渠道已保存')
      setCode('')
      setName('')
      list.reload()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div className="grid gap-4">
      <div>
        <h1 className="text-2xl font-semibold">渠道</h1>
        <p className="text-sm text-muted-foreground">发货时选择的去向：仓库、电商店铺、线下门店或经销商。</p>
      </div>
      {list.error && <p role="alert" className="text-sm text-destructive">{list.error}</p>}
      <div className="rounded-md border">
        <Table>
          <TableHeader><TableRow><TableHead>代码</TableHead><TableHead>名称</TableHead><TableHead>类型</TableHead></TableRow></TableHeader>
          <TableBody>
            {list.data?.map((c) => (
              <TableRow key={c.code}><TableCell className="font-mono">{c.code}</TableCell><TableCell>{c.name}</TableCell><TableCell>{KIND_LABEL[c.kind]}</TableCell></TableRow>
            ))}
            {list.data?.length === 0 && <TableRow><TableCell colSpan={3} className="py-10 text-center text-muted-foreground">还没有渠道，先添加一个才能发货</TableCell></TableRow>}
          </TableBody>
        </Table>
      </div>
      {me?.is_admin && (
        <Card>
          <CardHeader><CardTitle>添加或修改渠道</CardTitle></CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-4 sm:items-end">
            <div className="grid gap-2"><Label htmlFor="cc">代码（2–24 位小写字母、数字、-、_）</Label><Input id="cc" value={code} onChange={(e) => setCode(e.target.value)} placeholder="jd" /></div>
            <div className="grid gap-2"><Label htmlFor="cn">名称</Label><Input id="cn" value={name} onChange={(e) => setName(e.target.value)} placeholder="京东自营" /></div>
            <div className="grid gap-2">
              <Label>类型</Label>
              <Select value={kind} onValueChange={(v) => setKind(v as Channel['kind'])}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>{(Object.keys(KIND_LABEL) as Channel['kind'][]).map((k) => <SelectItem key={k} value={k}>{KIND_LABEL[k]}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <Button disabled={!code || !name.trim()} onClick={() => void save()}>保存</Button>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
