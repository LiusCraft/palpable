import { create } from 'zustand'
import type { Hello, LightState, PalpableMessage } from '../shared/protocol.gen'

export type Link = 'offline' | 'connecting' | 'online'

interface DeviceState {
  deviceId: string
  link: Link
  sessionId: string | null
  light: LightState
  lastError: string | null
  connect: (deviceId: string) => void
  disconnect: () => void
  wake: () => void
}

let socket: WebSocket | null = null

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

  connect: (deviceId) => {
    if (socket) return
    set({ deviceId, link: 'connecting', lastError: null })
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/ws`)
    ws.binaryType = 'arraybuffer'
    socket = ws

    ws.onopen = () => ws.send(JSON.stringify(hello(deviceId)))
    ws.onmessage = (ev) => {
      if (typeof ev.data !== 'string') return // 音频帧：M1 处理
      const msg = JSON.parse(ev.data) as PalpableMessage
      switch (msg.type) {
        case 'hello.ack':
          set({ link: 'online', sessionId: msg.session_id })
          break
        case 'light':
          set({ light: msg.state })
          break
        case 'error':
          set({ lastError: `${msg.code}: ${msg.message}` })
          break
      }
    }
    ws.onclose = () => {
      socket = null
      set({ link: 'offline', sessionId: null, light: 'idle' })
    }
  },

  disconnect: () => socket?.close(),

  wake: () => {
    if (get().link !== 'online') return
    socket?.send(JSON.stringify({ type: 'wake', source: 'touch' }))
  },
}))
