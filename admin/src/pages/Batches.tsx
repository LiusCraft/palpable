import { PackagePlus } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { useLoad } from '@/hooks/useLoad'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { fmtTime } from '@/lib/labels'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

const SKINS = ['dragon', 'rabbit', 'bear', 'cat']

// 每行一台：MAC 与公钥（base64 的 SPKI），用逗号、制表符或空格分隔——烧录工位导出的清单可以直接粘贴。
function parseUnits(text: string): { mac: string; public_key: string }[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('#'))
    .map((l) => {
      const [mac = '', public_key = ''] = l.split(/[\s,;]+/)
      return { mac, public_key }
    })
}

export function Batches() {
  const { me } = useAuth()
  const list = useLoad(() => api.batches(), [])
  const [open, setOpen] = useState(false)

  return (
    <div className="grid gap-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">生产批次</h1>
          <p className="text-sm text-muted-foreground">产线烧录后导入出厂清单。导入时公钥会同步给线上服务，设备才能上线。</p>
        </div>
        {me?.can_import && <Button onClick={() => setOpen(true)}><PackagePlus className="size-4" />导入批次</Button>}
      </div>
      {list.error && <p role="alert" className="text-sm text-destructive">{list.error}</p>}
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow><TableHead>批次</TableHead><TableHead>型号</TableHead><TableHead>皮套</TableHead><TableHead>固件</TableHead><TableHead className="text-right">数量</TableHead><TableHead className="hidden md:table-cell">导入</TableHead></TableRow>
          </TableHeader>
          <TableBody>
            {list.data?.map((b) => (
              <TableRow key={b.code}>
                <TableCell><Link to={`/units?batch=${b.code}`} className="font-mono text-primary hover:underline">{b.code}</Link></TableCell>
                <TableCell>{b.model}</TableCell>
                <TableCell>{b.skin}</TableCell>
                <TableCell>{b.firmware || '—'}</TableCell>
                <TableCell className="text-right tabular-nums">{b.units}</TableCell>
                <TableCell className="hidden text-sm text-muted-foreground md:table-cell">{fmtTime(b.created_at)} · {b.created_by}</TableCell>
              </TableRow>
            ))}
            {list.data?.length === 0 && <TableRow><TableCell colSpan={6} className="py-10 text-center text-muted-foreground">还没有批次</TableCell></TableRow>}
          </TableBody>
        </Table>
      </div>
      <ImportDialog open={open} onClose={() => setOpen(false)} onDone={list.reload} />
    </div>
  )
}

function ImportDialog({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const [code, setCode] = useState('')
  const [model, setModel] = useState('可感 一代')
  const [skin, setSkin] = useState('bear')
  const [firmware, setFirmware] = useState('')
  const [note, setNote] = useState('')
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const units = parseUnits(text)

  async function submit() {
    setBusy(true)
    try {
      const r = await api.importBatch({ code, model, skin, firmware, note, units })
      toast.success(`批次 ${r.code} 已导入 ${r.count} 台`)
      setText('')
      onDone()
      onClose()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>导入生产批次</DialogTitle>
          <DialogDescription>任何一行有问题或重复，整批都不会入库。序列号按行序自动生成，形如 PP批次号-00001。</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-2"><Label htmlFor="code">批次号（3–12 位大写字母或数字）</Label><Input id="code" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} placeholder="A2611" /></div>
          <div className="grid gap-2"><Label htmlFor="model">型号</Label><Input id="model" value={model} onChange={(e) => setModel(e.target.value)} /></div>
          <div className="grid gap-2">
            <Label>皮套</Label>
            <Select value={skin} onValueChange={setSkin}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>{SKINS.map((s) => <SelectItem key={s} value={s}>{s}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <div className="grid gap-2"><Label htmlFor="fw">固件版本</Label><Input id="fw" value={firmware} onChange={(e) => setFirmware(e.target.value)} placeholder="1.0.0" /></div>
        </div>
        <div className="grid gap-2"><Label htmlFor="note">备注（选填）</Label><Input id="note" value={note} onChange={(e) => setNote(e.target.value)} maxLength={200} /></div>
        <div className="grid gap-2">
          <Label htmlFor="units">设备清单：每行“MAC 公钥”，公钥为 base64 的 SPKI（P-256）</Label>
          <Textarea id="units" rows={7} className="font-mono text-xs" value={text} onChange={(e) => setText(e.target.value)}
            placeholder={'aa:bb:cc:dd:ee:01 MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...\n# 以 # 开头的行会被忽略'} />
          <p className="text-xs text-muted-foreground">已识别 {units.length} 台。私钥只在设备里，这里只需要公钥。</p>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button disabled={busy || !code || !model.trim() || units.length === 0} onClick={() => void submit()}>{busy ? '导入中…' : `导入 ${units.length} 台`}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
