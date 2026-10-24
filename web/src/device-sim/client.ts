// 这个页面就是一台设备的客户端：一个浏览器页面 = 一只玩偶 = 一个小朋友。
// 想要第二个小朋友，就再开一个页面（或另一个浏览器）；玩偶之间只通过服务端互通。
import { create } from 'zustand'
import type { MAC } from '../shared/protocol.gen'
import { MicCapture } from './audio'
import { VirtualDevice } from './device'
import { deviceSkin, randomMAC } from './identity'

const STORAGE_KEY = 'palpable.devices'
const MAC_RE = /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/
const MAX_KNOWN = 8

interface Saved {
  mac: MAC
  skin: string
}

function load(): Saved[] {
  try {
    const list = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]') as Saved[]
    if (Array.isArray(list)) return list.filter((s) => MAC_RE.test(s.mac))
  } catch {
    // 浏览器存储不可用时每次打开都是一只新玩偶
  }
  return []
}

function save(list: Saved[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(list.slice(-MAX_KNOWN)))
  } catch {
    // 忽略
  }
}

/** 页面存活期间占住这只玩偶；别的页面抢不到，就会拿到另一只。 */
function lock(mac: MAC): Promise<boolean> {
  if (!navigator.locks) return Promise.resolve(true)
  return new Promise((resolve) => {
    void navigator.locks.request(`palpable:${mac}`, { ifAvailable: true }, (l) => {
      resolve(l !== null)
      return l ? new Promise<void>(() => {}) : undefined
    })
  })
}

/**
 * 认领这个页面的玩偶：优先是这个浏览器上次用的、且没被别的页面占着的那只，
 * 否则新开一只（相当于拆一个新盒子）。?device=<mac> 可指定。
 */
async function claim(): Promise<VirtualDevice> {
  const fromURL = new URLSearchParams(location.search).get('device')?.toLowerCase()
  const known = load()
  if (fromURL && MAC_RE.test(fromURL)) {
    const skin = known.find((s) => s.mac === fromURL)?.skin ?? deviceSkin()
    return new VirtualDevice(fromURL, skin)
  }
  for (const s of known.reverse()) {
    if (await lock(s.mac)) return new VirtualDevice(s.mac, new URLSearchParams(location.search).get('skin') ?? s.skin)
  }
  for (;;) {
    const mac = randomMAC()
    if (await lock(mac)) {
      const skin = deviceSkin()
      save([...load(), { mac, skin }])
      return new VirtualDevice(mac, skin)
    }
  }
}

interface ClientState {
  device: VirtualDevice | null
  start: () => Promise<void>
  changeSkin: (skin: string) => void
}

export const useClient = create<ClientState>((set, get) => ({
  device: null,
  start: async () => {
    if (get().device) return
    set({ device: await claim() })
  },
  changeSkin: (skin) => {
    const d = get().device
    if (!d) return
    d.setSkin(skin)
    save(load().map((s) => (s.mac === d.mac ? { ...s, skin } : s)))
  },
}))

/** 拿到本页的玩偶；只在 start() 完成后的界面里使用。 */
export function useDevice(): VirtualDevice {
  const d = useClient((s) => s.device)
  if (!d) throw new Error('device not claimed yet')
  return d
}

const mic = new MicCapture()

/** 打开物理麦克风，把每一帧交给本页的设备。 */
export async function startMic(): Promise<void> {
  await mic.start((pcm, rms) => useClient.getState().device?.onPCM(pcm, rms))
}

/**
 * 玩偶放上桌面、通电：上线，并默认打开麦克风，开口就能对话（设备端 VAD 判断哪些是话，不需要按键）。
 * 必须在用户手势里调用：浏览器要求手势才允许播放声音与打开麦克风。
 */
export async function activate(device: VirtualDevice): Promise<void> {
  device.connect()
  try {
    await startMic()
    device.setMicEnabled(true)
  } catch (e) {
    device.store.setState({ lastError: `无法打开麦克风：${e instanceof Error ? e.message : String(e)}` })
  }
}

/** 记下这只玩偶已经拆过箱，下次打开直接在桌面上。 */
const UNBOXED = 'palpable.unboxed'
export function markUnboxed(mac: MAC): void {
  try {
    const list = new Set(JSON.parse(localStorage.getItem(UNBOXED) ?? '[]') as string[])
    list.add(mac)
    localStorage.setItem(UNBOXED, JSON.stringify([...list]))
  } catch {
    // 忽略
  }
}

export function wasUnboxed(mac: MAC): boolean {
  try {
    return (JSON.parse(localStorage.getItem(UNBOXED) ?? '[]') as string[]).includes(mac)
  } catch {
    return false
  }
}
