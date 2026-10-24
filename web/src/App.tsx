import { useEffect, useState } from 'react'
import { activate, useClient, wasUnboxed } from './device-sim/client'
import type { VirtualDevice } from './device-sim/device'
import { connectWorld } from './map/world'
import { ParentPanel } from './parent-sim/ParentPanel'
import { Home } from './site/Home'
import { Skins } from './scenes/extras/Skins'
import { Hud } from './scenes/hud/Hud'
import { useJourney } from './scenes/journey'
import { ParkStage } from './scenes/park/ParkStage'
import { Room } from './scenes/room/Room'
import { copy } from './ui/copy'
import { Card } from './ui/components'

// 这个网站就是一台设备的客户端：打开页面，你就站在自己的房间里。
// 家长端是另一个客户端（#/parent），孩子之间只通过服务端互通。

type Route = 'home' | 'play' | 'parent'

function useRoute(): Route {
  const read = (): Route => (location.hash.startsWith('#/parent') ? 'parent' : location.hash.startsWith('#/play') ? 'play' : 'home')
  const [route, setRoute] = useState(read)
  useEffect(() => {
    const on = () => setRoute(read())
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])
  return route
}

export default function App() {
  const route = useRoute()
  if (route === 'home') return <Home />
  return route === 'parent' ? <ParentPage /> : <Client />
}

function ParentPage() {
  useEffect(connectWorld, [])
  return (
    <div className="mx-auto max-w-7xl px-4 pb-16 pt-5 sm:px-6">
      <header className="mb-4 flex items-center gap-3">
        <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-[#2f6bff] text-2xl shadow">🧸</div>
        <span className="text-3xl font-black text-[#1d3557]">{copy.brand.name}</span>
        <span className="text-2xl font-semibold text-[#2f6bff]">{copy.parent.title}</span>
        <span className="text-sm text-slate-500">{copy.parent.sub}</span>
        <a href="#/" className="ml-auto text-sm text-slate-500 hover:text-[#2f6bff]">
          ← 首页
        </a>
      </header>
      <Card>
        <ParentPanel />
      </Card>
    </div>
  )
}

function Client() {
  useEffect(connectWorld, [])
  const device = useClient((s) => s.device)
  useEffect(() => {
    void useClient.getState().start()
  }, [])
  if (!device) return <div className="flex h-dvh items-center justify-center text-slate-400">…</div>
  return <World device={device} />
}

function World({ device }: { device: VirtualDevice }) {
  const place = useJourney((s) => s.place)
  const fade = useJourney((s) => s.fade)
  const back = wasUnboxed(device.mac)
  const entered = useJourney((s) => s.entered)
  useEffect(() => () => useJourney.getState().setEntered(false), [])

  // 拆过箱的玩偶直接在桌面上，进门就醒着。
  const enter = () => {
    useJourney.getState().setEntered(true)
    // 进门这一下点击同时把鼠标锁进画面，之后像 FPS 一样移动鼠标看四周。
    void Promise.resolve(document.querySelector('canvas')?.requestPointerLock?.()).catch(() => {})
    if (back) {
      useJourney.getState().setPhase('placed')
      void activate(device)
    }
  }

  return (
    <div className="fixed inset-0 overflow-hidden bg-[#fbe9d6]">
      {place === 'room' ? <Room device={device} /> : <ParkStage device={device} />}
      {entered && <Hud device={device} />}
      {!entered && <Gate device={device} back={back} onEnter={enter} />}
      <div className={`pointer-events-none absolute inset-0 z-20 flex items-center justify-center bg-[#fbe9d6] transition-opacity duration-500 ${fade ? 'opacity-100' : 'opacity-0'}`}>
        <span className="text-2xl font-semibold text-[#1d3557]">{fade}</span>
      </div>
    </div>
  )
}

/** 进门前的一次点击：浏览器要求有手势才能出声、才能打开麦克风。第一次来先选一只玩偶。 */
function Gate({ device, back, onEnter }: { device: VirtualDevice; back: boolean; onEnter: () => void }) {
  const brand = (
    <div className="flex items-baseline justify-center gap-2">
      <span className="text-4xl font-black text-[#1d3557]">{copy.brand.name}</span>
      <span className="text-2xl font-semibold text-[#2f6bff]">{copy.brand.en}</span>
    </div>
  )
  if (back) {
    return (
      <button type="button" onClick={onEnter} className="absolute inset-0 z-30 flex flex-col items-center justify-center gap-4 bg-[#fbe9d6]/92 backdrop-blur-sm">
        <div className="flex h-20 w-20 items-center justify-center rounded-3xl bg-[#2f6bff] text-5xl shadow-lg">🧸</div>
        {brand}
        <span className="text-sm text-slate-500">{copy.brand.slogan}</span>
        <span className="mt-4 rounded-full bg-[#2f6bff] px-6 py-2 text-lg font-medium text-white shadow">{copy.room.gateBack}</span>
      </button>
    )
  }
  return (
    <div className="absolute inset-0 z-30 flex items-center justify-center overflow-auto bg-[#fbe9d6]/95 p-4 backdrop-blur-sm">
      <div className="w-full max-w-xl rounded-3xl bg-white/90 p-5 text-center shadow-xl ring-1 ring-sky-100">
        {brand}
        <h2 className="mt-3 text-xl font-bold text-[#1d3557]">{copy.room.choose}</h2>
        <p className="mb-3 text-sm text-slate-500">{copy.room.chooseSub}</p>
        <Skins device={device} />
        <button type="button" onClick={onEnter} className="mt-4 rounded-full bg-[#2f6bff] px-6 py-2 text-lg font-medium text-white shadow">
          {copy.room.gate}
        </button>
      </div>
    </div>
  )
}
