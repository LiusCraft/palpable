import { useFrame } from '@react-three/fiber'
import { useRef } from 'react'
import type { Group, Mesh, MeshStandardMaterial } from 'three'
import type { LightState } from '../shared/protocol.gen'
import { LIGHT_COLORS, SKINS, type SkinLook } from './looks'

// 玩偶与颈部灯环。外观由皮套决定，灯色与动画由设备状态驱动；这里只负责渲染，不做业务判断。

export interface PlushProps {
  skin: string
  light: LightState
  /** 覆盖灯色：设备工具设置的颜色，或所在频道的颜色。 */
  color?: string
  speaking?: boolean
  shakeAt?: number
  onTap?: () => void
  position?: [number, number, number]
  scale?: number
}

function Ear({ look, side }: { look: SkinLook; side: 1 | -1 }) {
  switch (look.ear) {
    case 'long':
      return (
        <group position={[0.28 * side, 0.62, 0]} rotation={[0, 0, -0.18 * side]}>
          <mesh scale={[0.13, 0.45, 0.09]}>
            <sphereGeometry args={[1, 24, 24]} />
            <meshStandardMaterial color={look.body} roughness={0.95} />
          </mesh>
          <mesh position={[0, 0, 0.05]} scale={[0.07, 0.33, 0.04]}>
            <sphereGeometry args={[1, 16, 16]} />
            <meshStandardMaterial color={look.earInner} roughness={0.9} />
          </mesh>
        </group>
      )
    case 'triangle':
      return (
        <mesh position={[0.32 * side, 0.45, 0]} rotation={[0, 0, -0.35 * side]}>
          <coneGeometry args={[0.16, 0.3, 4]} />
          <meshStandardMaterial color={look.body} roughness={0.95} />
        </mesh>
      )
    case 'spike':
      return (
        <mesh position={[0.22 * side, 0.5, -0.05]} rotation={[0, 0, -0.25 * side]}>
          <coneGeometry args={[0.1, 0.24, 12]} />
          <meshStandardMaterial color={look.earInner} roughness={0.8} />
        </mesh>
      )
    default:
      return (
        <group position={[0.36 * side, 0.42, 0]}>
          <mesh>
            <sphereGeometry args={[0.16, 24, 24]} />
            <meshStandardMaterial color={look.body} roughness={0.95} />
          </mesh>
          <mesh position={[0, 0, 0.08]} scale={[1, 1, 0.5]}>
            <sphereGeometry args={[0.09, 16, 16]} />
            <meshStandardMaterial color={look.earInner} roughness={0.9} />
          </mesh>
        </group>
      )
  }
}

