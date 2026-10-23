import { useFrame } from '@react-three/fiber'
import { useRef } from 'react'
import type { Mesh, MeshStandardMaterial } from 'three'
import type { LightState } from '../shared/protocol.gen'

// 灯带颜色由设备的 light 状态驱动；这里只负责渲染，不做业务判断。
const COLORS: Record<LightState, string> = {
  idle: '#9db7ff',
  listening: '#7fe0ff',
  thinking: '#ffd37f',
  speaking: '#ff9fc4',
  peer_talking: '#9fffb4',
  muted: '#8a8a8a',
}

export function Plush({ light, onTap }: { light: LightState; onTap: () => void }) {
  const body = useRef<Mesh>(null)
  const strip = useRef<Mesh>(null)

  useFrame(({ clock }) => {
    const t = clock.getElapsedTime()
    if (body.current) body.current.position.y = Math.sin(t * 1.5) * 0.04
    const mat = strip.current?.material as MeshStandardMaterial | undefined
    if (mat) mat.emissiveIntensity = 1.2 + Math.sin(t * 3) * 0.6
  })

  return (
    <group onClick={onTap}>
      <mesh ref={body}>
        <sphereGeometry args={[1, 48, 48]} />
        <meshStandardMaterial color="#8fb4ff" roughness={0.95} />
      </mesh>
      <mesh ref={strip} position={[0, 0, 0.98]}>
        <capsuleGeometry args={[0.07, 0.8, 8, 16]} />
        <meshStandardMaterial color="#ffffff" emissive={COLORS[light]} emissiveIntensity={1.5} />
      </mesh>
    </group>
  )
}
