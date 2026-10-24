import { Canvas, useFrame } from '@react-three/fiber'
import { useEffect, useMemo, useRef } from 'react'
import { useStore } from 'zustand'
import { activate, markUnboxed } from '../../device-sim/client'
import type { VirtualDevice } from '../../device-sim/device'
import { setSceneDescriber } from '../../device-sim/environment'
import { DevicePlush } from '../../plush/DevicePlush'
import { Plush } from '../../plush/Plush'
import { FirstPersonRig, HeldPlush } from '../world/FirstPerson'
import { Interactor, type Spot } from '../world/Interactor'
import { copy } from '../../ui/copy'
import { useJourney } from '../journey'
import { Box, BOX_SCALE } from './Box'
import type { Group } from 'three'

// 房间：孩子站在自己的房间里，真的走过去打开盒子、抱出玩偶、放到桌面上，它就醒了。
// 单位与公园一致（1 单位约 1 米），玩偶缩成一个孩子抱得动的大小。

const HALF_X = 5
const HALF_Z = 4
const WALL_H = 3.2
const DESK: [number, number, number] = [-2.5, 0.62, -3.35]
const BOX: [number, number, number] = [1.5, 0, 0.6]
const DOOR_Z = 0
const PLUSH = 0.4
const STAND = 1.22 * PLUSH // 玩偶原点到脚底的距离

function Wall({ pos, size, rot = [0, 0, 0] }: { pos: [number, number, number]; size: [number, number]; rot?: [number, number, number] }) {
  return (
    <mesh position={pos} rotation={rot}>
      <planeGeometry args={size} />
      <meshStandardMaterial color="#fbe9d6" roughness={1} side={2} />
    </mesh>
  )
}

function Furniture() {
  return (
    <group>
      {/* 地板、地毯、天花板 */}
      <mesh rotation={[-Math.PI / 2, 0, 0]}>
        <planeGeometry args={[HALF_X * 2, HALF_Z * 2]} />
        <meshStandardMaterial color="#d9b48c" roughness={0.8} />
      </mesh>
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0.5, 0.01, 0.6]}>
        <circleGeometry args={[2.4, 48]} />
        <meshStandardMaterial color="#f6b8c8" roughness={1} />
      </mesh>
      <mesh rotation={[Math.PI / 2, 0, 0]} position={[0, WALL_H, 0]}>
        <planeGeometry args={[HALF_X * 2, HALF_Z * 2]} />
        <meshStandardMaterial color="#fff6ea" roughness={1} />
      </mesh>
      {/* 四面墙 */}
      <Wall pos={[0, WALL_H / 2, -HALF_Z]} size={[HALF_X * 2, WALL_H]} />
      <Wall pos={[0, WALL_H / 2, HALF_Z]} size={[HALF_X * 2, WALL_H]} />
      <Wall pos={[-HALF_X, WALL_H / 2, 0]} size={[HALF_Z * 2, WALL_H]} rot={[0, Math.PI / 2, 0]} />
      <Wall pos={[HALF_X, WALL_H / 2, 0]} size={[HALF_Z * 2, WALL_H]} rot={[0, Math.PI / 2, 0]} />
      {/* 窗户：北墙，外面是晴天 */}
      <group position={[2.6, 1.9, -HALF_Z + 0.02]}>
        <mesh>
          <planeGeometry args={[2.2, 1.4]} />
          <meshStandardMaterial color="#bfe3ff" emissive="#bfe3ff" emissiveIntensity={0.9} />
        </mesh>
        {[
          [0, 0.7, 2.3, 0.1],
          [0, -0.7, 2.3, 0.1],
          [-1.1, 0, 0.1, 1.5],
          [1.1, 0, 0.1, 1.5],
          [0, 0, 0.06, 1.4],
          [0, 0, 2.2, 0.06],
        ].map(([x, y, w, h], i) => (
          <mesh key={i} position={[x, y, 0.02]}>
            <boxGeometry args={[w, h, 0.05]} />
            <meshStandardMaterial color="#ffffff" />
          </mesh>
        ))}
      </group>
      {/* 书架上的几本书 */}
      <mesh position={[-HALF_X + 0.3, 1.2, 2.2]}>
        <boxGeometry args={[0.5, 0.06, 2]} />
        <meshStandardMaterial color="#b98a5e" />
      </mesh>
      {['#7fb8ff', '#ff9fb3', '#ffd24d', '#8fd38a'].map((c, i) => (
        <mesh key={c} position={[-HALF_X + 0.3, 1.45, 1.5 + i * 0.35]}>
          <boxGeometry args={[0.35, 0.5, 0.12]} />
          <meshStandardMaterial color={c} />
        </mesh>
      ))}
    </group>
  )
}

