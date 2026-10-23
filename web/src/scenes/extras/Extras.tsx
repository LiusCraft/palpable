import { useState } from 'react'
import { useStore } from 'zustand'
import type { VirtualDevice } from '../../device-sim/device'
import { sayToCompanion } from '../../map/demoTool'
import { copy } from '../../ui/copy'
import { Button } from '../../ui/components'

// 其他功能（示例）：群聊、切换群、画面分享。对应效果图第 6 格。
// 孩子对小伙伴说出来即可；演示时按钮代替孩子说，服务端执行同样的业务工具。

const ACTIONS = [
  { tool: 'self.group.create', args: { ttl: 'today' }, label: '创建群聊' },
  { tool: 'self.group.switch', args: { target: 'next' }, label: '下一个群' },
  { tool: 'self.share.publish', args: { description: '一棵很高很高的大树，树下有小朋友在放风筝' }, label: '分享眼前的画面' },
] as const

export function Extras({ device }: { device: VirtualDevice }) {
  const s = useStore(device.store)
  const [notes, setNotes] = useState<Record<string, string>>({})
  const run = async (tool: string, args: Record<string, unknown>) => {
    const r = await sayToCompanion(s.mac, tool, args)
    setNotes((n) => ({ ...n, [tool]: r.text }))
  }
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      {copy.extras.items.map((item, i) => {
        const a = ACTIONS[i]
        return (
          <div key={item.title} className="flex flex-col rounded-2xl bg-white p-4 ring-1 ring-sky-100">
            <div className="font-semibold text-[#1d3557]">{item.title}</div>
            <div className="mt-1 text-sm text-slate-500">
              对小伙伴说 <span className="font-medium text-[#2f6bff]">{item.say}</span>
            </div>
            <div className="mt-1 text-xs text-slate-400">{item.note}</div>
            {notes[a.tool] && <div className="mt-2 rounded-lg bg-sky-50 px-2 py-1.5 text-xs text-sky-800">{notes[a.tool]}</div>}
            <Button className="mt-auto self-start" tone="ghost" disabled={s.link !== 'online'} onClick={() => void run(a.tool, a.args)}>
              {a.label}
            </Button>
          </div>
        )
      })}
    </div>
  )
}
