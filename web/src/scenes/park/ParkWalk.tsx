import { Html } from '@react-three/drei'
import { Canvas, useFrame } from '@react-three/fiber'
import { useEffect, useMemo, useRef } from 'react'
import { Shape, type Group, type Mesh, type MeshBasicMaterial } from 'three'
import type { VirtualDevice } from '../../device-sim/device'
import { pairKey, useWorld, type LatLng } from '../../map/world'
import { Plush } from '../../plush/Plush'
import { copy } from '../../ui/copy'
import { useJourney } from '../journey'
import { FirstPersonRig, HeldPlush } from '../world/FirstPerson'
import { Interactor, type Spot } from '../world/Interactor'
import { MAP_H, MAP_W, startPoint, toLatLng, toXY } from './geo'
import { usePositions } from './sensors'

// 公园第一人称：孩子带着玩偶在 3D 公园里走，走到小伙伴身边"贴一贴"。
// 位置只写进 usePositions，定位与靠近扫描仍由 useParkSensors 按原路径上报；
// 这里不做任何业务判断，其他玩偶只来自 /world 的环境输入。

/** 地图 1 像素约 1 米；3D 里缩小 20 倍，使"贴近"（≤ 50 米）约等于一臂远，整个公园十几秒走完。 */
const S = 20
const PLUSH = 0.6
const BODY = 0.9
/** 与 environment.strengthAt 的"贴近"门槛（强度 0.8 ⇔ 50 米）对齐，仅用于脚下光圈提示。 */
const TOUCH_RANGE = 50 / S
const SYNC_MS = 100

const toWorld = (x: number, y: number): [number, number] => [(x - MAP_W / 2) / S, (y - MAP_H / 2) / S]
/** 家在公园西门外：从房间出来站在门前，回家就是走回这扇门。 */
const HOME = toWorld(36, 200)
const HOME_SPOT: [number, number] = [HOME[0] + 1.4, HOME[1]]
const llToWorld = (p: LatLng): [number, number] => {
  const { x, y } = toXY(p)
  return toWorld(x, y)
}
const worldToLatLng = (x: number, z: number): LatLng => toLatLng(x * S + MAP_W / 2, z * S + MAP_H / 2)

const TREES: [number, number, number][] = [
  [210, 150, 16], [250, 130, 20], [300, 160, 14], [560, 140, 18], [610, 170, 15], [660, 140, 20],
  [700, 230, 16], [230, 420, 18], [270, 460, 15], [640, 420, 17], [690, 460, 21], [600, 500, 14],
  [420, 120, 13], [480, 500, 16], [190, 300, 14], [720, 330, 15],
  [60, 120, 12], [90, 560, 14], [800, 100, 13], [790, 560, 15],
]

const HOUSES: [number, number, string][] = [
  [40, 70, '#f6c48f'], [100, 50, '#9ec5f8'], [780, 40, '#f4a6b8'], [40, 600, '#b9e3a5'], [800, 610, '#f6c48f'],
]

type Pt = [number, number]
const bezier = (p0: Pt, p1: Pt, p2: Pt, p3: Pt, t: number): Pt => {
  const u = 1 - t
  return [
    u * u * u * p0[0] + 3 * u * u * t * p1[0] + 3 * u * t * t * p2[0] + t * t * t * p3[0],
    u * u * u * p0[1] + 3 * u * u * t * p1[1] + 3 * u * t * t * p2[1] + t * t * t * p3[1],
  ]
}

/** 与 ParkMap 里小河的两段三次曲线一致。 */
const RIVER: Pt[] = (() => {
  const out: Pt[] = []
  for (let i = 0; i <= 20; i++) out.push(bezier([150, 560], [260, 470], [330, 520], [420, 420], i / 20))
  for (let i = 1; i <= 20; i++) out.push(bezier([420, 420], [510, 320], [560, 260], [720, 180], i / 20))
  return out
})()

