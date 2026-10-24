import { useState } from 'react'
import { sayToCompanion } from '../../map/demoTool'
import { Button } from '../../ui/components'

/**
 * 演示遥控：小伙伴暂时听不到（比如灵矽不可用）时，代替小朋友说出两个名字。
 * 服务端执行与 AI 调用相同的 self.profile.set_names，之后照常交棒给风格智能体。
 */
export function DemoNaming({ mac }: { mac: string }) {
  const [doll, setDoll] = useState('')
  const [kid, setKid] = useState('')
  const [note, setNote] = useState('')
  const submit = async () => {
    const r = await sayToCompanion(mac, 'self.profile.set_names', { doll_name: doll, kid_name: kid })
    setNote(r.text)
  }
  return (
    <div className="mt-3 rounded-xl bg-slate-50 p-3 text-sm ring-1 ring-slate-200">
      <div className="mb-2 text-xs text-slate-500">演示：替小朋友说出名字（只要小名，不要全名）</div>
      <div className="flex flex-wrap gap-2">
        <input value={doll} onChange={(e) => setDoll(e.target.value)} maxLength={12} placeholder="玩偶的名字，如 小熊" className="w-32 rounded-lg px-2 py-1 ring-1 ring-slate-200" />
        <input value={kid} onChange={(e) => setKid(e.target.value)} maxLength={12} placeholder="小朋友的小名，如 朵朵" className="w-36 rounded-lg px-2 py-1 ring-1 ring-slate-200" />
        <Button tone="ghost" disabled={!doll.trim() || !kid.trim()} onClick={() => void submit()}>
          说出名字
        </Button>
      </div>
      {note && <div className="mt-2 text-xs text-emerald-600">{note}</div>}
    </div>
  )
}
