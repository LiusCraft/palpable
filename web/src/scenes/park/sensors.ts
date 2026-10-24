// 本页玩偶在公园里的位置，以及据此模拟的 GPS 与 BLE 扫描。
// 其他玩偶（含别的浏览器窗口）的位置来自 /world 环境输入，不读取它们的设备状态。
import { useEffect } from 'react'
import { create } from 'zustand'
import { distanceMeters, strengthAt } from '../../device-sim/environment'
import { useClient } from '../../device-sim/client'
import { useWorld, type LatLng } from '../../map/world'

interface Positions {
  pos: Record<string, LatLng>
  move: (mac: string, p: LatLng) => void
}

export const usePositions = create<Positions>((set) => ({
  pos: {},
  move: (mac, p) => set((s) => ({ pos: { ...s.pos, [mac]: p } })),
}))

const SCAN_INTERVAL = 1000

/** 公园场景挂载期间：每秒上报本页玩偶的定位与靠近扫描。位置由第一人称走动写入 usePositions。 */
export function useParkSensors(): void {
  useEffect(() => {
    const tick = () => {
      const d = useClient.getState().device
      const me = d && usePositions.getState().pos[d.mac]
      if (!d || !me || d.store.getState().link !== 'online') return
      d.reportLocation(me.lat, me.lng, true)
      const peers: { deviceId: string; strength: number }[] = []
      for (const other of Object.values(useWorld.getState().devices)) {
        if (other.device === d.mac || !other.online || !other.pos) continue
        const s = strengthAt(distanceMeters(me, other.pos))
        if (s > 0) peers.push({ deviceId: other.device, strength: s })
      }
      peers.sort((a, b) => b.strength - a.strength)
      d.reportProximity(peers)
    }
    const t = window.setInterval(tick, SCAN_INTERVAL)
    return () => window.clearInterval(t)
  }, [])
}