export function Plush({ skin, light, color, speaking, shakeAt = 0, onTap, position = [0, 0, 0], scale = 1 }: PlushProps) {
  const look = SKINS[skin] ?? SKINS.bear
  const root = useRef<Group>(null)
  const head = useRef<Group>(null)
  const ring = useRef<Mesh>(null)
  const glow = color ?? LIGHT_COLORS[light]

  useFrame(({ clock }) => {
    const t = clock.getElapsedTime()
    const since = (Date.now() - shakeAt) / 1000
    if (root.current) {
      root.current.position.y = position[1] + Math.sin(t * 1.6) * 0.02
      root.current.rotation.z = since < 0.6 ? Math.sin(since * 50) * 0.08 * (1 - since / 0.6) : 0
    }
    if (head.current) head.current.rotation.x = speaking ? Math.sin(t * 9) * 0.05 : 0
    const mat = ring.current?.material as MeshStandardMaterial | undefined
    if (mat) {
      const pulse = light === 'listening' || light === 'peer_talking' ? 3.2 : light === 'thinking' ? 5 : 1.6
      mat.emissiveIntensity = 1.4 + Math.sin(t * pulse) * 0.7
    }
  })

  return (
    <group
      ref={root}
      position={position}
      scale={scale}
      onClick={(e) => {
        e.stopPropagation()
        onTap?.()
      }}
      onPointerOver={() => onTap && (document.body.style.cursor = 'pointer')}
      onPointerOut={() => (document.body.style.cursor = '')}
    >
      {/* 身体 */}
      <mesh position={[0, -0.55, 0]} scale={[0.62, 0.6, 0.55]}>
        <sphereGeometry args={[1, 40, 40]} />
        <meshStandardMaterial color={look.body} roughness={0.97} />
      </mesh>
      <mesh position={[0, -0.6, 0.36]} scale={[0.36, 0.38, 0.22]}>
        <sphereGeometry args={[1, 32, 32]} />
        <meshStandardMaterial color={look.belly} roughness={0.97} />
      </mesh>
      {[-1, 1].map((s) => (
        <mesh key={`arm${s}`} position={[0.55 * s, -0.5, 0.12]} scale={[0.17, 0.26, 0.17]} rotation={[0, 0, 0.5 * s]}>
          <sphereGeometry args={[1, 20, 20]} />
          <meshStandardMaterial color={look.body} roughness={0.97} />
        </mesh>
      ))}
      {[-1, 1].map((s) => (
        <mesh key={`leg${s}`} position={[0.3 * s, -1.08, 0.22]} scale={[0.2, 0.14, 0.24]}>
          <sphereGeometry args={[1, 20, 20]} />
          <meshStandardMaterial color={look.body} roughness={0.97} />
        </mesh>
      ))}

      {/* 颈部灯环：设备的灯带 */}
      <mesh ref={ring} position={[0, -0.12, 0]} rotation={[Math.PI / 2 - 0.12, 0, 0]}>
        <torusGeometry args={[0.44, 0.045, 16, 64]} />
        <meshStandardMaterial color="#ffffff" emissive={glow} emissiveIntensity={1.5} toneMapped={false} />
      </mesh>
      <mesh position={[0, -0.2, 0.44]}>
        <sphereGeometry args={[0.07, 20, 20]} />
        <meshStandardMaterial color="#ffffff" emissive={glow} emissiveIntensity={2} toneMapped={false} />
      </mesh>

      {/* 头 */}
      <group ref={head} position={[0, 0.3, 0]}>
        <mesh scale={[0.56, 0.5, 0.5]}>
          <sphereGeometry args={[1, 40, 40]} />
          <meshStandardMaterial color={look.body} roughness={0.97} />
        </mesh>
        <Ear look={look} side={1} />
        <Ear look={look} side={-1} />
        {look.ear === 'spike' && (
          <mesh position={[0, 0.55, -0.1]} rotation={[-0.3, 0, 0]}>
            <coneGeometry args={[0.09, 0.22, 12]} />
            <meshStandardMaterial color={look.earInner} roughness={0.8} />
          </mesh>
        )}
        <mesh position={[0, -0.1, 0.4]} scale={[0.24, 0.17, 0.14]}>
          <sphereGeometry args={[1, 24, 24]} />
          <meshStandardMaterial color={look.belly} roughness={0.95} />
        </mesh>
        <mesh position={[0, -0.04, 0.53]} scale={[0.07, 0.05, 0.04]}>
          <sphereGeometry args={[1, 16, 16]} />
          <meshStandardMaterial color={look.nose} roughness={0.5} />
        </mesh>
        {[-1, 1].map((s) => (
          <group key={`eye${s}`}>
            <mesh position={[0.19 * s, 0.07, 0.44]}>
              <sphereGeometry args={[0.055, 20, 20]} />
              <meshStandardMaterial color="#1c1c22" roughness={0.3} />
            </mesh>
            <mesh position={[0.2 * s, 0.09, 0.49]}>
              <sphereGeometry args={[0.016, 12, 12]} />
              <meshStandardMaterial color="#ffffff" emissive="#ffffff" emissiveIntensity={0.6} />
            </mesh>
            <mesh position={[0.31 * s, -0.07, 0.38]} scale={[1, 0.6, 0.4]}>
              <sphereGeometry args={[0.07, 16, 16]} />
              <meshStandardMaterial color="#ff9fb3" transparent opacity={0.7} roughness={1} />
            </mesh>
          </group>
        ))}
      </group>
    </group>
  )
}
