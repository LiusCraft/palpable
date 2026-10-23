import type { ReactNode } from 'react'

// 页面通用的卡片、步骤徽标与标签，样式对齐 #93 的效果图：浅蓝底、白色圆角卡片、深蓝步骤圆标。

export function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <section className={`rounded-3xl bg-white/90 p-5 shadow-[0_8px_30px_rgba(40,80,160,0.08)] ring-1 ring-sky-100 ${className}`}>{children}</section>
}

export function StepBadge({ n, active = true }: { n: number; active?: boolean }) {
  return (
    <span
      className={`inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold ${
        active ? 'bg-[#1d3557] text-white' : 'bg-slate-200 text-slate-500'
      }`}
    >
      {n}
    </span>
  )
}

export function StepHeader({ n, title, sub, tag }: { n: number; title: string; sub: string; tag?: string }) {
  return (
    <header className="mb-3 flex items-start gap-3">
      <StepBadge n={n} />
      <div className="min-w-0">
        <h2 className="text-xl font-bold text-[#1d3557]">
          {title}
          {tag && <span className="ml-1 text-base font-medium text-slate-500">（{tag}）</span>}
        </h2>
        <p className="mt-0.5 text-sm text-slate-500">{sub}</p>
      </div>
    </header>
  )
}

export function Chip({ children, tone = 'blue' }: { children: ReactNode; tone?: 'blue' | 'green' | 'pink' | 'gray' | 'red' }) {
  const tones = {
    blue: 'bg-sky-50 text-sky-700 ring-sky-200',
    green: 'bg-emerald-50 text-emerald-700 ring-emerald-200',
    pink: 'bg-pink-50 text-pink-600 ring-pink-200',
    gray: 'bg-slate-50 text-slate-500 ring-slate-200',
    red: 'bg-rose-50 text-rose-600 ring-rose-200',
  }
  return <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ${tones[tone]}`}>{children}</span>
}

export function Button({
  children,
  onClick,
  tone = 'primary',
  disabled,
  className = '',
}: {
  children: ReactNode
  onClick?: () => void
  tone?: 'primary' | 'ghost' | 'pink'
  disabled?: boolean
  className?: string
}) {
  const tones = {
    primary: 'bg-[#2f6bff] text-white hover:bg-[#2559d9] shadow-sm',
    ghost: 'bg-white text-slate-600 ring-1 ring-slate-200 hover:bg-slate-50',
    pink: 'bg-pink-500 text-white hover:bg-pink-600 shadow-sm',
  }
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`rounded-full px-4 py-1.5 text-sm font-medium transition disabled:opacity-40 ${tones[tone]} ${className}`}
    >
      {children}
    </button>
  )
}

/** 说话气泡：left 为小伙伴，right 为小朋友。 */
export function Bubble({ side, children, tone }: { side: 'left' | 'right'; children: ReactNode; tone?: 'blue' | 'pink' }) {
  const color =
    side === 'right' ? (tone === 'pink' ? 'bg-pink-400 text-white' : 'bg-[#2f6bff] text-white') : 'bg-slate-100 text-slate-700'
  return (
    <div className={`flex ${side === 'right' ? 'justify-end' : 'justify-start'}`}>
      <div className={`max-w-[80%] rounded-2xl px-3 py-2 text-sm leading-relaxed ${color}`}>{children}</div>
    </div>
  )
}

/** 说话时的声波动画。 */
export function Wave({ active, tone = 'blue' }: { active: boolean; tone?: 'blue' | 'pink' }) {
  const color = tone === 'pink' ? 'bg-pink-400' : 'bg-sky-400'
  return (
    <span className="inline-flex h-4 items-end gap-0.5" aria-hidden>
      {[0, 1, 2, 3, 4].map((i) => (
        <span
          key={i}
          className={`w-0.5 rounded ${color} ${active ? 'animate-[wave_0.9s_ease-in-out_infinite]' : ''}`}
          style={{ height: active ? undefined : 4, animationDelay: `${i * 0.12}s` }}
        />
      ))}
    </span>
  )
}
