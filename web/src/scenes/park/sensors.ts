// 本页玩偶在公园里的位置，以及据此模拟的 GPS 与 BLE 扫描。
// 其他玩偶（含别的浏览器窗口）的位置来自 /world 环境输入，不读取它们的设备状态。
import { useEffect } from 'react'
import { create } from 'zustand'
import { distanceMeters, strengthAt } from '../../device-sim/environment'
import { useDevices } from '../../device-sim/devices'
import { useWorld, type LatLng } from '../../map/world'
import { startPoint } from './geo'

interface Positions {
  pos: Record<string, LatLng>
  move: (mac: string, p: LatLng) => void
}

export const usePositions = create<Positions>((set) => ({
  pos: {},
  move: (mac, p) => set((s) => ({ pos: { ...s.pos, [mac]: p } })),
}))

const SCAN_INTERVAL = 1000

/** 公园场景挂载期间：给每只本地玩偶一个位置，每秒上报定位与靠近扫描。 */
export function useParkSensors(): void {
  const devices = useDevices((s) => s.devices)
  useEffect(() => {
    const { pos, move } = usePositions.getState()
    devices.forEach((d, i) => {
      if (!pos[d.mac]) move(d.mac, startPoint(i))
    })
  }, [devices])

  useEffect(() => {
    const tick = () => {
      const { pos } = usePositions.getState()
      const world = useWorld.getState().devices
      for (const d of useDevices.getState().devices) {
        const me = pos[d.mac]
        if (!me || d.store.getState().link !== 'online') continue
        d.reportLocation(me.lat, me.lng, true)
        const peers: { deviceId: string; strength: number }[] = []
        for (const other of Object.values(world)) {
          if (other.device === d.mac || !other.online) continue
          const there = pos[other.device] ?? other.pos
          if (!there) continue
          const s = strengthAt(distanceMeters(me, there))
          if (s > 0) peers.push({ deviceId: other.device, strength: s })
        }
        peers.sort((a, b) => b.strength - a.strength)
        d.reportProximity(peers)
      }
    }
    const t = window.setInterval(tick, SCAN_INTERVAL)
    return () => window.clearInterval(t)
  }, [])
}
