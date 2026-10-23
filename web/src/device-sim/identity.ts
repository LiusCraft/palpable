import type { MAC } from '../shared/protocol.gen'

const MAC_RE = /^[0-9a-f]{2}(:[0-9a-f]{2}){5}$/
const STORAGE_KEY = 'palpable.mac'

// 本地管理的单播地址（首字节第二位为 1、最低位为 0），不会与真实厂商 MAC 冲突。
function randomMAC(): MAC {
  const b = crypto.getRandomValues(new Uint8Array(6))
  b[0] = (b[0] | 0x02) & 0xfe
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join(':')
}

// 虚拟设备的 MAC：真实设备出厂自带，模拟器首次打开时生成并存在浏览器里。
// ?device=<mac> 可覆盖，便于同一浏览器开多个"小朋友窗口"。
export function deviceMAC(): MAC {
  const fromURL = new URLSearchParams(location.search).get('device')?.toLowerCase()
  if (fromURL && MAC_RE.test(fromURL)) return fromURL
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && MAC_RE.test(saved)) return saved
    const mac = randomMAC()
    localStorage.setItem(STORAGE_KEY, mac)
    return mac
  } catch {
    return randomMAC()
  }
}

// 皮套款式；?skin= 可切换，默认小龙。
export function deviceSkin(): string {
  return new URLSearchParams(location.search).get('skin') ?? 'dragon'
}
