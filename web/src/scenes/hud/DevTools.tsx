import { useState } from 'react'
import type { VirtualDevice } from '../../device-sim/device'
import { useStore } from 'zustand'
import { copy } from '../../ui/copy'
import { Extras } from '../extras/Extras'
import { Skins } from '../extras/Skins'
import { DemoNaming } from './DemoNaming'

/**
 * 演示遥控：正式体验全靠开口说话，这里只是在灵矽暂时听不到时代替小朋友说，
 * 以及展示"换皮套"这类需要硬件配合的功能。服务端执行与 AI 调用相同的业务工具。
 */
export function DevTools({ device }: { device: VirtualDevice }) {
  const [open, setOpen] = useState(false)
  const s = useStore(device.store)
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="rounded-full glass-pill px-3 py-1 text-sm text-slate-600 shadow ring-1 ring-slate-200 hover:bg-white"
        title="演示遥控"
      >
        🛠 演示遥控
      </button>
      {open && (
        <div className="absolute right-0 top-10 z-20 max-h-[80vh] w-[min(92vw,440px)] space-y-4 overflow-auto rounded-2xl bg-white p-4 text-sm shadow-xl ring-1 ring-sky-100">
          <section>
            <h3 className="font-semibold text-[#1d3557]">替小朋友起名字</h3>
            <DemoNaming mac={s.mac} />
          </section>
          <section>
            <h3 className="mb-2 font-semibold text-[#1d3557]">{copy.extras.title}</h3>
            <p className="mb-2 text-xs text-slate-500">{copy.extras.sub}</p>
            <Extras device={device} />
          </section>
          <section>
            <h3 className="font-semibold text-[#1d3557]">{copy.skins.title}</h3>
            <p className="mb-2 text-xs text-slate-500">{copy.skins.sub}</p>
            <Skins device={device} />
          </section>
        </div>
      )}
    </>
  )
}