function Home() {
  return (
    <group position={[HOME[0], 0, HOME[1]]}>
      <mesh position={[0, 1.1, 0]}>
        <boxGeometry args={[3, 2.2, 2.6]} />
        <meshStandardMaterial color="#fbe9d6" roughness={1} />
      </mesh>
      <mesh position={[0, 2.6, 0]} rotation={[0, Math.PI / 4, 0]}>
        <coneGeometry args={[2.5, 1.1, 4]} />
        <meshStandardMaterial color="#f4a6b8" roughness={1} />
      </mesh>
      {/* 门朝东，对着公园 */}
      <mesh position={[1.52, 0.9, 0]}>
        <boxGeometry args={[0.06, 1.8, 1.0]} />
        <meshStandardMaterial color="#8fd0e8" roughness={0.6} />
      </mesh>
      <mesh position={[1.57, 0.9, 0.3]}>
        <sphereGeometry args={[0.06, 12, 12]} />
        <meshStandardMaterial color="#ffd24d" metalness={0.6} roughness={0.3} />
      </mesh>
      <Html position={[1.6, 2.3, 0]} center zIndexRange={[10, 0]} style={{ pointerEvents: 'none' }}>
        <div className="whitespace-nowrap rounded-full bg-white/90 px-2 py-0.5 text-xs font-semibold text-[#1d3557] shadow">🏠 我的家</div>
      </Html>
    </group>
  )
}

function Ground() {
  const zone = useWorld((w) => w.zones[0])
  const shape = useMemo(() => {
    if (!zone) return null
    const s = new Shape()
    zone.polygon.forEach((p, i) => {
      const [x, z] = llToWorld(p)
      if (i) s.lineTo(x, -z)
      else s.moveTo(x, -z)
    })
    return s
  }, [zone])

  return (
    <group>
      <mesh rotation={[-Math.PI / 2, 0, 0]}>
        <planeGeometry args={[MAP_W / S + 40, MAP_H / S + 40]} />
        <meshStandardMaterial color="#b4d99a" roughness={1} />
      </mesh>
      {shape && (
        <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, 0.01, 0]}>
          <shapeGeometry args={[shape]} />
          <meshStandardMaterial color="#7fd06a" roughness={1} side={2} />
        </mesh>
      )}
      {/* 外围道路 */}
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, 0.02, toWorld(0, 330)[1]]}>
        <planeGeometry args={[MAP_W / S, 1.3]} />
        <meshStandardMaterial color="#f1e6d0" roughness={1} />
      </mesh>
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[toWorld(430, 0)[0], 0.02, 0]}>
        <planeGeometry args={[0.9, MAP_H / S]} />
        <meshStandardMaterial color="#f1e6d0" roughness={1} />
      </mesh>
      {/* 小河 */}
      {RIVER.slice(1).map((p, i) => {
        const [x0, z0] = toWorld(...RIVER[i])
        const [x1, z1] = toWorld(...p)
        const len = Math.hypot(x1 - x0, z1 - z0)
        return (
          <mesh key={i} position={[(x0 + x1) / 2, 0.03, (z0 + z1) / 2]} rotation={[-Math.PI / 2, 0, -Math.atan2(z1 - z0, x1 - x0)]}>
            <planeGeometry args={[len + 0.1, 1.3]} />
            <meshStandardMaterial color="#8fd3f4" roughness={0.3} />
          </mesh>
        )
      })}
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[toWorld(360, 230)[0], 0.03, toWorld(360, 230)[1]]}>
        <circleGeometry args={[2.1, 32]} />
        <meshStandardMaterial color="#9fdcf6" roughness={0.3} />
      </mesh>
    </group>
  )
}

