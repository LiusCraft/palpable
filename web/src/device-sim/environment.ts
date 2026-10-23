// 虚拟设备所处的"物理环境"：Web 演示里由当前场景提供。
// 摄像头看到什么、身边有哪些设备的信号，都是环境给传感器的输入，不是读取其他设备的内部状态。

type Describe = (mac: string) => string

let describe: Describe = () => '眼前是一张小桌子，上面有一盏暖暖的台灯。'

/** 场景切换时登记"摄像头会看到什么"。 */
export function setSceneDescriber(fn: Describe): void {
  describe = fn
}

export function lookAround(mac: string): string {
  return describe(mac)
}

/**
 * 模拟 BLE 信号强度：距离越近越强。演示地图约 1 像素 1 米，头像直径约 48 像素，
 * 两只玩偶在地图上碰到一起（≤ 50 米）时强度达到 0.8，满足服务端"贴近"的判定。
 */
export function strengthAt(meters: number): number {
  return Math.max(0, Math.min(1, 1 - meters / 250))
}

const EARTH = 6371000

export function distanceMeters(a: { lat: number; lng: number }, b: { lat: number; lng: number }): number {
  const toRad = (d: number) => (d * Math.PI) / 180
  const x = toRad(b.lng - a.lng) * Math.cos(toRad((a.lat + b.lat) / 2))
  const y = toRad(b.lat - a.lat)
  return Math.hypot(x, y) * EARTH
}
