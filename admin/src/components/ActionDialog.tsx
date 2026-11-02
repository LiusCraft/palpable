import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { api, type ActionName, type ActionParams, type BulkResult, type Channel } from '@/lib/api'
import { ACTION_HINT, ACTION_LABEL, DANGEROUS, GRADE_LABEL, LOCK_LABEL, NEEDS, NOTE_PLACEHOLDER } from '@/lib/labels'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

interface Props {
  /** 要执行的动作；null 表示关闭 */
  action: ActionName | null
  serials: string[]
  onClose: () => void
  /** 全部或部分成功后调用，让页面刷新 */
  onDone: () => void
}

// 动作表单：按动作需要显示输入框，提交到单条或批量接口。校验在服务端，这里只做必填的前置提示。
export function ActionDialog({ action, serials, onClose, onDone }: Props) {
  const [channels, setChannels] = useState<Channel[]>([])
  const [p, setP] = useState<ActionParams>({})
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState<BulkResult[]>([])
  const need = action ? NEEDS[action] ?? {} : {}

  useEffect(() => {
    if (!action) return
    setP({})
    setFailed([])
    if (NEEDS[action]?.channel) api.channels().then(setChannels, () => toast.error('读取渠道失败'))
  }, [action])

  if (!action) return null
  const missing =
    (need.channel && !p.channel) || (need.order && !p.order_ref?.trim()) || (need.note && !p.note?.trim()) ||
    (need.grade && !p.grade) || (need.lockReason && !p.lock_reason)

  async function submit() {
    if (!action) return
    setBusy(true)
    try {
      if (serials.length === 1) {
        await api.act(serials[0], action, p)
        toast.success(`${ACTION_LABEL[action]}：完成`)
        onDone()
        onClose()
        return
      }
      const { results } = await api.bulk(serials, action, p)
      const bad = results.filter((r) => !r.ok)
      if (bad.length === 0) {
        toast.success(`${ACTION_LABEL[action]}：${results.length} 台全部完成`)
        onDone()
        onClose()
        return
      }
      toast.warning(`${results.length - bad.length} 台成功，${bad.length} 台失败`)
      setFailed(bad)
      onDone()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const danger = DANGEROUS.includes(action)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{ACTION_LABEL[action]}{serials.length > 1 ? `（${serials.length} 台）` : ''}</DialogTitle>
          <DialogDescription>{serials.length === 1 ? serials[0] : '对所选设备逐台执行，失败的会列在下面，不影响其他设备。'}</DialogDescription>
        </DialogHeader>

        {ACTION_HINT[action] && (
          <p className={danger ? 'rounded-md bg-red-50 p-3 text-sm text-red-700' : 'rounded-md bg-muted p-3 text-sm text-muted-foreground'}>
            {ACTION_HINT[action]}
          </p>
        )}

        <div className="grid gap-4">
          {need.channel && (
            <div className="grid gap-2">
              <Label>发往渠道</Label>
              <Select value={p.channel ?? ''} onValueChange={(v) => setP({ ...p, channel: v })}>
                <SelectTrigger><SelectValue placeholder={channels.length ? '选择渠道' : '还没有渠道，请先到“渠道”页添加'} /></SelectTrigger>
                <SelectContent>{channels.map((c) => <SelectItem key={c.code} value={c.code}>{c.name}（{c.code}）</SelectItem>)}</SelectContent>
              </Select>
            </div>
          )}
          {need.order && (
            <div className="grid gap-2">
              <Label htmlFor="order">订单号</Label>
              <Input id="order" value={p.order_ref ?? ''} maxLength={40} onChange={(e) => setP({ ...p, order_ref: e.target.value })} />
              <p className="text-xs text-muted-foreground">只填订单号。不要填买家姓名、电话或地址。</p>
            </div>
          )}
          {need.grade && (
            <div className="grid gap-2">
              <Label>成色</Label>
              <Select value={p.grade ?? ''} onValueChange={(v) => setP({ ...p, grade: v })}>
                <SelectTrigger><SelectValue placeholder="选择成色" /></SelectTrigger>
                <SelectContent>{(['A', 'B'] as const).map((g) => <SelectItem key={g} value={g}>{GRADE_LABEL[g]}</SelectItem>)}</SelectContent>
              </Select>
            </div>
          )}
          {need.lockReason && (
            <div className="grid gap-2">
              <Label>原因</Label>
              <Select value={p.lock_reason ?? ''} onValueChange={(v) => setP({ ...p, lock_reason: v })}>
                <SelectTrigger><SelectValue placeholder="选择原因" /></SelectTrigger>
                <SelectContent>{Object.entries(LOCK_LABEL).map(([k, v]) => <SelectItem key={k} value={k}>{v}</SelectItem>)}</SelectContent>
              </Select>
            </div>
          )}
          <div className="grid gap-2">
            <Label htmlFor="note">备注{need.note ? '' : '（选填）'}</Label>
            <Textarea id="note" rows={2} maxLength={200} value={p.note ?? ''} placeholder={NOTE_PLACEHOLDER[action]}
              onChange={(e) => setP({ ...p, note: e.target.value })} />
          </div>
        </div>

        {failed.length > 0 && (
          <div className="max-h-40 overflow-auto rounded-md border p-3 text-sm">
            <p className="mb-1 font-medium">失败的设备</p>
            {failed.map((f) => <p key={f.serial} className="text-muted-foreground"><span className="font-mono">{f.serial}</span>：{f.error}</p>)}
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{failed.length ? '关闭' : '取消'}</Button>
          {!failed.length && (
            <Button variant={danger ? 'destructive' : 'default'} disabled={busy || Boolean(missing)} onClick={() => void submit()}>
              {busy ? '处理中…' : '确认'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
