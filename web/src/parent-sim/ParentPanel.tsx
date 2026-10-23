import { useCallback, useEffect, useState } from 'react'
import { copy } from '../ui/copy'
import { Button, Chip } from '../ui/components'
import { parentApi, type Digest, type Friend, type Group, type Kid } from './api'

// 家长端（Web 模拟）：好友与群聊、今日回顾、家长控制。对应效果图第 7 格。
// 只调 /api，所有规则（归属、确认、解除）都在服务端。

const SKIN_EMOJI: Record<string, string> = { bear: '🐻', rabbit: '🐰', dragon: '🐲', cat: '🐱' }
const POLL = 3000

type Tab = 'friends' | 'digest' | 'control'

export function ParentPanel() {
  const [kids, setKids] = useState<Kid[]>([])
  const [kid, setKid] = useState<string>('')
  const [friends, setFriends] = useState<Friend[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [digest, setDigest] = useState<Digest | null>(null)
  const [tab, setTab] = useState<Tab>('friends')
  const [error, setError] = useState('')

  // 默认看在线的孩子。
  const current = kid || kids.find((k) => k.online)?.id || kids[0]?.id || ''

  const refresh = useCallback(async () => {
    try {
      const ks = await parentApi.kids()
      setKids(ks)
      const id = kid || ks.find((k) => k.online)?.id || ks[0]?.id
      if (!id) return
      const [f, g, d] = await Promise.all([parentApi.friends(id), parentApi.groups(id), parentApi.digest(id)])
      setFriends(f)
      setGroups(g)
      setDigest(d)
      setError('')
    } catch (e) {
      setError(e instanceof Error && e.message === '404' ? '家长端未开放（服务端未配置 demo.parent_token）' : '连不上家长端')
    }
  }, [kid])

  useEffect(() => {
    const first = window.setTimeout(() => void refresh(), 0)
    const t = window.setInterval(() => void refresh(), POLL)
    return () => {
      window.clearTimeout(first)
      window.clearInterval(t)
    }
  }, [refresh])

  const act = async (fn: () => Promise<void>) => {
    await fn()
    await refresh()
  }

  const confirmed = friends.filter((f) => f.status === 'confirmed')
  const pending = friends.filter((f) => f.status === 'pending')
  const chats = digest?.chats.length ?? 0

  return (
    <div className="grid gap-4 lg:grid-cols-[180px_1fr_1fr_1fr]">
      {/* 侧栏 */}
      <aside className="rounded-2xl bg-white p-3 ring-1 ring-sky-100">
        <div className="mb-3 flex items-center gap-2 font-semibold text-[#1d3557]">
          <span className="flex h-8 w-8 items-center justify-center rounded-full bg-sky-100">👩</span>
          {copy.parent.title}
        </div>
        <nav className="space-y-1 text-sm">
          {(['friends', 'digest', 'control'] as Tab[]).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setTab(t)}
              className={`block w-full rounded-lg px-3 py-1.5 text-left ${tab === t ? 'bg-sky-50 font-medium text-[#2f6bff]' : 'text-slate-600 hover:bg-slate-50'}`}
            >
              {copy.parent.tabs[t]}
            </button>
          ))}
        </nav>
        <div className="mt-4 space-y-1">
          {kids.map((k) => (
            <button
              key={k.id}
              type="button"
              onClick={() => setKid(k.id)}
              className={`flex w-full items-center gap-2 rounded-lg px-2 py-1 text-left text-sm ${k.id === current ? 'bg-[#1d3557] text-white' : 'text-slate-600 hover:bg-slate-50'}`}
            >
              <span>{SKIN_EMOJI[k.skin] ?? '🧸'}</span>
              <span className="truncate">{k.kid_name ? `${k.kid_name}的${k.doll_name}` : '新玩偶'}</span>
              <span className={`ml-auto h-2 w-2 rounded-full ${k.online ? 'bg-emerald-400' : 'bg-slate-300'}`} />
            </button>
          ))}
          {kids.length === 0 && <p className="text-xs text-slate-400">{copy.parent.noKids}</p>}
        </div>
        {error && <p className="mt-3 text-xs text-rose-500">{error}</p>}
      </aside>

      {/* 好友列表 */}
      <section className="rounded-2xl bg-white p-4 ring-1 ring-sky-100">
        <h3 className="mb-3 font-semibold text-[#1d3557]">{copy.parent.friends}</h3>
        {pending.map((f) => (
          <div key={f.request_id} className="mb-2 flex items-center gap-2 rounded-xl bg-amber-50 px-3 py-2 text-sm">
            <span className="flex-1">
              🤝 {f.name || copy.friend.someone} <span className="text-xs text-amber-600">{copy.parent.pending}</span>
            </span>
            <Button onClick={() => void act(() => parentApi.confirm(f.request_id))}>{copy.parent.confirm}</Button>
          </div>
        ))}
        <ul className="space-y-2 text-sm">
          {confirmed.map((f) => (
            <li key={f.device_id} className="flex items-center gap-2">
              <span className="flex h-8 w-8 items-center justify-center rounded-full bg-pink-50">🐾</span>
              <span className="flex-1">
                <div className="font-medium">{f.name || copy.friend.someone}</div>
                <div className={`text-xs ${f.online ? 'text-emerald-600' : 'text-slate-400'}`}>● {f.online ? copy.parent.online : copy.parent.offline}</div>
              </span>
            </li>
          ))}
          {friends.length === 0 && <li className="text-xs text-slate-400">{copy.parent.noFriends}</li>}
        </ul>
        <h3 className="mb-2 mt-4 font-semibold text-[#1d3557]">{copy.parent.groups}</h3>
        <div className="flex flex-wrap gap-1">
          {groups.map((g) => (
            <Chip key={g.id} tone={g.foreground ? 'green' : 'gray'}>
              {g.name}
              {g.code && ` · ${g.code}`}
            </Chip>
          ))}
          {groups.length === 0 && <span className="text-xs text-slate-400">—</span>}
        </div>
      </section>

      {/* 今日回顾 */}
      <section className="rounded-2xl bg-white p-4 ring-1 ring-sky-100">
        <h3 className="mb-3 font-semibold text-[#1d3557]">{copy.parent.digest}</h3>
        <div className="grid grid-cols-3 gap-2 text-center">
          {[
            [copy.parent.chats, chats, copy.parent.unit.times],
            [copy.parent.met, digest?.met.length ?? 0, copy.parent.unit.people],
            [copy.parent.shared, digest?.shared.length ?? 0, copy.parent.unit.times],
          ].map(([label, n, unit]) => (
            <div key={label as string} className="rounded-xl bg-sky-50/60 py-2">
              <div className="text-xs text-slate-500">{label}</div>
              <div className="text-xl font-bold text-[#1d3557]">
                {n} <span className="text-xs font-normal text-slate-400">{unit}</span>
              </div>
            </div>
          ))}
        </div>
        {tab === 'digest' || tab === 'friends' ? (
          <ul className="mt-3 space-y-1 text-sm">
            {digest?.met.map((m) => (
              <li key={m.friend}>🤝 遇到了 {m.name || copy.friend.someone}</li>
            ))}
            {digest?.shared.map((s, i) => (
              <li key={i}>🌳 {s.name}分享：{s.text}</li>
            ))}
          </ul>
        ) : null}
      </section>

      {/* 家长控制 */}
      <section className="rounded-2xl bg-white p-4 ring-1 ring-sky-100">
        <h3 className="mb-3 font-semibold text-[#1d3557]">{copy.parent.tabs.control}</h3>
        {tab === 'control' ? (
          <ul className="space-y-2 text-sm">
            {confirmed.map((f) => (
              <li key={f.device_id} className="flex items-center justify-between">
                <span>{f.name || copy.friend.someone}</span>
                <Button tone="ghost" onClick={() => void act(() => parentApi.remove(current, f.device_id))}>
                  {copy.parent.remove}
                </Button>
              </li>
            ))}
            <li className="pt-2 text-xs text-slate-400">对话原话：</li>
            {digest?.chats.slice(-6).map((c, i) => (
              <li key={i} className={`text-xs ${c.role === 'user' ? 'text-slate-700' : 'text-sky-700'}`}>
                {c.role === 'user' ? '孩子' : '小伙伴'}：{c.text}
              </li>
            ))}
            {chats === 0 && <li className="text-xs text-slate-400">{copy.parent.noChats}</li>}
          </ul>
        ) : (
          <ul className="space-y-1 text-sm text-slate-600">
            {copy.parent.controls.map((c) => (
              <li key={c}>
                <button type="button" onClick={() => setTab('control')} className="flex w-full items-center justify-between rounded-lg px-2 py-2 hover:bg-slate-50">
                  {c}
                  <span className="text-slate-300">›</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
