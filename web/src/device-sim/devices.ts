// 本页面上的虚拟设备（"小朋友窗口"）与唯一的物理麦克风。
// 一个页面可以开多只玩偶；麦克风同一时刻只交给一只，相当于那个小朋友在说话。
import { create } from 'zustand'
import type { MAC } from '../shared/protocol.gen'
import { MicCapture } from './audio'
import { VirtualDevice } from './device'
import { deviceMAC, deviceSkin, randomMAC } from './identity'

const STORAGE_KEY = 'palpable.devices'

interface Saved {
  mac: MAC
  skin: string
}

function load(): Saved[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    const list = raw ? (JSON.parse(raw) as Saved[]) : []
    if (Array.isArray(list) && list.length > 0) return list
  } catch {
    // 浏览器存储不可用时只开默认的一只
  }
  return [{ mac: deviceMAC(), skin: deviceSkin() }]
}

function save(devices: VirtualDevice[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(devices.map((d) => ({ mac: d.mac, skin: d.store.getState().skin }))))
  } catch {
    // 忽略
  }
}

interface DevicesState {
  devices: VirtualDevice[]
  active: MAC | null // 麦克风交给谁
  add: (skin: string) => VirtualDevice
  remove: (mac: MAC) => void
  setActive: (mac: MAC) => void
}

// ?device=<mac> 指定时只开这一只，便于多个浏览器窗口各扮一个小朋友。
const fromURL = new URLSearchParams(location.search).has('device')
const initial = (fromURL ? [{ mac: deviceMAC(), skin: deviceSkin() }] : load()).map((s) => new VirtualDevice(s.mac, s.skin))

export const useDevices = create<DevicesState>((set, get) => ({
  devices: initial,
  active: initial[0]?.mac ?? null,
  add: (skin) => {
    const d = new VirtualDevice(randomMAC(), skin)
    const devices = [...get().devices, d]
    set({ devices })
    if (!fromURL) save(devices)
    return d
  },
  remove: (mac) => {
    const d = get().devices.find((x) => x.mac === mac)
    d?.dispose()
    const devices = get().devices.filter((x) => x.mac !== mac)
    set({ devices, active: get().active === mac ? (devices[0]?.mac ?? null) : get().active })
    if (!fromURL) save(devices)
  },
  setActive: (mac) => set({ active: mac }),
}))

const mic = new MicCapture()

/** 打开物理麦克风，把每一帧交给当前持有麦克风的设备。 */
export async function startMic(): Promise<void> {
  await mic.start((pcm, rms) => {
    const { devices, active } = useDevices.getState()
    devices.find((d) => d.mac === active)?.onPCM(pcm, rms)
  })
}

export function stopMic(): void {
  mic.stop()
}

export function micRunning(): boolean {
  return mic.running
}
