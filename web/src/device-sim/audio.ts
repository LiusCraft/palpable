// 设备的麦克风与扬声器：采集 16kHz 单声道、每 60ms 编码为一帧 Opus；播放时反向解码。
// 与协议约定一致（见 protocol/README.md）。依赖浏览器的 WebCodecs（Chromium 系）。

const SAMPLE_RATE = 16000
const FRAME_SAMPLES = 960 // 60ms

const WORKLET_SRC = `
class FrameCollector extends AudioWorkletProcessor {
  constructor() { super(); this.buf = new Float32Array(${FRAME_SAMPLES}); this.n = 0 }
  process(inputs) {
    const ch = inputs[0][0]
    if (!ch) return true
    for (let i = 0; i < ch.length; i++) {
      this.buf[this.n++] = ch[i]
      if (this.n === ${FRAME_SAMPLES}) {
        this.port.postMessage(this.buf.slice(0))
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

export class MicCapture {
  private ctx: AudioContext | null = null
  private stream: MediaStream | null = null
  private encoder: AudioEncoder | null = null

  async start(onFrame: (opus: ArrayBuffer) => void): Promise<void> {
    if (this.ctx) return
    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
    })
    const ctx = new AudioContext({ sampleRate: SAMPLE_RATE })
    this.ctx = ctx

    const encoder = new AudioEncoder({
      output: (chunk) => {
        const out = new ArrayBuffer(chunk.byteLength)
        chunk.copyTo(out)
        onFrame(out)
      },
      error: (e) => console.error('opus encoder', e),
    })
    encoder.configure({
      codec: 'opus',
      sampleRate: SAMPLE_RATE,
      numberOfChannels: 1,
      bitrate: 24000,
      opus: { frameDuration: 60000 },
    })
    this.encoder = encoder

    const url = URL.createObjectURL(new Blob([WORKLET_SRC], { type: 'text/javascript' }))
    await ctx.audioWorklet.addModule(url)
    URL.revokeObjectURL(url)

    const node = new AudioWorkletNode(ctx, 'frame-collector')
    let timestamp = 0
    node.port.onmessage = (ev: MessageEvent<Float32Array<ArrayBuffer>>) => {
      if (encoder.state !== 'configured') return
      const pcm = ev.data
      encoder.encode(
        new AudioData({
          format: 'f32-planar',
          sampleRate: SAMPLE_RATE,
          numberOfFrames: pcm.length,
          numberOfChannels: 1,
          timestamp,
          data: pcm,
        }),
      )
      timestamp += 60000
    }
    ctx.createMediaStreamSource(this.stream).connect(node)
  }

  stop(): void {
    this.stream?.getTracks().forEach((t) => t.stop())
    if (this.encoder && this.encoder.state !== 'closed') this.encoder.close()
    void this.ctx?.close()
    this.stream = null
    this.encoder = null
    this.ctx = null
  }
}

export class Player {
  private ctx: AudioContext | null = null
  private decoder: AudioDecoder | null = null
  private nextTime = 0
  private timestamp = 0

  private ensure(): void {
    if (this.ctx) return
    const ctx = new AudioContext({ sampleRate: SAMPLE_RATE })
    this.ctx = ctx
    this.decoder = new AudioDecoder({
      output: (data) => {
        const pcm = new Float32Array(data.numberOfFrames)
        data.copyTo(pcm, { planeIndex: 0, format: 'f32-planar' })
        data.close()
        const buf = ctx.createBuffer(1, pcm.length, SAMPLE_RATE)
        buf.copyToChannel(pcm, 0)
        const src = ctx.createBufferSource()
        src.buffer = buf
        src.connect(ctx.destination)
        const at = Math.max(ctx.currentTime + 0.05, this.nextTime)
        src.start(at)
        this.nextTime = at + buf.duration
      },
      error: (e) => console.error('opus decoder', e),
    })
    this.decoder.configure({ codec: 'opus', sampleRate: SAMPLE_RATE, numberOfChannels: 1 })
  }

  push(frame: ArrayBuffer): void {
    this.ensure()
    if (this.ctx?.state === 'suspended') void this.ctx.resume()
    this.decoder?.decode(
      new EncodedAudioChunk({ type: 'key', timestamp: this.timestamp, data: frame }),
    )
    this.timestamp += 60000
  }

  /** 被打断或一句话结束后清掉排队的音频。 */
  reset(): void {
    this.nextTime = 0
  }

  close(): void {
    if (this.decoder && this.decoder.state !== 'closed') this.decoder.close()
    void this.ctx?.close()
    this.decoder = null
    this.ctx = null
  }
}
