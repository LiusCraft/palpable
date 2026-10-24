import { useFrame, useThree } from '@react-three/fiber'
import { useEffect, useMemo, useRef } from 'react'
import { Euler, type Group } from 'three'
import type { VirtualDevice } from '../../device-sim/device'
import { DevicePlush } from '../../plush/DevicePlush'
import { useJourney } from '../journey'
import { EYE } from './touch'

// 第一人称：房间与公园共用的"走路 + 转头"控制器，和挂在胸前的玩偶。
// 这里只管相机，不含业务；位置去向由调用方通过回调决定。

const SPEED = 4
const TURN = 1.8
type Blocker = [x: number, z: number, r: number]

export interface RigProps {
  start: [number, number]
  /** 0 朝 -Z；正值向左转。 */
  yaw: number
  halfX: number
  halfZ: number
  /** 圆形障碍（别的玩偶、家具）：走近会被推开。 */
  blockers?: () => Blocker[]
  /** 每帧位置；调用方自己节流。 */
  onMove?: (x: number, z: number) => void
  /** 外部（如小地图）把人挪走时，返回新的位置让相机跟过去。 */
  follow?: () => [number, number] | null
  device: VirtualDevice
}

export function FirstPersonRig({ start, yaw, halfX, halfZ, blockers, onMove, follow, device }: RigProps) {
  const { camera, gl } = useThree()
  const st = useRef({ x: start[0], z: start[1], yaw, pitch: -0.05, keys: new Set<string>() })
  const euler = useMemo(() => new Euler(0, 0, 0, 'YXZ'), [])

  useEffect(() => {
    const s = st.current
    const typing = (t: EventTarget | null) => t instanceof HTMLElement && /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)
    const down = (e: KeyboardEvent) => {
      if (typing(e.target) || !useJourney.getState().entered) return
      if (e.code === 'KeyF' && !e.repeat) device.wake() // 摸一摸玩偶：触摸唤醒
      s.keys.add(e.code)
      if (e.code.startsWith('Arrow')) e.preventDefault()
    }
    const up = (e: KeyboardEvent) => s.keys.delete(e.code)
    const blur = () => s.keys.clear()
    window.addEventListener('keydown', down)
    window.addEventListener('keyup', up)
    window.addEventListener('blur', blur)

    // 像 FPS 一样：点一下画面锁住鼠标，之后直接移动鼠标转头；Esc 放开鼠标去点界面。
    const el = gl.domElement
    const lock = () => {
      if (!useJourney.getState().entered) return
      if (document.pointerLockElement !== el) void Promise.resolve(el.requestPointerLock?.()).catch(() => {})
    }
    const look = (e: MouseEvent) => {
      if (document.pointerLockElement !== el) return
      s.yaw -= e.movementX * 0.0025
      s.pitch = Math.max(-1.3, Math.min(1.2, s.pitch - e.movementY * 0.0025))
    }
    const changed = () => useJourney.getState().setLocked(document.pointerLockElement === el)
    el.addEventListener('click', lock)
    document.addEventListener('mousemove', look)
    document.addEventListener('pointerlockchange', changed)
    changed()
    lock() // 进门之后换场景（出门、回家）时靠刚才那下按键的激活期接着锁住；没有激活期就静默失败，等孩子点一下
    return () => {
      window.removeEventListener('keydown', down)
      window.removeEventListener('keyup', up)
      window.removeEventListener('blur', blur)
      el.removeEventListener('click', lock)
      document.removeEventListener('mousemove', look)
      document.removeEventListener('pointerlockchange', changed)
    }
  }, [gl, device])

  useFrame((_, dt) => {
    const s = st.current
    const k = s.keys
    const step = Math.min(dt, 0.1)
    if (k.has('ArrowLeft')) s.yaw += TURN * step
    if (k.has('ArrowRight')) s.yaw -= TURN * step
    const fwd = (k.has('KeyW') || k.has('ArrowUp') ? 1 : 0) - (k.has('KeyS') || k.has('ArrowDown') ? 1 : 0)
    const side = (k.has('KeyD') ? 1 : 0) - (k.has('KeyA') ? 1 : 0)
    if (fwd || side) {
      const len = Math.hypot(fwd, side)
      const sin = Math.sin(s.yaw)
      const cos = Math.cos(s.yaw)
      s.x += ((-sin * fwd + cos * side) / len) * SPEED * step
      s.z += ((-cos * fwd - sin * side) / len) * SPEED * step
    }
    s.x = Math.max(-halfX, Math.min(halfX, s.x))
    s.z = Math.max(-halfZ, Math.min(halfZ, s.z))
    for (const [bx, bz, r] of blockers?.() ?? []) {
      const dx = s.x - bx
      const dz = s.z - bz
      const d = Math.hypot(dx, dz)
      if (d < r && d > 0) {
        s.x = bx + (dx / d) * r
        s.z = bz + (dz / d) * r
      }
    }
    const moved = follow?.()
    if (moved) [s.x, s.z] = moved
    onMove?.(s.x, s.z)

    // 振动：画面轻轻晃一下；走路有一点起伏。
    const since = (Date.now() - device.store.getState().shakeAt) / 1000
    const shake = since < 0.5 ? Math.sin(since * 60) * 0.015 * (1 - since / 0.5) : 0
    const bob = fwd || side ? Math.sin(performance.now() / 130) * 0.025 : 0
    camera.position.set(s.x, EYE + bob + shake, s.z)
    camera.rotation.copy(euler.set(s.pitch, s.yaw, 0))
  })
  return null
}

/** 玩偶抱在胸前：灯环随设备状态变色；点一下是触摸唤醒。 */
export function HeldPlush({ device }: { device: VirtualDevice }) {
  const g = useRef<Group>(null)
  useFrame(({ camera }) => {
    if (!g.current) return
    g.current.position.copy(camera.position)
    g.current.quaternion.copy(camera.quaternion)
  })
  return (
    <group ref={g}>
      <DevicePlush device={device} position={[-0.5, -0.42, -0.9]} scale={0.15} />
    </group>
  )
}
