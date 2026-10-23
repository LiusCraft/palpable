// 一台虚拟设备：只发协议里的上行消息、只响应下行消息，不含任何业务判断。
// Web 模拟器与未来固件是同一个协议的两个客户端；跨设备信息只来自服务端下行。
import { createStore, type StoreApi } from 'zustand/vanilla'
import type { Capability, Hello, LightState, MAC, PalpableMessage, Proximity } from '../shared/protocol.gen'
import { copy } from '../ui/copy'
import { playCue } from '../ui/sounds'
import { audioSupported, FrameEncoder, Player } from './audio'
import { lookAround } from './environment'
import { handleMcp, type JsonRpc } from './mcp'
import { PREROLL_FRAMES, Vad } from './vad'

export type Link = 'offline' | 'connecting' | 'online'

export interface Friend {
  deviceId: string
  name: string
  status: 'pending' | 'confirmed'
}

export interface DeviceView {
  mac: MAC
  skin: string
  link: Link
  sessionId: string | null
  light: LightState
  /** 设备工具 self.light.set 临时指定的灯色；为空时按 light 与频道色。 */
  lightOverride: { color: string; pattern: string } | null
  speaking: boolean
  source: 'ai' | 'peer' | null
  peer: { from: string; name: string } | null
  micEnabled: boolean
  talking: boolean // VAD 判定正在说话（或按住说话）
  channel: { id: string; name: string; color: string } | null
  present: Record<string, { name: string; channel: string }> // 同频道在场的好友（服务端已过滤）
  friends: Record<string, Friend> // 来自 friend.state
  toast: { text: string; at: number } | null
  shakeAt: number
  lastError: string | null
}

const CAPABILITIES: Capability[] = ['mic', 'speaker', 'light', 'haptic', 'proximity', 'location', 'wake', 'mcp']
const LOCATION_INTERVAL = 1000

export class VirtualDevice {
  readonly store: StoreApi<DeviceView>
  private ws: WebSocket | null = null
  private player = new Player()
  private encoder: FrameEncoder | null = null
  private vad = new Vad()
  private preroll: Float32Array[] = []
  private pushToTalk = false
  private lastLocationAt = 0
  private overrideTimer = 0

  constructor(mac: MAC, skin: string) {
    this.store = createStore<DeviceView>(() => ({
      mac,
      skin,
      link: 'offline',
      sessionId: null,
      light: 'idle',
      lightOverride: null,
      speaking: false,
      source: null,
      peer: null,
      micEnabled: false,
      talking: false,
      channel: null,
      present: {},
      friends: {},
      toast: null,
      shakeAt: 0,
      lastError: null,
    }))
  }

  get mac(): MAC {
    return this.store.getState().mac
  }

  private set(patch: Partial<DeviceView>): void {
    this.store.setState(patch)
  }

  private toast(text: string): void {
    this.set({ toast: { text, at: Date.now() } })
  }

