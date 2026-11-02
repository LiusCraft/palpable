import { AlertTriangle, Lock } from 'lucide-react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { useLoad } from '@/hooks/useLoad'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { EVENT_LABEL, fmtTime, STATE_LABEL, STATE_ORDER, STATE_TONE } from '@/lib/labels'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

export function Dashboard() {
  const { me } = useAuth()
  const stats = useLoad(() => api.stats(), [])
  const anomalies = useLoad(() => api.events({ anomaly: true, limit: 20 }), [])
  const recent = useLoad(() => api.events({ limit: 12 }), [])

  async function resolve(id: number) {
    try {
      await api.resolve(id)
      toast.success('已标记为核对完成')
      anomalies.reload()
      stats.reload()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-2xl font-semibold">概览</h1>
        <p className="text-sm text-muted-foreground">每台设备从生产到报废所处的环节。点击卡片查看该环节的设备。</p>
      </div>

      {stats.error && <p role="alert" className="text-sm text-destructive">{stats.error}</p>}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-7">
        {stats.loading && !stats.data
          ? Array.from({ length: 7 }, (_, i) => <Skeleton key={i} className="h-20" />)
          : STATE_ORDER.map((s) => (
              <Link key={s} to={`/units?state=${s}`}>
                <Card className="gap-1 py-4 transition-shadow hover:shadow-md">
                  <CardContent className="px-4">
                    <span className={cn('rounded-md px-2 py-0.5 text-xs font-medium', STATE_TONE[s])}>{STATE_LABEL[s]}</span>
                    <p className="mt-2 text-2xl font-semibold tabular-nums">{stats.data?.by_state[s] ?? 0}</p>
                  </CardContent>
                </Card>
              </Link>
            ))}
      </div>
      {stats.data && (
        <p className="text-sm text-muted-foreground">
          共 {stats.data.total} 台
          {stats.data.locked > 0 && <Link to="/units?locked=1" className="ml-3 inline-flex items-center gap-1 text-red-600"><Lock className="size-3.5" />{stats.data.locked} 台已锁定</Link>}
        </p>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><AlertTriangle className="size-4 text-amber-600" />待核对的异常</CardTitle>
          <CardDescription>例如设备还没登记售出就被家长激活（串货、内部流出），或已报废的设备又连上来。核对后标记完成。</CardDescription>
        </CardHeader>
        <CardContent>
          {anomalies.data?.length === 0 && <p className="text-sm text-muted-foreground">没有待核对的异常。</p>}
          <ul className="grid gap-2">
            {anomalies.data?.map((e) => (
              <li key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border p-3 text-sm">
                <Link to={`/units/${e.serial}`} className="font-mono text-primary hover:underline">{e.serial}</Link>
                <span>{EVENT_LABEL[e.action] ?? e.action}</span>
                <span className="text-muted-foreground">{e.note}</span>
                <span className="ml-auto text-xs text-muted-foreground">{fmtTime(e.at)}</span>
                {me?.can_resolve && <Button size="sm" variant="outline" onClick={() => void resolve(e.id)}>已核对</Button>}
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>最近动态</CardTitle></CardHeader>
        <CardContent>
          <ul className="grid gap-1.5 text-sm">
            {recent.data?.map((e) => (
              <li key={e.id} className="flex flex-wrap gap-x-3">
                <span className="w-40 shrink-0 text-xs text-muted-foreground">{fmtTime(e.at)}</span>
                <Link to={`/units/${e.serial}`} className="font-mono text-primary hover:underline">{e.serial}</Link>
                <span>{EVENT_LABEL[e.action] ?? e.action}</span>
                <span className="text-muted-foreground">{e.actor}</span>
              </li>
            ))}
            {recent.data?.length === 0 && <li className="text-muted-foreground">还没有任何记录。先到“生产批次”导入第一批设备。</li>}
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}
