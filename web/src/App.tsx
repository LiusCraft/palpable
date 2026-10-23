import { useEffect } from 'react'
import { useStore } from 'zustand'
import type { VirtualDevice } from './device-sim/device'
import { useDevices } from './device-sim/devices'
import { connectWorld, useWorld } from './map/world'
import { useDemo, type Scene } from './scenes/demo'
import { Desk } from './scenes/desk/Desk'
import { ParkStage } from './scenes/park/ParkStage'
import { Unbox } from './scenes/unbox/Unbox'
import { copy } from './ui/copy'
import { Button, Card, StepHeader } from './ui/components'

const SCENE_OF_STEP: Scene[] = ['unbox', 'desk', 'park', 'park', 'park']

export default function App() {
  const { devices, active } = useDevices()
  const { scene, viewing, setScene, view } = useDemo()
  useEffect(connectWorld, [])

  const device = devices.find((d) => d.mac === (viewing ?? active)) ?? devices[0]
  const parkStep = useParkStep(device)
  const step = scene === 'unbox' ? 0 : scene === 'desk' ? 1 : parkStep

  return (
    <div className="mx-auto max-w-7xl px-4 pb-16 pt-5 sm:px-6">
      <Header />
      <StepNav current={step} onPick={(i) => setScene(SCENE_OF_STEP[i])} />
      <KidSwitcher current={device} onPick={view} />

      <Card className="mt-4">
        <StepHeader n={step + 1} title={copy.steps[step].title} sub={copy.steps[step].sub} tag={copy.steps[step].web ? 'Web 展示' : undefined} />
        {device && scene === 'unbox' && <UnboxStage device={device} onDone={() => setScene('desk')} />}
        {device && scene === 'desk' && <Desk device={device} onPark={() => setScene('park')} />}
        {scene === 'park' && <ParkStage />}
      </Card>
    </div>
  )
}

// 公园里的进度：有好友后到第 4 步，好友也在同一频道时到第 5 步。
function useParkStep(device?: VirtualDevice): number {
  const fallback = useDevices((s) => s.devices[0])
  const s = useStore((device ?? fallback).store)
  if (Object.keys(s.present).length > 0) return 4
  if (Object.values(s.friends).some((f) => f.status === 'confirmed')) return 3
  return 2
}

function UnboxStage({ device, onDone }: { device: VirtualDevice; onDone: () => void }) {
  const skin = useStore(device.store, (s) => s.skin)
  return <Unbox skin={skin} onDone={onDone} />
}

function Header() {
  const worldOn = useWorld((w) => w.connected)
  return (
    <header className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-3">
        <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-[#2f6bff] text-2xl shadow">🧸</div>
        <div className="flex items-baseline gap-2">
          <span className="text-3xl font-black text-[#1d3557]">{copy.brand.name}</span>
          <span className="text-2xl font-semibold text-[#2f6bff]">{copy.brand.en}</span>
        </div>
        <span className="hidden text-sm text-slate-500 md:inline">{copy.brand.slogan}</span>
      </div>
      <div className="flex items-center gap-3 text-sm text-slate-500">
        <span className={`h-2 w-2 rounded-full ${worldOn ? 'bg-emerald-400' : 'bg-slate-300'}`} title="演示地图通道" />
        {copy.brand.badge}
      </div>
    </header>
  )
}

function StepNav({ current, onPick }: { current: number; onPick: (i: number) => void }) {
  return (
    <nav className="mt-4 flex flex-wrap gap-2">
      {copy.steps.map((s, i) => (
        <button
          key={s.key}
          type="button"
          onClick={() => onPick(i)}
          className={`inline-flex items-center gap-2 rounded-full px-3 py-1.5 text-sm transition ${
            i === current ? 'bg-[#2f6bff] text-white shadow' : i < current ? 'bg-white text-[#2f6bff] ring-1 ring-sky-200' : 'bg-white/70 text-slate-500 ring-1 ring-slate-200'
          }`}
        >
          <span className={`flex h-5 w-5 items-center justify-center rounded-full text-xs ${i === current ? 'bg-white text-[#2f6bff]' : 'bg-slate-100'}`}>{i + 1}</span>
          {s.title}
        </button>
      ))}
    </nav>
  )
}

function KidSwitcher({ current, onPick }: { current?: VirtualDevice; onPick: (mac: string) => void }) {
  const { devices, add, active } = useDevices()
  const names = useWorld((w) => w.devices)
  return (
    <div className="mt-3 flex flex-wrap items-center gap-2 text-sm">
      <span className="text-slate-500">小朋友窗口：</span>
      {devices.map((d, i) => {
        const name = names[d.mac]?.name
        return (
          <button
            key={d.mac}
            type="button"
            onClick={() => onPick(d.mac)}
            className={`rounded-full px-3 py-1 ring-1 ${d === current ? 'bg-[#1d3557] text-white ring-[#1d3557]' : 'bg-white text-slate-600 ring-slate-200'}`}
          >
            {name || `玩偶 ${i + 1}`}
            {d.mac === active && ' 🎤'}
          </button>
        )
      })}
      <Button tone="ghost" onClick={() => onPick(add(devices.length % 2 ? 'rabbit' : 'cat').mac)}>
        + {copy.device.add}
      </Button>
    </div>
  )
}
