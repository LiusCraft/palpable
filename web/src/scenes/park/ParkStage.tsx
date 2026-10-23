import { copy } from '../../ui/copy'

/** 公园场景占位：地图与多设备在 W5 实现。 */
export function ParkStage() {
  return <div className="flex h-64 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-700">{copy.park.title}</div>
}