  connect(): void {
    if (this.ws) return
    const { mac, skin } = this.store.getState()
    this.set({ link: 'connecting', lastError: null })
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/ws`)
    ws.binaryType = 'arraybuffer'
    this.ws = ws

    const hello: Hello = {
      type: 'hello',
      device_id: mac,
      skin,
      firmware: 'web-sim-0.2',
      audio_params: { format: 'opus', sample_rate: 16000, channels: 1, frame_duration: 60 },
      capabilities: CAPABILITIES,
    }
    ws.onopen = () => ws.send(JSON.stringify(hello))
    ws.onmessage = (ev) => {
      if (typeof ev.data !== 'string') {
        this.player.push(ev.data as ArrayBuffer)
        return
      }
      this.onMessage(JSON.parse(ev.data) as PalpableMessage)
    }
    ws.onclose = () => {
      this.ws = null
      this.stopTalking(false)
      this.player.flush()
      this.set({ link: 'offline', sessionId: null, light: 'idle', speaking: false, source: null, peer: null, channel: null, present: {} })
    }
  }

  disconnect(): void {
    this.ws?.close()
  }

  /** 换皮套：皮套由硬件识别，在 hello 里上报；在线时重新上线让服务端按新皮套切换风格。 */
  setSkin(skin: string): void {
    this.set({ skin })
    const ws = this.ws
    if (!ws) return
    ws.addEventListener('close', () => this.connect(), { once: true })
    ws.close()
  }

  dispose(): void {
    this.disconnect()
    this.encoder?.close()
    this.player.close()
  }

  private send(msg: PalpableMessage): void {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(msg))
  }

  private onMessage(msg: PalpableMessage): void {
    switch (msg.type) {
      case 'hello.ack':
        this.set({ link: 'online', sessionId: msg.session_id })
        break
      case 'audio.downlink':
        this.player.configure(msg.sample_rate)
        break
      case 'light':
        this.set({ light: msg.state })
        break
      case 'speaker.state': {
        const src = msg.source
        if (msg.state === 'start') {
          // 同一时刻只有一种来源：换来源时清掉排队的旧音频。
          if (this.store.getState().source !== src) this.player.flush()
          this.set({
            speaking: true,
            source: src,
            peer: src === 'peer' ? { from: msg.from ?? '', name: msg.name ?? copy.friend.someone } : null,
            light: src === 'peer' ? 'peer_talking' : this.store.getState().light,
          })
        } else if (this.store.getState().source === src) {
          if (src === 'peer') this.player.flush()
          this.set({ speaking: false, source: null, peer: null, light: src === 'peer' ? 'idle' : this.store.getState().light })
        }
        break
      }
      case 'haptic':
        this.shake()
        break
      case 'sound':
        playCue(msg.name)
        if (msg.name === 'busy') this.toast(copy.park.busy)
        break
      case 'channel.state':
        if (msg.event === 'left') {
          this.set({ channel: null, present: {} })
        } else {
          this.set({ channel: { id: msg.channel, name: msg.name, color: msg.color ?? '#4caf50' }, present: {} })
        }
        break
      case 'presence': {
        const present = { ...this.store.getState().present }
        if (msg.event === 'join') {
          present[msg.device_id] = { name: msg.name || copy.friend.someone, channel: msg.channel }
          this.toast(copy.park.friendJoined(msg.name || copy.friend.someone))
        } else {
          delete present[msg.device_id]
        }
        this.set({ present })
        break
      }
      case 'friend.state': {
        const friends = { ...this.store.getState().friends }
        const name = msg.name || copy.friend.someone
        if (msg.event === 'removed') {
          delete friends[msg.device_id]
        } else {
          friends[msg.device_id] = { deviceId: msg.device_id, name, status: msg.event }
          this.toast(msg.event === 'pending' ? copy.park.friendPending(name) : copy.park.friendMade)
        }
        this.set({ friends })
        break
      }
      case 'mcp': {
        const reply = handleMcp(msg.payload as unknown as JsonRpc, {
          setLight: (color, pattern) => this.overrideLight(color, pattern),
          haptic: () => this.shake(),
          look: () => lookAround(this.mac),
        })
        if (reply) this.send({ type: 'mcp', payload: reply as unknown as Record<string, unknown> })
        break
      }
      case 'error':
        this.set({ lastError: `${msg.code}: ${msg.message}` })
        break
    }
  }

  private shake(): void {
    this.set({ shakeAt: Date.now() })
    navigator.vibrate?.(120)
  }

  private overrideLight(color: string, pattern: string): void {
    window.clearTimeout(this.overrideTimer)
    this.set({ lightOverride: { color, pattern } })
    this.overrideTimer = window.setTimeout(() => this.set({ lightOverride: null }), 15000)
  }

  /** 点一下玩偶：本地唤醒，接下来这句话说给小伙伴（AI）。 */
  wake(): void {
    if (this.store.getState().link !== 'online') return
    this.send({ type: 'wake', source: 'touch' })
  }

  setMicEnabled(on: boolean): void {
    if (on && !audioSupported()) {
      this.set({ lastError: copy.device.errorUnsupported })
      return
    }
    if (!on) this.stopTalking(true)
    this.set({ micEnabled: on })
  }

  /** 按住说话：不等 VAD，直接开始一句。 */
  setPushToTalk(on: boolean): void {
    this.pushToTalk = on
    if (on) this.startTalking([])
    else this.stopTalking(true)
  }

  /** 麦克风送来的一帧（由 MicHub 只送给当前持有麦克风的设备）。 */
  onPCM(pcm: Float32Array, rms: number): void {
    const s = this.store.getState()
    if (s.link !== 'online' || (!s.micEnabled && !this.pushToTalk)) return
    if (this.pushToTalk) {
      this.encode(pcm)
      return
    }
    const change = this.vad.push(rms)
    if (change === 'start') {
      this.startTalking(this.preroll)
      this.preroll = []
    }
    if (this.vad.active) {
      this.encode(pcm)
    } else {
      this.preroll.push(pcm)
      if (this.preroll.length > PREROLL_FRAMES) this.preroll.shift()
    }
    if (change === 'stop') this.stopTalking(true)
  }

  private startTalking(preroll: Float32Array[]): void {
    if (this.store.getState().talking) return
    this.encoder ??= new FrameEncoder((opus) => {
      if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(opus)
    })
    this.send({ type: 'mic.state', state: 'start' })
    this.set({ talking: true })
    for (const f of preroll) this.encode(f)
  }

  private stopTalking(notify: boolean): void {
    this.vad.reset()
    this.preroll = []
    if (!this.store.getState().talking) return
    if (notify) this.send({ type: 'mic.state', state: 'stop' })
    this.set({ talking: false })
  }

  private encode(pcm: Float32Array): void {
    this.encoder?.encode(pcm)
  }

  /** 定位：由场景里的位置给出（真实设备是 GPS）；限频上报。 */
  reportLocation(lat: number, lng: number, force = false): void {
    const now = Date.now()
    if (!force && now - this.lastLocationAt < LOCATION_INTERVAL) return
    this.lastLocationAt = now
    this.send({ type: 'location', lat, lng })
  }

  /** 靠近发现：一次完整的扫描结果（真实设备是 BLE / NFC）。 */
  reportProximity(peers: { deviceId: string; strength: number }[]): void {
    const list = peers.slice(0, 16).map((p) => ({ device_id: p.deviceId, strength: Math.round(p.strength * 100) / 100 }))
    this.send({ type: 'proximity', peers: list as Proximity['peers'] })
  }
}
