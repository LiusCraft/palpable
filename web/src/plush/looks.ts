import type { LightState } from '../shared/protocol.gen'

// 灯色与皮套外观：Plush 与 2D 地图头像共用。

export const LIGHT_COLORS: Record<LightState, string> = {
  idle: '#7fb8ff',
  listening: '#3ad0ff',
  thinking: '#ffc24d',
  speaking: '#ff86b6',
  peer_talking: '#5fe08a',
  muted: '#9aa3ad',
}

export interface SkinLook {
  body: string
  belly: string
  ear: 'round' | 'long' | 'triangle' | 'spike'
  earInner: string
  nose: string
  /** 耳朵、手脚的颜色；缺省与身体同色（熊猫是黑耳黑手脚）。 */
  accent?: string
}

export const SKINS: Record<string, SkinLook> = {
  bear: { body: '#b98a5e', belly: '#e8cfae', ear: 'round', earInner: '#e8cfae', nose: '#4a2e1f' },
  rabbit: { body: '#f6ece4', belly: '#ffffff', ear: 'long', earInner: '#ffc2d4', nose: '#e88aa5' },
  dragon: { body: '#7cc36a', belly: '#f3e6c2', ear: 'spike', earInner: '#f2a33c', nose: '#3c6b31' },
  cat: { body: '#34343c', belly: '#4a4a54', ear: 'triangle', earInner: '#ff9fb3', nose: '#e88aa5' },
  panda: { body: '#fbfbfb', belly: '#ffffff', ear: 'round', earInner: '#3a3a42', nose: '#222228', accent: '#1f1f26' },
}
