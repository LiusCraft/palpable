import { ContactShadows } from '@react-three/drei'
import { Canvas, useFrame } from '@react-three/fiber'
import { useMemo, useRef, useState } from 'react'
import { CanvasTexture, SRGBColorSpace, type Group } from 'three'
import { Plush } from '../../plush/Plush'
import { copy } from '../../ui/copy'
import { Bubble, Button } from '../../ui/components'

// 开箱是 Web 展示行为，不是设备行为：点击盒子 → 盒盖打开 → 玩偶出来 → 放到桌面。

type Phase = 'closed' | 'opening' | 'out'

const KRAFT = '#c9a27a'
const KRAFT_DARK = '#b08a62'

/** 盒面印刷：小熊脸与品牌名，画在 canvas 上当贴图。 */
function usePrint(): CanvasTexture {
  return useMemo(() => {
    const c = document.createElement('canvas')
    c.width = 512
    c.height = 340
    const g = c.getContext('2d')!
    g.fillStyle = KRAFT
    g.fillRect(0, 0, c.width, c.height)
    g.strokeStyle = '#6b4e33'
    g.fillStyle = '#6b4e33'
    g.lineWidth = 7
    // 小熊头像
    const cx = 256
    const cy = 118
    for (const dx of [-62, 62]) {
      g.beginPath()
      g.arc(cx + dx, cy - 52, 26, 0, Math.PI * 2)
      g.stroke()
    }
    g.beginPath()
    g.arc(cx, cy, 72, 0, Math.PI * 2)
    g.stroke()
    for (const dx of [-26, 26]) {
      g.beginPath()
      g.arc(cx + dx, cy - 8, 7, 0, Math.PI * 2)
      g.fill()
    }
    g.beginPath()
    g.ellipse(cx, cy + 20, 12, 8, 0, 0, Math.PI * 2)
    g.fill()
    g.textAlign = 'center'
    g.font = 'bold 64px "PingFang SC","Microsoft YaHei",sans-serif'
    g.fillText('可感', cx, 262)
    g.font = '36px system-ui,sans-serif'
    g.fillText('Palpable', cx, 310)
    const t = new CanvasTexture(c)
    t.colorSpace = SRGBColorSpace
    return t
  }, [])
}

function Box({ phase, onOpen }: { phase: Phase; onOpen: () => void }) {
  const print = usePrint()
  const flaps = useRef<Group[]>([])
  const open = useRef(0)
  useFrame((_, dt) => {
    const target = phase === 'closed' ? 0 : 1
    open.current += (target - open.current) * Math.min(1, dt * 3)
    const a = open.current * 2.1
    flaps.current[0]?.rotation.set(-a, 0, 0)
    flaps.current[1]?.rotation.set(a, 0, 0)
    flaps.current[2]?.rotation.set(0, 0, a)
    flaps.current[3]?.rotation.set(0, 0, -a)
  })
  const W = 1.8
  const D = 1.4
  const H = 1.2
  return (
    <group
      position={[0, -0.4, 0]}
      onClick={(e) => {
        e.stopPropagation()
        onOpen()
      }}
      onPointerOver={() => phase === 'closed' && (document.body.style.cursor = 'pointer')}
      onPointerOut={() => (document.body.style.cursor = '')}
    >
      {/* 四壁与底，开口朝上 */}
      <mesh position={[0, -H / 2, 0]}>
        <boxGeometry args={[W, 0.04, D]} />
        <meshStandardMaterial color={KRAFT_DARK} />
      </mesh>
      {[
        [0, 0, D / 2, W, H, 0.04],
        [0, 0, -D / 2, W, H, 0.04],
        [W / 2, 0, 0, 0.04, H, D],
        [-W / 2, 0, 0, 0.04, H, D],
      ].map(([x, y, z, w, h, d], i) => (
        <mesh key={i} position={[x, y, z]}>
          <boxGeometry args={[w, h, d]} />
          <meshStandardMaterial color={KRAFT} roughness={0.9} />
        </mesh>
      ))}
      {/* 盒盖四片：铰链在上沿 */}
      {[
        { pos: [0, H / 2, D / 2], size: [W, 0.03, D / 2], offset: [0, 0, -D / 4] },
        { pos: [0, H / 2, -D / 2], size: [W, 0.03, D / 2], offset: [0, 0, D / 4] },
        { pos: [W / 2, H / 2, 0], size: [W / 2, 0.03, D], offset: [-W / 4, 0, 0] },
        { pos: [-W / 2, H / 2, 0], size: [W / 2, 0.03, D], offset: [W / 4, 0, 0] },
      ].map((f, i) => (
        <group
          key={i}
          position={f.pos as [number, number, number]}
          ref={(g) => {
            if (g) flaps.current[i] = g
          }}
        >
          <mesh position={f.offset as [number, number, number]}>
            <boxGeometry args={f.size as [number, number, number]} />
            <meshStandardMaterial color={i < 2 ? KRAFT : KRAFT_DARK} roughness={0.9} />
          </mesh>
        </group>
      ))}
      {/* 盒面印刷 */}
      <mesh position={[0, 0, D / 2 + 0.022]}>
        <planeGeometry args={[W * 0.86, H * 0.86]} />
        <meshStandardMaterial map={print} roughness={0.9} />
      </mesh>
    </group>
  )
}

