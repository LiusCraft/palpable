// 订阅 /world 演示通道：所有设备的位置、名字、频道、谁在说话、好友关系、区域轮廓。
// 只给 Web 展示用（地图、名字标签、模拟 BLE 的环境输入），不属于设备协议。
import { create } from 'zustand'

export interface LatLng {
  lat: number
  lng: number
}

export interface WorldDevice {
  device: string
  name: string
  skin: string
  online: boolean
  pos?: LatLng
  channels: string[]
  speaking: boolean
}

export interface WorldZone {
  id: string
  name: string
  channel: string
  polygon: LatLng[]
}

interface Friendship {
  a: string
  b: string
  status: 'pending' | 'confirmed'
}

interface WorldEvent {
  kind: 'snapshot' | 'device' | 'pos' | 'member' | 'burst' | 'friend'
  device?: string
  name?: string
  skin?: string
  online?: boolean
  pos?: LatLng
  channel?: string
  joined?: boolean
  speaking?: boolean
  friend?: string
  status?: 'pending' | 'confirmed' | 'removed'
  devices?: WorldDevice[]
  friends?: Friendship[]
  zones?: WorldZone[]
}

interface WorldState {
  connected: boolean
  devices: Record<string, WorldDevice>
  friends: Record<string, Friendship['status']> // key: 排序后的 "a|b"
  zones: WorldZone[]
}

export const pairKey = (a: string, b: string) => (a < b ? `${a}|${b}` : `${b}|${a}`)

export const useWorld = create<WorldState>(() => ({ connected: false, devices: {}, friends: {}, zones: [] }))

function blank(device: string): WorldDevice {
  return { device, name: '', skin: '', online: false, channels: [], speaking: false }
}

function apply(e: WorldEvent): void {
  useWorld.setState((s) => {
    if (e.kind === 'snapshot') {
      const devices: Record<string, WorldDevice> = {}
      for (const d of e.devices ?? []) devices[d.device] = d
      const friends: WorldState['friends'] = {}
      for (const f of e.friends ?? []) friends[pairKey(f.a, f.b)] = f.status
      return { devices, friends, zones: e.zones ?? [] }
    }
    if (e.kind === 'friend' && e.device && e.friend) {
      const friends = { ...s.friends }
      const k = pairKey(e.device, e.friend)
      if (e.status === 'removed' || !e.status) delete friends[k]
      else friends[k] = e.status
      return { friends }
    }
    if (!e.device) return {}
    const d = { ...(s.devices[e.device] ?? blank(e.device)) }
    switch (e.kind) {
      case 'device':
        if (e.online !== undefined) {
          d.online = e.online
          if (!e.online) {
            d.channels = []
            d.speaking = false
          }
        }
        if (e.name) d.name = e.name
        if (e.skin) d.skin = e.skin
        break
      case 'pos':
        d.pos = e.pos
        break
      case 'member':
        d.channels = d.channels.filter((c) => c !== e.channel)
        if (e.joined && e.channel) d.channels = [...d.channels, e.channel]
        break
      case 'burst':
        d.speaking = !!e.speaking
        break
    }
    return { devices: { ...s.devices, [e.device]: d } }
  })
}

let socket: WebSocket | null = null

/** 连接演示通道；断开后自动重连。服务端未开启演示模式时安静失败。 */
export function connectWorld(): void {
  if (socket) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const ws = new WebSocket(`${proto}://${location.host}/world`)
  socket = ws
  ws.onopen = () => useWorld.setState({ connected: true })
  ws.onmessage = (ev) => apply(JSON.parse(ev.data as string) as WorldEvent)
  ws.onclose = () => {
    socket = null
    useWorld.setState({ connected: false })
    window.setTimeout(connectWorld, 3000)
  }
}

export function friendStatus(a: string, b: string): Friendship['status'] | undefined {
  return useWorld.getState().friends[pairKey(a, b)]
}
