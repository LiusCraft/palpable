// 设备的麦克风与扬声器：采集 16kHz 单声道、每 60ms 一帧，按需编码为 Opus；播放时反向解码。
// 与协议约定一致（见 protocol/README.md）。依赖浏览器的 WebCodecs（Chromium 系）。

const SAMPLE_RATE = 16000 // 上行（麦克风）固定 16kHz
const FRAME_SAMPLES = 960 // 60ms
const FRAME_US = 60000
const DEFAULT_DOWNLINK_RATE = 16000 // 下行默认值，收到 audio.downlink 后以服务端为准

const WORKLET_SRC = `
class FrameCollector extends AudioWorkletProcessor {
  constructor() { super(); this.buf = new Float32Array(${FRAME_SAMPLES}); this.n = 0 }
  process(inputs) {
    const ch = inputs[0][0]
    if (!ch) return true
    for (let i = 0; i < ch.length; i++) {
      this.buf[this.n++] = ch[i]
      if (this.n === ${FRAME_SAMPLES}) {
        let sum = 0
        for (let j = 0; j < this.buf.length; j++) sum += this.buf[j] * this.buf[j]
        this.port.postMessage({ pcm: this.buf.slice(0), rms: Math.sqrt(sum / this.buf.length) })
        this.n = 0
      }
    }
    return true
  }
}
registerProcessor('frame-collector', FrameCollector)
`

export function audioSupported(): boolean {
  return typeof AudioEncoder !== 'undefined' && typeof AudioDecoder !== 'undefined'
}

/** 采集 60ms 一帧的 PCM 与能量，由设备决定哪些帧要编码上行（设备端 VAD）。 */
export class MicCapture {
  private ctx: AudioContext | null = null
  private stream: MediaStream | null = null

  get running(): boolean {
    return this.ctx !== null
  }

  async start(onFrame: (pcm: Float32Array, rms: number) => void): Promise<void> {
    if (this.ctx) return
    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    })
    const ctx = new AudioContext({ sampleRate: SAMPLE_RATE })
    this.ctx = ctx
    const url = URL.createObjectURL(new Blob([WORKLET_SRC], { type: 'text/javascript' }))
    await ctx.audioWorklet.addModule(url)
    URL.revokeObjectURL(url)
    const node = new AudioWorkletNode(ctx, 'frame-collector')
    node.port.onmessage = (ev: MessageEvent<{ pcm: Float32Array; rms: number }>) => onFrame(ev.data.pcm, ev.data.rms)
    ctx.createMediaStreamSource(this.stream).connect(node)
  }

  stop(): void {
    this.stream?.getTracks().forEach((t) => t.stop())
    void this.ctx?.close()
    this.stream = null
    this.ctx = null
  }
}

/** 把 PCM 帧编码为 Opus（60ms 一帧）。每台虚拟设备一个，时间戳各自独立。 */
export class FrameEncoder {
  private encoder: AudioEncoder
  private timestamp = 0

  constructor(onOpus: (opus: ArrayBuffer) => void) {
    this.encoder = new AudioEncoder({
      output: (chunk) => {
        const out = new ArrayBuffer(chunk.byteLength)
        chunk.copyTo(out)
        onOpus(out)
      },
      error: (e) => console.error('opus encoder', e),
    })
    this.encoder.configure({
      codec: 'opus',
      sampleRate: SAMPLE_RATE,
      numberOfChannels: 1,
      bitrate: 24000,
      opus: { frameDuration: FRAME_US },
    })
  }

  encode(pcm: Float32Array): void {
    if (this.encoder.state !== 'configured') return
    const data = new Float32Array(pcm)
    this.encoder.encode(
      new AudioData({ format: 'f32-planar', sampleRate: SAMPLE_RATE, numberOfFrames: data.length, numberOfChannels: 1, timestamp: this.timestamp, data }),
    )
    this.timestamp += FRAME_US
  }

  close(): void {
    if (this.encoder.state !== 'closed') this.encoder.close()
  }
}

/**
 * 下行播放。AI 的 TTS 与好友的语音都是 Opus：Opus 解码可以输出任意采样率，
 * 所以同一个解码器按 audio.downlink 的采样率解码两种来源即可。
 */
export class Player {
  private ctx: AudioContext | null = null
  private decoder: AudioDecoder | null = null
  private rate = DEFAULT_DOWNLINK_RATE
  private nextTime = 0
  private timestamp = 0
  private sources = new Set<AudioBufferSourceNode>()

  /** 服务端下行采样率可能是 24kHz；变化时重建解码器与播放上下文，否则声音会变调。 */
  configure(sampleRate: number): void {
    if (sampleRate === this.rate) return
    this.close()
    this.rate = sampleRate
  }

  private ensure(): void {
    if (this.ctx) return
    const rate = this.rate
    const ctx = new AudioContext({ sampleRate: rate })
    this.ctx = ctx
    this.decoder = new AudioDecoder({
      output: (data) => {
        const pcm = new Float32Array(data.numberOfFrames)
        data.copyTo(pcm, { planeIndex: 0, format: 'f32-planar' })
        data.close()
        const buf = ctx.createBuffer(1, pcm.length, rate)
        buf.copyToChannel(pcm, 0)
        const src = ctx.createBufferSource()
        src.buffer = buf
        src.connect(ctx.destination)
        const at = Math.max(ctx.currentTime + 0.05, this.nextTime)
        src.start(at)
        this.nextTime = at + buf.duration
        this.sources.add(src)
        src.onended = () => this.sources.delete(src)
      },
      error: (e) => console.error('opus decoder', e),
    })
    this.decoder.configure({ codec: 'opus', sampleRate: rate, numberOfChannels: 1 })
  }

  push(frame: ArrayBuffer): void {
    this.ensure()
    if (this.ctx?.state === 'suspended') void this.ctx.resume()
    this.decoder?.decode(new EncodedAudioChunk({ type: 'key', timestamp: this.timestamp, data: frame }))
    this.timestamp += FRAME_US
  }

  /** 换来源或被打断时，立即停掉已排队的音频。 */
  flush(): void {
    for (const s of this.sources) {
      try {
        s.stop()
      } catch {
        // 已经停了
      }
    }
    this.sources.clear()
    this.nextTime = 0
  }

  close(): void {
    this.flush()
    if (this.decoder && this.decoder.state !== 'closed') this.decoder.close()
    void this.ctx?.close()
    this.decoder = null
    this.ctx = null
    this.timestamp = 0
  }
}
