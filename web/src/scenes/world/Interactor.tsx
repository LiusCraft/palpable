import { useFrame } from '@react-three/fiber'
import { useEffect, useRef, type MutableRefObject } from 'react'
import { useJourney } from '../journey'

// 互动点：走到附近，画面下方出现"按 E …"，按 E 才发生。房间与公园共用。

export interface Spot {
  x: number
  z: number
  /** 离互动点中心多远算"在那个位置"。 */
  r: number
  label: string
  /** false：只显示提示（还差什么），按 E 不会发生任何事。 */
  enabled: boolean
  act: () => void
}

export function Interactor({ spots, pos }: { spots: Spot[]; pos: MutableRefObject<[number, number]> }) {
  const live = useRef<Spot | null>(null)
  const all = useRef(spots)
  useEffect(() => {
    all.current = spots
  })

  useFrame(() => {
    const [px, pz] = pos.current
    let best: Spot | null = null
    let bestD = Infinity
    for (const s of all.current) {
      const d = Math.hypot(px - s.x, pz - s.z)
      if (d <= s.r && d < bestD) {
        best = s
        bestD = d
      }
    }
    live.current = best
    useJourney.getState().setPrompt(best ? { label: best.label, enabled: best.enabled } : null)
  })

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.code !== 'KeyE' || e.repeat) return
      if (e.target instanceof HTMLElement && /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)) return
      const s = live.current
      if (s?.enabled && !useJourney.getState().fade) s.act()
    }
    window.addEventListener('keydown', down)
    return () => {
      window.removeEventListener('keydown', down)
      useJourney.getState().setPrompt(null)
    }
  }, [])
  return null
}