function RisingPlush({ phase, skin }: { phase: Phase; skin: string }) {
  const g = useRef<Group>(null)
  const y = useRef(-1.2)
  const x = useRef(0)
  useFrame((_, dt) => {
    const ty = phase === 'closed' ? -0.6 : phase === 'opening' ? 0.75 : 0.2
    const tx = phase === 'out' ? 1.9 : 0
    y.current += (ty - y.current) * Math.min(1, dt * 2.2)
    x.current += (tx - x.current) * Math.min(1, dt * 2.2)
    g.current?.position.set(x.current, y.current, phase === 'out' ? 0.3 : 0)
  })
  return (
    <group ref={g} visible={phase !== 'closed'}>
      <Plush skin={skin} light="idle" scale={0.7} />
    </group>
  )
}

export function Unbox({ skin, onDone }: { skin: string; onDone: () => void }) {
  const [phase, setPhase] = useState<Phase>('closed')
  return (
    <div className="relative h-[460px] overflow-hidden rounded-2xl bg-gradient-to-b from-[#fbe9d6] to-[#f3d5b5]">
      <Canvas camera={{ position: [0.6, 1.6, 5.2], fov: 42 }} shadows>
        <ambientLight intensity={0.75} />
        <directionalLight position={[3, 5, 4]} intensity={1.6} castShadow />
        <group position={[-0.6, 0, 0]}>
          <Box phase={phase} onOpen={() => phase === 'closed' && setPhase('opening')} />
          <RisingPlush phase={phase} skin={skin} />
        </group>
        <ContactShadows position={[0, -1.05, 0]} opacity={0.35} scale={10} blur={2.5} />
      </Canvas>

      {phase === 'closed' && (
        <div className="absolute inset-x-0 top-4 flex justify-center">
          <button
            type="button"
            aria-label="打开盒子"
            onClick={() => setPhase('opening')}
            className="rounded-full bg-white/90 px-4 py-1 text-sm text-slate-600 shadow-sm transition hover:bg-white"
          >
            {copy.unbox.hint}
          </button>
        </div>
      )}

      {phase !== 'closed' && (
        <div className="absolute right-6 top-16 w-64 animate-[pop_0.4s_ease-out]">
          <Bubble side="left">{copy.unbox.caption}</Bubble>
        </div>
      )}

      <div className="absolute inset-x-0 bottom-5 flex justify-center gap-2">
        {phase === 'opening' && <Button onClick={() => setPhase('out')}>{copy.unbox.take}</Button>}
        {phase === 'out' && <Button onClick={onDone}>{copy.unbox.place}</Button>}
      </div>
    </div>
  )
}
