import { create } from 'zustand'
import type { Hello, LightState, PalpableMessage } from '../shared/protocol.gen'
import { audioSupported, MicCapture, Player } from './audio'

export type Link = 'offline' | 'connecting' | 'online'

interface DeviceState {
  deviceId: string
  link: Link
  sessionId: string | null
  light: LightState
  lastError: string | null
  micOn: boolean
  speaking: boolean
  connect: (deviceId: string) => void
  disconnect: () => void
  wake: () => void
  setMic: (on: boolean) => Promise<void>
}

let socket: WebSocket | null = null
const mic = new MicCapture()
const player = new Player()

const hello = (deviceId: string): Hello => ({
  type: 'hello',
  device_id: deviceId,
  firmware: 'web-sim-0.1',
  audio_params: { format: 'opus', sample_rate: 16000, channels: 1, frame_duration: 60 },
  capabilities: ['mic', 'speaker', 'light', 'haptic', 'proximity', 'location', 'wake'],
})

// 虚拟设备：只发协议里的上行消息、只响应下行消息，不含任何业务判断。
export const useDevice = create<DeviceState>((set, get) => ({
  deviceId: '',
  link: 'offline',
  sessionId: null,
  light: 'idle',
  lastError: null,
  micOn: false,
  speaking: false,

  connect: (deviceId) => {
    if (socket) return
    set({ deviceId, link: 'connecting', lastError: null })
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/ws`)
    ws.binaryType = 'arraybuffer'
    socket = ws

    ws.onopen = () => ws.send(JSON.stringify(hello(deviceId)))
    ws.onmessage = (ev) => {
      if (typeof ev.data !== 'string') {
        player.push(ev.data as ArrayBuffer)
        return
      }
      const msg = JSON.parse(ev.data) as PalpableMessage
      switch (msg.type) {
        case 'hello.ack':
          set({ link: 'online', sessionId: msg.session_id })
          break
        case 'light':
          set({ light: msg.state })
          break
        case 'audio.downlink':
          player.configure(msg.sample_rate)
          break
        case 'speaker.state':
          set({ speaking: msg.state === 'start' })
          if (msg.state === 'stop') player.reset()
          break
        case 'error':
          set({ lastError: `${msg.code}: ${msg.message}` })
          break
      }
    }
    ws.onclose = () => {
      socket = null
      mic.stop()
      player.reset()
      set({ link: 'offline', sessionId: null, light: 'idle', micOn: false, speaking: false })
    }
  },

  disconnect: () => socket?.close(),

  wake: () => {
    if (get().link !== 'online') return
    socket?.send(JSON.stringify({ type: 'wake', source: 'touch' }))
    void get().setMic(true)
  },

  setMic: async (on) => {
    if (get().link !== 'online' || get().micOn === on) return
    if (!on) {
      mic.stop()
      socket?.send(JSON.stringify({ type: 'mic.state', state: 'stop' }))
      set({ micOn: false })
      return
    }
    if (!audioSupported()) {
      set({ lastError: '当前浏览器不支持 WebCodecs，请使用 Chrome / Edge' })
      return
    }
    try {
      await mic.start((frame) => {
        if (socket?.readyState === WebSocket.OPEN) socket.send(frame)
      })
    } catch (e) {
      set({ lastError: `无法打开麦克风：${e instanceof Error ? e.message : String(e)}` })
      return
    }
    socket?.send(JSON.stringify({ type: 'mic.state', state: 'start' }))
    set({ micOn: true })
  },
}))
