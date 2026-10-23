// 设备端语音端点检测：只按能量判断"开始说"与"说完"，决定 mic.state 的起止与哪些帧要上行。
// 服务端另有看门狗，这里只要不切碎句子即可。

const START_FRAMES = 2 // 连续 2 帧（120ms）有声才算开始，避免咳嗽、碰撞触发
const HANG_FRAMES = 10 // 静音 10 帧（600ms）才算说完，句中停顿不切断
export const PREROLL_FRAMES = 3 // 开始前补发的帧，保住第一个字

export class Vad {
  active = false
  private loud = 0
  private quiet = 0
  private readonly threshold: number

  constructor(threshold = 0.02) {
    this.threshold = threshold
  }

  /** 返回本帧引起的状态变化。 */
  push(rms: number): 'start' | 'stop' | null {
    const voiced = rms >= this.threshold
    if (!this.active) {
      this.loud = voiced ? this.loud + 1 : 0
      if (this.loud >= START_FRAMES) {
        this.active = true
        this.quiet = 0
        return 'start'
      }
      return null
    }
    this.quiet = voiced ? 0 : this.quiet + 1
    if (this.quiet >= HANG_FRAMES) {
      this.active = false
      this.loud = 0
      return 'stop'
    }
    return null
  }

  reset(): void {
    this.active = false
    this.loud = 0
    this.quiet = 0
  }
}
