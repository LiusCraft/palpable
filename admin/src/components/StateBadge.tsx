import { Lock } from 'lucide-react'
import type { Unit } from '@/lib/api'
import { STATE_LABEL, STATE_TONE } from '@/lib/labels'
import { cn } from '@/lib/utils'

export function StateBadge({ unit }: { unit: Pick<Unit, 'state' | 'locked'> }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn('rounded-md px-2 py-0.5 text-xs font-medium', STATE_TONE[unit.state])}>{STATE_LABEL[unit.state]}</span>
      {unit.locked && (
        <span className="inline-flex items-center gap-1 rounded-md bg-red-600 px-2 py-0.5 text-xs font-medium text-white">
          <Lock className="size-3" />已锁定
        </span>
      )}
    </span>
  )
}