/** 桌子：玩偶放在桌面上的小圈圈里。 */
function Desk({ holding }: { holding: boolean }) {
  const ring = useRef<Group>(null)
  useFrame(({ clock }) => {
    const r = ring.current
    if (!r) return
    r.visible = holding
    r.scale.setScalar(1 + Math.sin(clock.getElapsedTime() * 4) * 0.08)
  })
  return (
    <group>
      <mesh position={[DESK[0], DESK[1] - 0.04, DESK[2]]}>
        <boxGeometry args={[2.4, 0.08, 1.1]} />
        <meshStandardMaterial color="#c99a6b" roughness={0.7} />
      </mesh>
      {[-1.1, 1.1].flatMap((x) => [-0.45, 0.45].map((z) => [x, z] as const)).map(([x, z]) => (
        <mesh key={`${x}${z}`} position={[DESK[0] + x, DESK[1] / 2 - 0.04, DESK[2] + z]}>
          <boxGeometry args={[0.08, DESK[1] - 0.08, 0.08]} />
          <meshStandardMaterial color="#a97c50" />
        </mesh>
      ))}
      {/* 台灯 */}
      <group position={[DESK[0] - 0.95, DESK[1], DESK[2] - 0.2]}>
        <mesh position={[0, 0.03, 0]}>
          <cylinderGeometry args={[0.16, 0.18, 0.05, 24]} />
          <meshStandardMaterial color="#f4f1ea" />
        </mesh>
        <mesh position={[0, 0.4, 0]}>
          <cylinderGeometry args={[0.02, 0.02, 0.75, 8]} />
          <meshStandardMaterial color="#f4f1ea" />
        </mesh>
        <mesh position={[0.08, 0.8, 0]} rotation={[0, 0, -0.5]}>
          <coneGeometry args={[0.2, 0.26, 24, 1, true]} />
          <meshStandardMaterial color="#fff6e5" emissive="#ffd9a0" emissiveIntensity={0.8} side={2} />
        </mesh>
      </group>
      {/* 图画书 */}
      <mesh position={[DESK[0] + 0.85, DESK[1] + 0.03, DESK[2] + 0.1]} rotation={[0, 0.4, 0]}>
        <boxGeometry args={[0.5, 0.06, 0.38]} />
        <meshStandardMaterial color="#7fb8ff" />
      </mesh>
      {/* 放玩偶的圈 */}
      <group ref={ring} position={[DESK[0], DESK[1] + 0.005, DESK[2] + 0.05]} visible={false}>
        <mesh rotation={[-Math.PI / 2, 0, 0]}>
          <ringGeometry args={[0.28, 0.36, 40]} />
          <meshBasicMaterial color="#ffd24d" transparent opacity={0.9} />
        </mesh>
      </group>
    </group>
  )
}

/** 门：抱着玩偶走到门口，点一下去公园。 */
function Door({ glow }: { glow: boolean }) {
  return (
    <group position={[HALF_X - 0.03, 0, DOOR_Z]}>
      <mesh position={[0, 1.1, 0]} rotation={[0, -Math.PI / 2, 0]}>
        <boxGeometry args={[1.3, 2.2, 0.08]} />
        <meshStandardMaterial color="#8fd0e8" roughness={0.6} emissive={glow ? '#ffd24d' : '#000000'} emissiveIntensity={glow ? 0.35 : 0} />
      </mesh>
      <mesh position={[-0.06, 1.05, 0.45]}>
        <sphereGeometry args={[0.06, 12, 12]} />
        <meshStandardMaterial color="#ffd24d" metalness={0.6} roughness={0.3} />
      </mesh>
      {/* 门框与门牌：画着一棵小树，告诉孩子外面是公园 */}
      <mesh position={[0, 2.35, 0]} rotation={[0, -Math.PI / 2, 0]}>
        <boxGeometry args={[1.5, 0.12, 0.12]} />
        <meshStandardMaterial color="#ffffff" />
      </mesh>
      <mesh position={[-0.05, 2.75, 0]}>
        <sphereGeometry args={[0.3, 16, 16]} />
        <meshStandardMaterial color="#5fb85a" />
      </mesh>
      <mesh position={[-0.05, 2.38, 0]}>
        <cylinderGeometry args={[0.04, 0.05, 0.3, 8]} />
        <meshStandardMaterial color="#9b7653" />
      </mesh>
    </group>
  )
}

