import { ContactShadows } from '@react-three/drei'
import { Canvas } from '@react-three/fiber'
import { useEffect, useRef, useState } from 'react'
import { useStore } from 'zustand'
import type { VirtualDevice } from '../../device-sim/device'
import { setSceneDescriber } from '../../device-sim/environment'
import { useWorld } from '../../map/world'
import { DevicePlush } from '../../plush/DevicePlush'
import { LIGHT_COLORS } from '../../plush/looks'
import { copy } from '../../ui/copy'
import { Button, Chip, Wave } from '../../ui/components'
import { DemoNaming } from './DemoNaming'
import { MicControls } from './MicControls'

// 桌面场景：起名引导与一对一聊天的画面。设备没有屏幕，这里的对话气泡只表现"谁在说话"，
// 小伙伴说的第一句取自文案表，与服务端的欢迎语一致。

interface Turn {
  who: 'ai' | 'kid'
  id: number
}

function Desk3D({ device }: { device: VirtualDevice }) {
  return (
    <Canvas camera={{ position: [0, 0.5, 4.2], fov: 40 }} shadows>
      <ambientLight intensity={0.7} />
      <directionalLight position={[2.5, 4, 3]} intensity={1.5} castShadow />
      <pointLight position={[-2.2, 1.6, 0.5]} intensity={6} color="#ffd9a0" />
      {/* 桌面 */}
      <mesh position={[0, -1.25, 0]} receiveShadow>
        <boxGeometry args={[6, 0.12, 3]} />
        <meshStandardMaterial color="#d9b48c" roughness={0.7} />
      </mesh>
      {/* 台灯 */}
      <group position={[-2, -1.2, -0.6]}>
        <mesh position={[0, 0.05, 0]}>
          <cylinderGeometry args={[0.28, 0.32, 0.08, 32]} />
          <meshStandardMaterial color="#f4f1ea" />
        </mesh>
        <mesh position={[0, 0.7, 0]}>
          <cylinderGeometry args={[0.03, 0.03, 1.3, 12]} />
          <meshStandardMaterial color="#f4f1ea" />
        </mesh>
        <mesh position={[0.15, 1.35, 0]} rotation={[0, 0, -0.5]}>
          <coneGeometry args={[0.3, 0.4, 32, 1, true]} />
          <meshStandardMaterial color="#fff6e5" emissive="#ffd9a0" emissiveIntensity={0.6} side={2} />
        </mesh>
      </group>
      {/* 小书 */}
      <mesh position={[1.9, -1.12, -0.4]} rotation={[0, 0.4, 0]}>
        <boxGeometry args={[0.8, 0.14, 0.6]} />
        <meshStandardMaterial color="#7fb8ff" />
      </mesh>
      <DevicePlush device={device} position={[0, 0, 0]} />
      <ContactShadows position={[0, -1.18, 0]} opacity={0.35} scale={8} blur={2.4} />
    </Canvas>
  )
}

export function Desk({ device, onPark }: { device: VirtualDevice; onPark: () => void }) {
  const s = useStore(device.store)
  const name = useWorld((w) => w.devices[s.mac]?.name ?? '')
  const [turns, setTurns] = useState<Turn[]>([])
  const seq = useRef(0)

  // 进入桌面即上线（相当于把玩偶放下、开机）。
  useEffect(() => {
    if (device.store.getState().link === 'offline') device.connect()
  }, [device])

  useEffect(() => {
    setSceneDescriber(() => '眼前是一张木头小桌子，上面有一盏暖黄色的台灯和一本蓝色的图画书。')
  }, [])

  // 每次开口记一个回合，气泡只保留最近几轮。
  useEffect(() => {
    if (s.talking) setTurns((t) => [...t.slice(-5), { who: 'kid', id: ++seq.current }])
  }, [s.talking])
  useEffect(() => {
    if (s.speaking && s.source === 'ai') setTurns((t) => [...t.slice(-5), { who: 'ai', id: ++seq.current }])
  }, [s.speaking, s.source])

  const companion = name || '小可'
  const last = turns[turns.length - 1]

  return (
    <div className="grid gap-4 md:grid-cols-[1.3fr_1fr]">
      <div className="relative h-[420px] overflow-hidden rounded-2xl bg-gradient-to-b from-[#fff6ea] to-[#f5e6d3]">
        <Desk3D device={device} />
        <div className="absolute left-4 top-4 flex flex-wrap gap-2">
          <Chip tone={s.link === 'online' ? 'green' : 'gray'}>{copy.link[s.link]}</Chip>
          <Chip>
            <span className="inline-block h-2 w-2 rounded-full" style={{ background: s.lightOverride?.color ?? LIGHT_COLORS[s.light] }} />
            {copy.light[s.light]}
          </Chip>
          {name && <Chip tone="pink">{name}</Chip>}
        </div>
        <p className="absolute inset-x-0 bottom-3 text-center text-xs text-slate-500">{copy.desk.tapToTalk}</p>
      </div>

      <div className="flex flex-col rounded-2xl bg-white p-4 ring-1 ring-sky-100">
        <div className="mb-3 flex items-center justify-between">
          <div className="text-sm font-semibold text-[#1d3557]">{name ? copy.desk.namedAs(name, '小朋友') : copy.desk.unnamed}</div>
          <span className="text-xs text-slate-400">{s.mac}</span>
        </div>

        <div className="flex min-h-56 flex-1 flex-col gap-3">
          {!name && (
            <Row who="ai" label="小可">
              {copy.desk.welcomeBubble}
            </Row>
          )}
          {turns.map((t) => (
            <Row key={t.id} who={t.who} label={t.who === 'ai' ? companion : '我'}>
              <span className="inline-flex items-center gap-2">
                <Wave active={t === last && (t.who === 'ai' ? s.speaking : s.talking)} tone={t.who === 'ai' ? 'blue' : 'pink'} />
                {t.who === 'ai' ? `${companion}${t === last && s.speaking ? '正在说…' : '说完了'}` : t === last && s.talking ? '我在说…' : '说完了'}
              </span>
            </Row>
          ))}
          {s.light === 'thinking' && (
            <Row who="ai" label={companion}>
              <span className="text-slate-400">在想呢…</span>
            </Row>
          )}
        </div>

        {s.lastError && <p className="mt-2 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{s.lastError}</p>}
        {s.link === 'online' && !name && s.lastError && <DemoNaming mac={s.mac} />}
        <MicControls device={device} />
        <Button className="mt-3 self-end" onClick={onPark}>
          {copy.desk.toPark} →
        </Button>
      </div>
    </div>
  )
}

function Row({ who, label, children }: { who: 'ai' | 'kid'; label: string; children: React.ReactNode }) {
  const mine = who === 'kid'
  return (
    <div className={`flex items-end gap-2 ${mine ? 'flex-row-reverse' : ''}`}>
      <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs ${mine ? 'bg-sky-100' : 'bg-amber-100'}`}>
        {mine ? '🧒' : '🧸'}
      </span>
      <div className={`max-w-[75%] rounded-2xl px-3 py-2 text-sm ${mine ? 'bg-[#2f6bff] text-white' : 'bg-slate-100 text-slate-700'}`}>
        <div className={`mb-0.5 text-[10px] ${mine ? 'text-white/70' : 'text-slate-400'}`}>{label}</div>
        {children}
      </div>
    </div>
  )
}
