import { useFrame } from '@react-three/fiber'
import { useMemo, useRef } from 'react'
import { CanvasTexture, SRGBColorSpace, type Group } from 'three'

// 包装盒：四片盒盖真的会翻开。盒面印着小熊脸与品牌名，画在 canvas 上当贴图。

const KRAFT = '#c9a27a'
const KRAFT_DARK = '#b08a62'
const W = 1.8
const D = 1.4
const H = 1.2
/** 盒子整体缩放：开箱时的盒子大小约等于一个孩子抱得动的纸箱。 */
export const BOX_SCALE = 0.55

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

export function Box({ open, position }: { open: boolean; position: [number, number, number] }) {
  const print = usePrint()
  const flaps = useRef<Group[]>([])
  const progress = useRef(0)
  useFrame((_, dt) => {
    progress.current += ((open ? 1 : 0) - progress.current) * Math.min(1, dt * 3)
    const a = progress.current * 2.1
    flaps.current[0]?.rotation.set(-a, 0, 0)
    flaps.current[1]?.rotation.set(a, 0, 0)
    flaps.current[2]?.rotation.set(0, 0, a)
    flaps.current[3]?.rotation.set(0, 0, -a)
  })
  return (
    <group position={[position[0], position[1] + (H / 2) * BOX_SCALE, position[2]]} scale={BOX_SCALE}>
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