function Scenery() {
  return (
    <group>
      {TREES.map(([x, y, r], i) => {
        const [wx, wz] = toWorld(x, y)
        const h = 1.4 + r / 8
        return (
          <group key={i} position={[wx, 0, wz]}>
            <mesh position={[0, h / 2, 0]}>
              <cylinderGeometry args={[0.1, 0.14, h, 8]} />
              <meshStandardMaterial color="#9b7653" roughness={1} />
            </mesh>
            <mesh position={[0, h + 0.3, 0]}>
              <sphereGeometry args={[0.55 + r / 25, 16, 16]} />
              <meshStandardMaterial color="#5fb85a" roughness={1} />
            </mesh>
            <mesh position={[0.25, h + 0.65, 0.1]}>
              <sphereGeometry args={[0.35 + r / 40, 16, 16]} />
              <meshStandardMaterial color="#7ccf6b" roughness={1} />
            </mesh>
          </group>
        )
      })}
      {HOUSES.map(([x, y, c], i) => {
        const [wx, wz] = toWorld(x, y)
        return (
          <group key={i} position={[wx, 0, wz]}>
            <mesh position={[0, 0.7, 0]}>
              <boxGeometry args={[1.8, 1.4, 1.4]} />
              <meshStandardMaterial color="#fffaf2" roughness={1} />
            </mesh>
            <mesh position={[0, 1.75, 0]} rotation={[0, Math.PI / 4, 0]}>
              <coneGeometry args={[1.5, 0.8, 4]} />
              <meshStandardMaterial color={c} roughness={1} />
            </mesh>
          </group>
        )
      })}
    </group>
  )
}

function Peer({ viewer, id }: { viewer: string; id: string }) {
  const d = useWorld((w) => w.devices[id])
  const rel = useWorld((w) => w.friends[pairKey(viewer, id)])
  const root = useRef<Group>(null)
  const ring = useRef<Mesh>(null)
  const burst = useRef<{ at: number; group: Group | null }>({ at: -10, group: null })
  const prev = useRef(rel)
  const placed = useRef(false)
  const target = d?.pos
  const goal = useMemo<[number, number] | null>(() => (target ? llToWorld(target) : null), [target])

  // 刚成为好友（或刚有待确认请求）时，在对方身上放一次庆祝。
  useEffect(() => {
    if (rel && !prev.current) burst.current.at = performance.now() / 1000
    prev.current = rel
  }, [rel])

  useFrame(({ camera, clock }, dt) => {
    const g = root.current
    if (!g || !goal) return
    // 窗口里别的玩偶是 /world 一两秒推一次，平滑过去；第一次直接就位。
    const k = placed.current ? 1 - Math.exp(-dt * 8) : 1
    placed.current = true
    g.position.x += (goal[0] - g.position.x) * k
    g.position.z += (goal[1] - g.position.z) * k
    g.rotation.y = Math.atan2(camera.position.x - g.position.x, camera.position.z - g.position.z)

    const near = Math.hypot(camera.position.x - g.position.x, camera.position.z - g.position.z) <= TOUCH_RANGE
    if (ring.current) {
      ring.current.visible = near
      const m = ring.current.material as MeshBasicMaterial
      m.opacity = 0.35 + Math.sin(clock.getElapsedTime() * 5) * 0.15
    }
    const t = performance.now() / 1000 - burst.current.at
    const b = burst.current.group
    if (b) {
      b.visible = t >= 0 && t < 2
      if (b.visible) {
        b.children.forEach((c, i) => {
          const a = (i / b.children.length) * Math.PI * 2
          const r = t * 1.6
          c.position.set(Math.cos(a) * r, 0.4 + t * 1.2 - t * t * 0.3, Math.sin(a) * r)
          c.scale.setScalar(Math.max(0.01, 1 - t / 2))
        })
      }
    }
  })

  if (!d || !goal || !d.online) return null
  const tone = rel === 'confirmed' ? '#3bb273' : rel === 'pending' ? '#f5a524' : '#f0566b'
  const label = rel === 'confirmed' ? '好友' : rel === 'pending' ? '待确认' : copy.park.blocked
  return (
    <group ref={root}>
      <mesh ref={ring} rotation={[-Math.PI / 2, 0, 0]} position={[0, 0.05, 0]} visible={false}>
        <ringGeometry args={[1.0, 1.25, 40]} />
        <meshBasicMaterial color={tone} transparent opacity={0.4} />
      </mesh>
      <Plush skin={d.skin || 'bear'} light={d.speaking ? 'peer_talking' : 'idle'} speaking={d.speaking} position={[0, 1.22 * PLUSH, 0]} scale={PLUSH} />
      <group ref={(g) => { burst.current.group = g }} visible={false}>
        {Array.from({ length: 10 }, (_, i) => (
          <mesh key={i}>
            <sphereGeometry args={[0.09, 8, 8]} />
            <meshBasicMaterial color={i % 2 ? '#ff86b6' : '#ffd24d'} />
          </mesh>
        ))}
      </group>
      <Html position={[0, 1.6, 0]} center zIndexRange={[10, 0]} style={{ pointerEvents: 'none' }}>
        <div className="flex items-center gap-1 whitespace-nowrap rounded-full bg-white/90 px-2 py-0.5 text-xs font-semibold text-[#1d3557] shadow">
          {d.name || '玩偶'}
          <span className="rounded-full px-1.5 text-[10px] text-white" style={{ background: tone }}>
            {label}
          </span>
        </div>
      </Html>
    </group>
  )
}

