// 演示进度：当前展示哪个场景、看的是哪个小朋友的玩偶。只属于 Web 展示，不进协议。
import { create } from 'zustand'

export type Scene = 'unbox' | 'desk' | 'park'

interface DemoState {
  scene: Scene
  viewing: string | null // 舞台上展示的玩偶 MAC；为空时用麦克风所在的那只
  setScene: (s: Scene) => void
  view: (mac: string) => void
}

export const useDemo = create<DemoState>((set) => ({
  scene: 'unbox',
  viewing: null,
  setScene: (scene) => set({ scene }),
  view: (viewing) => set({ viewing }),
}))
