// 设备端唤醒词引擎（Web 模拟）：只在本地判断"是不是在叫小伙伴"，命中就回调；
// 要听的词（孩子给小伙伴起的名字）由服务端经 wake.config 下发，命中之后怎么路由也由服务端决定。
// 真实固件换成本地关键词模型，对外行为相同。
// 用浏览器的 SpeechRecognition 模拟，Chrome 的实现会把语音送到浏览器厂商的云端，只用于演示。

const COOLDOWN_MS = 3000 // 同一句话的中间结果会反复命中
const RESTART_MS = 300 // 浏览器每隔一阵会自行结束识别，需要续上

interface Recognition {
  lang: string
  continuous: boolean
  interimResults: boolean
  onresult: ((ev: { resultIndex: number; results: ArrayLike<ArrayLike<{ transcript: string }>> }) => void) | null
  onend: (() => void) | null
  onerror: ((ev: { error: string }) => void) | null
  start(): void
  stop(): void
}

type RecognitionCtor = new () => Recognition

function ctor(): RecognitionCtor | null {
  const w = window as unknown as { SpeechRecognition?: RecognitionCtor; webkitSpeechRecognition?: RecognitionCtor }
  return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null
}

export function wakeWordSupported(): boolean {
  return ctor() !== null
}

function squash(text: string): string {
  return text.replace(/[\s\p{P}]/gu, '')
}

/** 去掉空白和标点后看是否包含任一唤醒词。 */
export function matchesWakeWord(text: string, words: string[]): boolean {
  const t = squash(text)
  return words.some((w) => {
    const s = squash(w)
    return s !== '' && t.includes(s)
  })
}

export class WakeWordEngine {
  private rec: Recognition | null = null
  private want = false
  private enabled = false
  private lastHit = 0
  private words: string[] = []
  private timer = 0

  private readonly onWake: () => void

  constructor(onWake: () => void) {
    this.onWake = onWake
  }

  /** 整体替换要听的词；没有词时不开识别，省得白白听。 */
  setWords(words: string[]): void {
    this.words = words
    if (words.length === 0) this.pause()
    else if (this.enabled) this.start()
  }

  /** 麦克风开着时才该听；词到位之后才真正开始识别。 */
  enable(on: boolean): void {
    this.enabled = on
    if (on && this.words.length > 0) this.start()
    else this.pause()
  }

  private pause(): void {
    this.want = false
    window.clearTimeout(this.timer)
    this.rec?.stop()
    this.rec = null
  }

  private start(): void {
    const C = ctor()
    if (!C || this.want) return
    this.want = true
    const rec = new C()
    rec.lang = 'zh-CN'
    rec.continuous = true
    rec.interimResults = true
    rec.onresult = (ev) => {
      for (let i = ev.resultIndex; i < ev.results.length; i++) {
        if (!matchesWakeWord(ev.results[i][0].transcript, this.words)) continue
        const now = Date.now()
        if (now - this.lastHit < COOLDOWN_MS) return
        this.lastHit = now
        this.onWake()
        return
      }
    }
    rec.onerror = (ev) => {
      // 没有麦克风权限时不再重试，其余错误（无语音、网络抖动）等 onend 续上。
      if (ev.error === 'not-allowed' || ev.error === 'service-not-allowed') this.want = false
    }
    rec.onend = () => {
      this.rec = null
      if (this.want) this.timer = window.setTimeout(() => this.restart(), RESTART_MS)
    }
    this.rec = rec
    this.begin(rec)
  }

  private restart(): void {
    if (!this.want) return
    this.want = false
    this.start()
  }

  private begin(rec: Recognition): void {
    try {
      rec.start()
    } catch {
      // 已经在识别中
    }
  }
}
