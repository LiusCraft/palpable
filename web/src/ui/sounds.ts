// 设备提示音：用 WebAudio 合成，不依赖音频文件。名字与协议的 sound.name 一一对应。
import type { Sound } from '../shared/protocol.gen'

type Note = [freq: number, start: number, dur: number]

const CUES: Record<Sound['name'], Note[]> = {
  connected: [[660, 0, 0.12], [880, 0.12, 0.16]],
  joined: [[523, 0, 0.1], [659, 0.1, 0.1], [784, 0.2, 0.18]],
  left: [[784, 0, 0.1], [523, 0.1, 0.18]],
  switched: [[700, 0, 0.08], [700, 0.12, 0.08]],
  error: [[330, 0, 0.2], [262, 0.2, 0.25]],
  busy: [[440, 0, 0.08], [440, 0.14, 0.08], [440, 0.28, 0.08]],
  friend: [[659, 0, 0.1], [784, 0.1, 0.1], [988, 0.2, 0.1], [1319, 0.3, 0.22]],
}

let ctx: AudioContext | null = null

export function playCue(name: Sound['name']): void {
  try {
    ctx ??= new AudioContext()
    if (ctx.state === 'suspended') void ctx.resume()
    const now = ctx.currentTime
    for (const [freq, start, dur] of CUES[name]) {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      osc.type = 'sine'
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.0001, now + start)
      gain.gain.exponentialRampToValueAtTime(0.18, now + start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, now + start + dur)
      osc.connect(gain).connect(ctx.destination)
      osc.start(now + start)
      osc.stop(now + start + dur + 0.02)
    }
  } catch {
    // 没有音频设备时静默。
  }
}
