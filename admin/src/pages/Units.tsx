import { Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ActionDialog } from '@/components/ActionDialog'
import { StateBadge } from '@/components/StateBadge'
import { useLoad } from '@/hooks/useLoad'
import { api, type ActionName } from '@/lib/api'
import { ACTION_LABEL, fmtDate, STATE_LABEL, STATE_ORDER } from '@/lib/labels'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const PAGE = 50
const ALL = 'all'

export function Units() {
  const [sp, setSp] = useSearchParams()
  const state = sp.get('state') ?? ''
  const q = sp.get('q') ?? ''
  const batch = sp.get('batch') ?? ''
  const locked = sp.get('locked') === '1'
  const page = Number(sp.get('page') ?? '0') || 0
  const [draft, setDraft] = useState(q)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [action, setAction] = useState<ActionName | null>(null)

  const list = useLoad(
    () => api.units({ state, q, batch, locked: locked || undefined, limit: PAGE, offset: page * PAGE }),
    [state, q, batch, locked, page],
  )
  const items = list.data?.items ?? []
  const total = list.data?.total ?? 0

  function set(k: string, v: string) {
    const next = new URLSearchParams(sp)
    if (v) next.set(k, v)
    else next.delete(k)
    if (k !== 'page') next.delete('page')
    setSp(next)
    setSelected(new Set())
  }

  // 批量只提供所选设备“共同可做”的动作，避免点了才发现一半不行。
  const common = useMemo<ActionName[]>(() => {
    const chosen = items.filter((u) => selected.has(u.serial))
    if (chosen.length === 0) return []
    return chosen[0].actions.filter((a) => chosen.every((u) => u.actions.includes(a)))
  }, [items, selected])

  const allOnPage = items.length > 0 && items.every((u) => selected.has(u.serial))

  return (
    <div className="grid gap-4">
      <div>
        <h1 className="text-2xl font-semibold">设备台账</h1>
        <p className="text-sm text-muted-foreground">共 {total} 台{state ? `，状态：${STATE_LABEL[state as keyof typeof STATE_LABEL] ?? state}` : ''}{batch ? `，批次 ${batch}` : ''}{locked ? '，仅已锁定' : ''}</p>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); set('q', draft.trim()) }}>
          <Input className="w-64" placeholder="序列号、MAC 或订单号" value={draft} onChange={(e) => setDraft(e.target.value)} />
          <Button type="submit" variant="outline"><Search className="size-4" />搜索</Button>
        </form>
        <Select value={state || ALL} onValueChange={(v) => set('state', v === ALL ? '' : v)}>
          <SelectTrigger className="w-40"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>全部状态</SelectItem>
            {STATE_ORDER.map((s) => <SelectItem key={s} value={s}>{STATE_LABEL[s]}</SelectItem>)}
          </SelectContent>
        </Select>
        <Button variant={locked ? 'default' : 'outline'} onClick={() => set('locked', locked ? '' : '1')}>只看已锁定</Button>
        {(state || q || batch || locked) && <Button variant="ghost" onClick={() => { setDraft(''); setSp(new URLSearchParams()); setSelected(new Set()) }}>清除筛选</Button>}
      </div>

      {selected.size > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/50 p-3 text-sm">
          <span>已选 {selected.size} 台</span>
          {common.length === 0
            ? <span className="text-muted-foreground">所选设备没有共同可做的动作</span>
            : common.map((a) => <Button key={a} size="sm" variant="outline" onClick={() => setAction(a)}>{ACTION_LABEL[a]}</Button>)}
          <Button size="sm" variant="ghost" className="ml-auto" onClick={() => setSelected(new Set())}>取消选择</Button>
        </div>
      )}

      {list.error && <p role="alert" className="text-sm text-destructive">{list.error}</p>}
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">
                <input type="checkbox" aria-label="全选本页" checked={allOnPage}
                  onChange={() => setSelected(allOnPage ? new Set() : new Set(items.map((u) => u.serial)))} />
              </TableHead>
              <TableHead>序列号</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="hidden md:table-cell">MAC</TableHead>
              <TableHead className="hidden lg:table-cell">渠道 / 订单</TableHead>
              <TableHead className="hidden lg:table-cell">保修至</TableHead>
              <TableHead className="hidden xl:table-cell">批次</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((u) => (
              <TableRow key={u.serial}>
                <TableCell>
                  <input type="checkbox" aria-label={`选择 ${u.serial}`} checked={selected.has(u.serial)}
                    onChange={() => { const n = new Set(selected); if (n.has(u.serial)) n.delete(u.serial); else n.add(u.serial); setSelected(n) }} />
                </TableCell>
                <TableCell><Link to={`/units/${u.serial}`} className="font-mono text-primary hover:underline">{u.serial}</Link></TableCell>
                <TableCell><StateBadge unit={u} /></TableCell>
                <TableCell className="hidden font-mono text-xs md:table-cell">{u.mac}</TableCell>
                <TableCell className="hidden text-sm lg:table-cell">{[u.channel, u.order_ref].filter(Boolean).join(' / ') || '—'}</TableCell>
                <TableCell className="hidden text-sm lg:table-cell">{u.warranty_until ? <span className={u.in_warranty ? '' : 'text-muted-foreground'}>{fmtDate(u.warranty_until)}{u.in_warranty ? '' : '（已过保）'}</span> : '—'}</TableCell>
                <TableCell className="hidden text-sm xl:table-cell">{u.batch}</TableCell>
              </TableRow>
            ))}
            {!list.loading && items.length === 0 && (
              <TableRow><TableCell colSpan={7} className="py-10 text-center text-muted-foreground">没有符合条件的设备</TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>第 {page + 1} 页 / 共 {Math.max(1, Math.ceil(total / PAGE))} 页</span>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={page === 0} onClick={() => set('page', String(page - 1))}>上一页</Button>
          <Button variant="outline" size="sm" disabled={(page + 1) * PAGE >= total} onClick={() => set('page', String(page + 1))}>下一页</Button>
        </div>
      </div>

      <ActionDialog action={action} serials={[...selected]} onClose={() => setAction(null)}
        onDone={() => { list.reload(); setSelected(new Set()) }} />
    </div>
  )
}
