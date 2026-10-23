// 演示地图的坐标换算。地图覆盖公园及周边约 860 × 670 米，SVG 里大约 1 像素 1 米。
// 公园轮廓以服务端 /world 下发的为准，这里的常量只给初始位置与装饰用。
import type { LatLng } from '../../map/world'

export const MAP_W = 860
export const MAP_H = 660

const BOUNDS = { north: 31.23, south: 31.224, west: 121.4685, east: 121.4775 }

export function toXY(p: LatLng): { x: number; y: number } {
  return {
    x: ((p.lng - BOUNDS.west) / (BOUNDS.east - BOUNDS.west)) * MAP_W,
    y: ((BOUNDS.north - p.lat) / (BOUNDS.north - BOUNDS.south)) * MAP_H,
  }
}

export function toLatLng(x: number, y: number): LatLng {
  return {
    lat: BOUNDS.north - (y / MAP_H) * (BOUNDS.north - BOUNDS.south),
    lng: BOUNDS.west + (x / MAP_W) * (BOUNDS.east - BOUNDS.west),
  }
}

/** 点是否在多边形内（射线法）。只用于本地展示提示，是否"进入区域"由服务端判定。 */
export function inside(p: LatLng, poly: LatLng[]): boolean {
  let hit = false
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const a = poly[i]
    const b = poly[j]
    if (a.lat > p.lat !== b.lat > p.lat && p.lng < ((b.lng - a.lng) * (p.lat - a.lat)) / (b.lat - a.lat) + a.lng) hit = !hit
  }
  return hit
}

/** 新玩偶出现在公园西门外的小路上，走进去才会触发"进入区域"。 */
export function startPoint(i: number): LatLng {
  return toLatLng(44 + Math.floor(i / 4) * 60, 200 + (i % 4) * 70)
}