/** 盒子里的玩偶：盒盖打开后升起来，等孩子抱出来。 */
function BoxPlush({ device, show }: { device: VirtualDevice; show: boolean }) {
  const g = useRef<Group>(null)
  const y = useRef(0)
  const skin = useStore(device.store, (s) => s.skin)
  useFrame((_, dt) => {
    y.current += ((show ? 1 : 0) - y.current) * Math.min(1, dt * 2.5)
    g.current?.position.set(BOX[0], 0.1 + y.current * 0.4, BOX[2])
    if (g.current) g.current.visible = show || y.current > 0.02
  })
  return (
    <group ref={g} visible={false}>
      <Plush skin={skin} light="idle" scale={0.36} position={[0, 1.22 * 0.36, 0]} />
    </group>
  )
}

/** 桌上的玩偶：放下时"噗"地出现；点一下又能抱起来。 */
function DeskPlush({ device, show }: { device: VirtualDevice; show: boolean }) {
  const g = useRef<Group>(null)
  const k = useRef(0)
  useFrame((_, dt) => {
    k.current += ((show ? 1 : 0) - k.current) * Math.min(1, dt * 7)
    g.current?.scale.setScalar(Math.max(0.001, k.current))
    if (g.current) g.current.visible = k.current > 0.01
  })
  return (
    <group ref={g} position={[DESK[0], DESK[1], DESK[2] + 0.05]} visible={false}>
      <DevicePlush device={device} position={[0, STAND, 0]} scale={PLUSH} />
    </group>
  )
}

function Scene({ device }: { device: VirtualDevice }) {
  const { phase, setPhase, travel } = useJourney()
  const start = useMemo<[number, number]>(() => (useJourney.getState().arrival === 'door' ? [HALF_X - 1.8, DOOR_Z] : [0, 3]), [])
  const yaw = useMemo(() => (useJourney.getState().arrival === 'door' ? Math.PI / 2 : 0), [])
  const pos = useRef<[number, number]>(start)

  const place = () => {
    setPhase('placed')
    markUnboxed(device.mac)
    void activate(device)
  }

  // 站在那个位置，按 E。
  const spots: Spot[] = [
    ...(phase === 'boxed' ? [{ x: BOX[0], z: BOX[2], r: 1.7, label: copy.room.openBox, enabled: true, act: () => setPhase('open') }] : []),
    ...(phase === 'open' ? [{ x: BOX[0], z: BOX[2], r: 1.7, label: copy.room.takeOut, enabled: true, act: () => setPhase('held') }] : []),
    ...(phase === 'held' ? [{ x: DESK[0], z: DESK[2] + 1.0, r: 1.5, label: copy.room.putDown, enabled: true, act: place }] : []),
    ...(phase === 'placed' ? [{ x: DESK[0], z: DESK[2] + 1.0, r: 1.5, label: copy.room.pickUp, enabled: true, act: () => setPhase('held') }] : []),
    { x: HALF_X - 1.1, z: DOOR_Z, r: 1.6, label: phase === 'held' ? copy.room.goOut : copy.room.needHold, enabled: phase === 'held', act: () => travel('park') },
  ]

  return (
    <>
      <color attach="background" args={['#fbe9d6']} />
      <hemisphereLight args={['#fff8ec', '#f0d6b4', 2.4]} />
      <pointLight position={[DESK[0] - 0.8, 1.6, DESK[2] + 0.2]} intensity={5} color="#ffd9a0" />
      <pointLight position={[0, 2.8, 0]} intensity={4} color="#fff2dd" />
      <directionalLight position={[2.6, 2.5, -3]} intensity={0.8} />

      <Furniture />
      <Desk holding={phase === 'held'} />
      <Box open={phase !== 'boxed'} position={BOX} />
      <BoxPlush device={device} show={phase === 'open'} />
      <DeskPlush device={device} show={phase === 'placed'} />
      <Door glow={phase === 'held'} />

      <FirstPersonRig
        start={start}
        yaw={yaw}
        halfX={HALF_X - 0.4}
        halfZ={HALF_Z - 0.4}
        device={device}
        onMove={(x, z) => {
          pos.current = [x, z]
        }}
        blockers={() => [
          [DESK[0] - 0.7, DESK[2], 0.95],
          [DESK[0], DESK[2], 0.95],
          [DESK[0] + 0.7, DESK[2], 0.95],
          [BOX[0], BOX[2], 0.75 * BOX_SCALE * 1.6],
        ]}
      />
      <Interactor spots={spots} pos={pos} />
      {phase === 'held' && <HeldPlush device={device} />}
    </>
  )
}

export function Room({ device }: { device: VirtualDevice }) {
  useEffect(() => {
    setSceneDescriber(() => '眼前是一间暖暖的小房间，有一张木头小桌子，桌上有一盏暖黄色的台灯和一本蓝色的图画书，窗外阳光很好。')
  }, [])
  return (
    <Canvas flat camera={{ fov: 70, near: 0.05, far: 60 }} className="touch-none">
      <Scene device={device} />
    </Canvas>
  )
}
