import { ContactShadows } from '@react-three/drei'
import { Canvas } from '@react-three/fiber'
import { useState } from 'react'
import { Plush } from '../plush/Plush'

// 官网首屏的 3D：四只玩偶并排坐在石头上，脖子上的灯环各亮各的色。点一只，它会抖一抖打招呼。
// 只负责好看，不连设备。

interface Member {
  skin: string
  at: [number, number]
  scale: number
  turn: number
  glow: string
}

const FAMILY: Member[] = [
  { skin: 'rabbit', at: [-2.35, 0.5], scale: 1.0, turn: 0.28, glow: '#ff6ad5' },
  { skin: 'bear', at: [-0.35, -0.25], scale: 1.5, turn: 0.04, glow: '#4aa8ff' },
  { skin: 'dragon', at: [1.75, 0.35], scale: 1.12, turn: -0.22, glow: '#3be0a8' },
  { skin: 'panda', at: [3.3, 0.3], scale: 0.95, turn: -0.38, glow: '#4aa8ff' },
]

export function Family() {
  const [shook, setShook] = useState<Record<string, number>>({})
  return (
    <Canvas flat camera={{ position: [0.2, 2.6, 9.4], fov: 34 }} onCreated={({ camera }) => camera.lookAt(0.2, 1.7, 0)}>
      <hemisphereLight args={['#ffffff', '#cfe6c4', 2.2]} />
      <directionalLight position={[3, 5, 6]} intensity={1.1} />
      {FAMILY.map((m) => (
        <group key={m.skin} position={[m.at[0], 0, m.at[1]]} rotation={[0, m.turn, 0]}>
          <Plush
            skin={m.skin}
            light="idle"
            color={m.glow}
            position={[0, 1.22 * m.scale, 0]}
            scale={m.scale}
            shakeAt={shook[m.skin] ?? 0}
            onTap={() => setShook((s) => ({ ...s, [m.skin]: Date.now() }))}
          />
        </group>
      ))}
      <ContactShadows position={[0.6, 0.01, 0.2]} opacity={0.3} scale={11} blur={2.4} />
    </Canvas>
  )
}
