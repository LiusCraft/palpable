import type { MAC } from '../shared/protocol.gen'

// 本地管理的单播地址（首字节第二位为 1、最低位为 0），不会与真实厂商 MAC 冲突。
// 真实设备的 MAC 出厂自带；模拟器每开一个新盒子生成一个，由 client.ts 记在浏览器里。
export function randomMAC(): MAC {
  const b = crypto.getRandomValues(new Uint8Array(6))
  b[0] = (b[0] | 0x02) & 0xfe
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join(':')
}

// 皮套款式；?skin= 可切换，默认小熊（毛绒皮套）。
export function deviceSkin(): string {
  return new URLSearchParams(location.search).get('skin') ?? 'bear'
}
