import { useEffect, useState } from 'react'
import { useStore } from 'zustand'
import type { VirtualDevice } from '../../device-sim/device'
import { setSceneDescriber } from '../../device-sim/environment'
import { sayToCompanion } from '../../map/demoTool'
import { pairKey, useWorld } from '../../map/world'
import { copy } from '../../ui/copy'
import { Button, Chip } from '../../ui/components'
import { inside } from './geo'
import { ParkMap } from './ParkMap'
import { ParkWalk } from './ParkWalk'
import { useParkSensors, usePositions } from './sensors'

// 公园：孩子第一人称走在公园里。区域频道、陌生人屏蔽、贴近成为好友、好友互相听见都在这里发生。
// 这里的面板只是"画面上的提示"：规则由服务端裁决，设备端不做业务判断。

export function ParkStage({ device }: { device: VirtualDevice }) {
  useParkSensors()

  useEffect(() => {
    setSceneDescriber((mac) => {
      const p = usePositions.getState().pos[mac]
      const zone = useWorld.getState().zones[0]
      if (p && zone && inside(p, zone.polygon)) return '公园里绿绿的草地上有一条弯弯的小河，河边有柳树和长椅，远处有一棵很高很高的大树，还有小朋友在放风筝。'
      return '路边有几栋彩色的小房子和一排小树，前面就是公园的大门。'
    })
  }, [])

  return (
    <div className="absolute inset-0">
      <ParkWalk device={device} />
      <div className="pointer-events-none absolute bottom-3 right-3 w-44 overflow-hidden rounded-xl opacity-90 shadow-lg ring-2 ring-white/80">
        <div className="aspect-[860/660]">
          <ParkMap viewer={device.mac} />
        </div>
      </div>
      <ZonePrompt device={device} />
      <Toast device={device} />
      <Members device={device} />
    </div>
  )
}

/** 进入公园后，小伙伴会问要不要加入；演示时也可以点按钮代替孩子说"加入"。 */
function ZonePrompt({ device }: { device: VirtualDevice }) {
  const s = useStore(device.store)
  const pos = usePositions((p) => p.pos[s.mac])
  const zone = useWorld((w) => w.zones[0])
  const inPark = !!(pos && zone && inside(pos, zone.polygon))
  if (!inPark || !zone || s.channel || s.link !== 'online') return null
  // 每次走进公园都重新问一次：离开时卡片卸载，"暂不加入"随之清掉。
  return <PromptCard mac={s.mac} zoneName={zone.name} />
}

function PromptCard({ mac, zoneName }: { mac: string; zoneName: string }) {
  const [dismissed, setDismissed] = useState(false)
  const [note, setNote] = useState('')
  if (dismissed) return null

  const join = async () => {
    const r = await sayToCompanion(mac, 'self.zone.join')
    setNote(r.text)
  }
  return (
    <div className="absolute left-1/2 top-4 w-72 -translate-x-1/2 animate-[pop_0.35s_ease-out] rounded-2xl bg-white/95 p-4 text-center shadow-lg ring-1 ring-sky-100">
      <div className="text-2xl">📍</div>
      <div className="mt-1 font-semibold text-[#1d3557]">{copy.park.askTitle(zoneName + '区域')}</div>
      <div className="text-sm text-slate-500">{copy.park.askSub}</div>
      <div className="mt-1 text-xs text-slate-400">{copy.park.askHint}</div>
      {note && <div className="mt-2 text-xs text-rose-500">{note}</div>}
      <div className="mt-3 flex justify-center gap-2">
        <Button tone="ghost" onClick={() => setDismissed(true)}>
          {copy.park.later}
        </Button>
        <Button onClick={() => void join()}>{copy.park.join}</Button>
      </div>
    </div>
  )
}

function Toast({ device }: { device: VirtualDevice }) {
  const toast = useStore(device.store, (s) => s.toast)
  const [now, setNow] = useState(0)
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 500)
    return () => window.clearInterval(t)
  }, [])
  if (!toast || now - toast.at > 4000) return null
  return (
    <div className="pointer-events-none absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2">
      <div key={toast.at} className="flex animate-[pop_0.35s_ease-out] items-center gap-2 rounded-full bg-[#1d3557]/85 px-5 py-2.5 text-white shadow-lg">
        <span className="flex h-7 w-7 items-center justify-center rounded-full bg-white/20">🔗</span>
        {toast.text}
      </div>
    </div>
  )
}

/** 频道成员：好友来自服务端已过滤的 presence；陌生人只在地图上标"已屏蔽"，听不到也看不到彼此。 */
function Members({ device }: { device: VirtualDevice }) {
  const s = useStore(device.store)
  const world = useWorld()
  const zone = world.zones[0]
  const myPos = usePositions((p) => p.pos[s.mac])
  const nearby = Object.values(world.devices).filter(
    (d) => d.device !== s.mac && d.online && d.pos && zone && inside(d.pos, zone.polygon) && !s.present[d.device],
  )
  const myName = world.devices[s.mac]?.name

  return (
    <div className="absolute left-3 top-14 w-56 rounded-2xl glass p-3 text-sm">
      <div className="mb-2 flex items-baseline gap-1">
        <span className="font-semibold text-[#1d3557]">{copy.park.membersTitle}</span>
        <span className="text-xs text-slate-400">{copy.park.membersSub}</span>
      </div>
      {s.channel ? (
        <Chip tone="green">
          <span className="inline-block h-2 w-2 rounded-full" style={{ background: s.channel.color }} />
          {s.channel.name}
        </Chip>
      ) : (
        <Chip tone="gray">{myPos && zone && inside(myPos, zone.polygon) ? '还没加入频道' : '不在任何频道'}</Chip>
      )}
      <ul className="mt-3 space-y-2 text-sm">
        <li className="flex items-center justify-between">
          <span>🧸 {myName || '我的玩偶'}</span>
          <span className="text-xs text-sky-600">{copy.park.you}</span>
        </li>
        {Object.entries(s.present).map(([id, p]) => (
          <li key={id} className="flex items-center justify-between">
            <span>🐾 {p.name}</span>
            <span className="text-xs text-emerald-600">{copy.park.joined}</span>
          </li>
        ))}
        {nearby.map((d) => {
          const rel = world.friends[pairKey(s.mac, d.device)]
          return (
            <li key={d.device} className="flex items-center justify-between">
              <span className="text-slate-500">🐾 {d.name || '玩偶'}</span>
              <span className={`text-xs ${rel === 'confirmed' ? 'text-slate-400' : rel === 'pending' ? 'text-amber-500' : 'text-rose-500'}`}>
                {rel === 'confirmed' ? '未加入' : rel === 'pending' ? copy.park.pending : copy.park.blocked}
              </span>
            </li>
          )
        })}
      </ul>
      {Object.values(s.friends).some((f) => f.status === 'pending') && (
        <p className="mt-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">有新朋友在等爸爸妈妈确认</p>
      )}
    </div>
  )
}
