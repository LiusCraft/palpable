import { ArrowLeft, Repeat } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ActionDialog } from '@/components/ActionDialog'
import { StateBadge } from '@/components/StateBadge'
import { useLoad } from '@/hooks/useLoad'
import { api, type ActionName } from '@/lib/api'
import { ACTION_LABEL, DANGEROUS, EVENT_LABEL, fmtDate, fmtTime, GRADE_LABEL, LOCK_LABEL, STATE_LABEL } from '@/lib/labels'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

function Row({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b py-2 text-sm last:border-0">
      <dt className="text-muted-foreground">{k}</dt>
      <dd className="text-right">{children}</dd>
    </div>
  )
}

export function UnitDetail() {
  const { serial = '' } = useParams()
  const d = useLoad(() => api.unit(serial), [serial])
  const [action, setAction] = useState<ActionName | null>(null)
  const u = d.data?.unit

  return (
    <div className="grid gap-4">
      <Link to="/units" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"><ArrowLeft className="size-4" />设备台账</Link>
      {d.error && <p role="alert" className="text-sm text-destructive">{d.error}</p>}
      {u && (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="font-mono text-2xl font-semibold">{u.serial}</h1>
            <StateBadge unit={u} />
            {u.transfer_count > 0 && <Badge variant="secondary" className="gap-1"><Repeat className="size-3" />二手，换过 {u.transfer_count} 次主人</Badge>}
            {u.grade !== 'new' && <Badge variant="secondary">{GRADE_LABEL[u.grade]}</Badge>}
          </div>

          {u.actions.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {u.actions.map((a) => (
                <Button key={a} variant={DANGEROUS.includes(a) ? 'destructive' : 'outline'} size="sm" onClick={() => setAction(a)}>{ACTION_LABEL[a]}</Button>
              ))}
            </div>
          )}

          <div className="grid gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader><CardTitle>设备信息</CardTitle></CardHeader>
              <CardContent>
                <dl>
                  <Row k="MAC"><span className="font-mono">{u.mac}</span></Row>
                  <Row k="密钥指纹"><span className="font-mono">{u.key_fingerprint}</span></Row>
                  <Row k="批次"><Link to={`/units?batch=${u.batch}`} className="text-primary hover:underline">{u.batch}</Link></Row>
                  <Row k="皮套 / 固件">{u.skin} / {u.firmware || '—'}</Row>
                  <Row k="翻新次数">{u.refurb_count}</Row>
                  <Row k="入库时间">{fmtTime(u.created_at)}</Row>
                </dl>
              </CardContent>
            </Card>
            <Card>
              <CardHeader><CardTitle>销售与归属</CardTitle></CardHeader>
              <CardContent>
                <dl>
                  <Row k="渠道">{u.channel || '—'}</Row>
                  <Row k="订单号">{u.order_ref || '—'}</Row>
                  <Row k="售出日期">{fmtDate(u.sold_at)}</Row>
                  <Row k="保修至">{u.warranty_until ? `${fmtDate(u.warranty_until)}${u.in_warranty ? '' : '（已过保）'}` : '—'}</Row>
                  <Row k="当前有家长绑定">{u.has_owner ? '是' : '否'}</Row>
                  {u.locked && <Row k="锁定原因"><span className="text-red-600">{LOCK_LABEL[u.lock_reason] ?? u.lock_reason}</span></Row>}
                  {u.resume_state && <Row k="结束后回到">{STATE_LABEL[u.resume_state]}</Row>}
                </dl>
                <p className="mt-3 text-xs text-muted-foreground">后台看不到家长和孩子的任何信息，只知道“有没有人绑定”以及“换过几次主人”。</p>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader><CardTitle>履历</CardTitle></CardHeader>
            <CardContent>
              <ol className="relative ml-2 grid gap-4 border-l pl-5">
                {[...(d.data?.events ?? [])].reverse().map((e) => (
                  <li key={e.id} className="relative text-sm">
                    <span className={`absolute -left-[1.6rem] top-1.5 size-2.5 rounded-full ${e.anomaly ? 'bg-amber-500' : 'bg-primary'}`} />
                    <p className="flex flex-wrap items-center gap-x-2">
                      <span className="font-medium">{EVENT_LABEL[e.action] ?? e.action}</span>
                      {e.from && e.to && e.from !== e.to && <span className="text-muted-foreground">{STATE_LABEL[e.from]} → {STATE_LABEL[e.to]}</span>}
                      {e.anomaly && <Badge variant="outline" className="border-amber-500 text-amber-700">{e.resolved_at ? '异常（已核对）' : '异常'}</Badge>}
                    </p>
                    {e.note && <p className="text-muted-foreground">{e.note}</p>}
                    <p className="text-xs text-muted-foreground">{fmtTime(e.at)} · {e.actor}{e.resolved_by ? ` · 核对人 ${e.resolved_by}` : ''}</p>
                  </li>
                ))}
              </ol>
            </CardContent>
          </Card>

          <ActionDialog action={action} serials={[u.serial]} onClose={() => setAction(null)} onDone={d.reload} />
        </>
      )}
    </div>
  )
}
