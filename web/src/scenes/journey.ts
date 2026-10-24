// 小朋友的一段旅程：在房间里开箱、把玩偶放上桌面唤醒，再抱着它去公园，也能回家。
// 只属于 Web 展示（相当于玩偶之外的"世界"），不进协议；设备本身不知道自己在哪个场景。
import { create } from 'zustand'

export type Place = 'room' | 'park'
/** boxed 盒子没开 → open 盒盖打开 → held 抱在手里 → placed 放在桌面上（通电）。 */
export type Phase = 'boxed' | 'open' | 'held' | 'placed'

/** 站在离按键最近的互动点上时画面下方的提示；enabled 为 false 只告诉孩子还差什么。 */
export interface Prompt {
  label: string
  enabled: boolean
}

interface JourneyState {
  place: Place
  phase: Phase
  /** 进入房间时站在哪：第一次在房间中央，从公园回来站在门口。 */
  arrival: 'start' | 'door'
  prompt: Prompt | null
  /** 已点过门卡的"进入"：在这之前不抢鼠标、不响应走路。 */
  entered: boolean
  /** 鼠标已锁在画面里（FPS 视角）。 */
  locked: boolean
  /** 转场中：遮住画面并显示这句话。 */
  fade: string | null
  setPhase: (p: Phase) => void
  setPrompt: (p: Prompt | null) => void
  setEntered: (e: boolean) => void
  setLocked: (l: boolean) => void
  travel: (to: Place) => void
}

const FADE_MS = 600

export const useJourney = create<JourneyState>((set, get) => ({
  place: 'room',
  phase: 'boxed',
  arrival: 'start',
  prompt: null,
  entered: false,
  locked: false,
  fade: null,
  setPhase: (phase) => set({ phase }),
  setEntered: (entered) => set({ entered }),
  setLocked: (locked) => set({ locked }),
  setPrompt: (prompt) => {
    const cur = get().prompt
    if (cur?.label === prompt?.label && cur?.enabled === prompt?.enabled) return
    set({ prompt })
  },
  // 先遮住，换场景，再放开：两边各留一个渐变的时间。玩偶一直抱在怀里。
  travel: (to) => {
    if (get().fade) return
    set({ fade: to === 'park' ? '去公园啦…' : '回家啦…', prompt: null })
    window.setTimeout(() => {
      set({ place: to, phase: 'held', arrival: to === 'room' ? 'door' : 'start' })
      window.setTimeout(() => set({ fade: null }), FADE_MS / 2)
    }, FADE_MS)
  },
}))