function Peers({ viewer }: { viewer: string }) {
  const ids = useWorld((w) => Object.keys(w.devices).filter((k) => k !== viewer).join(','))
  return (
    <>
      {ids
        .split(',')
        .filter(Boolean)
        .map((id) => (
          <Peer key={id} viewer={viewer} id={id} />
        ))}
    </>
  )
}

function Scene({ device }: { device: VirtualDevice }) {
  const mac = device.mac
  const sent = useRef({ at: 0, last: null as LatLng | null })
  const start = useMemo<[number, number]>(() => {
    const [x, z] = llToWorld(startPoint(0))
    return [x + 3, z]
  }, [])
  const pos = useRef<[number, number]>(start)
  const travel = useJourney((j) => j.travel)
  const spots: Spot[] = [{ x: HOME_SPOT[0], z: HOME_SPOT[1], r: 1.5, label: copy.room.goHome, enabled: true, act: () => travel('room') }]

  // 位置只写进 usePositions；定位与靠近扫描仍由 useParkSensors 上报。
  const onMove = (x: number, z: number) => {
    pos.current = [x, z]
    const now = performance.now()
    if (now - sent.current.at < SYNC_MS) return
    sent.current.at = now
    const ll = worldToLatLng(x, z)
    const last = sent.current.last
    if (last && last.lat === ll.lat && last.lng === ll.lng) return
    sent.current.last = ll
    usePositions.getState().move(mac, ll)
  }

  return (
    <>
      <color attach="background" args={['#bfe3ff']} />
      <fog attach="fog" args={['#cfe9ff', 25, 70]} />
      <hemisphereLight args={['#ffffff', '#c9e8b0', 1.6]} />
      <directionalLight position={[12, 20, 8]} intensity={0.9} />
      <Ground />
      <Scenery />
      <Home />
      <Peers viewer={mac} />
      <FirstPersonRig
        start={start}
        yaw={-Math.PI / 2}
        halfX={MAP_W / S / 2 - 0.5}
        halfZ={MAP_H / S / 2 - 0.5}
        device={device}
        onMove={onMove}
        // 别的玩偶是实心的：贴近但不穿进去。
        blockers={() => [
          [HOME[0], HOME[1], 1.7],
          ...Object.values(useWorld.getState().devices)
            .filter((o) => o.device !== mac && o.online && o.pos)
            .map((o): [number, number, number] => [...llToWorld(o.pos!), BODY]),
        ]}
      />
      <Interactor spots={spots} pos={pos} />
      <HeldPlush device={device} />
    </>
  )
}

export function ParkWalk({ device }: { device: VirtualDevice }) {
  return (
    <Canvas flat camera={{ fov: 70, near: 0.05, far: 120 }} className="touch-none">
      <Scene device={device} />
    </Canvas>
  )
}
