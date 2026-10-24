import { useStore } from 'zustand'
import { startMic } from '../../device-sim/client'
import type { VirtualDevice } from '../../device-sim/device'
import { useWorld } from '../../map/world'
import { LIGHT_COLORS } from '../../plush/looks'
import { copy } from '../../ui/copy'
import { Chip, Wave } from '../../ui/components'
import { useJourney } from '../journey'
import { DevTools } from './DevTools'

/** 画面四周的提示。设备没有屏幕，这些只是给"站在房间里的孩子"的旁白，不是设备的一部分。 */
export function Hud({ device }: { device: VirtualDevice }) {
  const s = useStore(device.store)
  const { place, phase, prompt, locked } = useJourney()
  const named = useWorld((w) => !!w.devices[s.mac]?.name)

  const base =
    place === 'park'
      ? copy.park.walkHint
      : phase === 'boxed'
        ? copy.room.boxed
        : phase === 'open'
          ? copy.room.open
          : phase === 'held'
            ? copy.room.held
            : named
              ? copy.room.placedNamed
              : copy.room.placedUnnamed
  const needMic = !s.micEnabled && (place === 'park' || phase === 'placed')
  const retryMic = async () => {
    try {
      await startMic()
      device.setMicEnabled(true)
    } catch (e) {
      device.store.setState({ lastError: copy.device.errorMic(e instanceof Error ? e.message : String(e)) })
    }
  }

  return (
    <>
      <div className="pointer-events-none absolute left-3 top-3 flex flex-wrap items-center gap-2">
        {phase === 'placed' || place === 'park' ? (
          <>
            <Chip tone={s.link === 'online' ? 'green' : 'gray'}>{copy.link[s.link]}</Chip>
            <Chip>
              <span className="inline-block h-2 w-2 rounded-full" style={{ background: s.lightOverride?.color ?? LIGHT_COLORS[s.light] }} />
              {copy.light[s.light]}
            </Chip>
          </>
        ) : null}
        {s.micEnabled && (
          <Chip tone="pink">
            <Wave active={s.talking} tone="pink" />
            {s.talking ? '我在说…' : copy.room.listening}
          </Chip>
        )}
      </div>

      {!locked && (
        <p className="pointer-events-none absolute inset-x-0 top-14 mx-auto w-fit rounded-full bg-[#1d3557]/80 px-4 py-1.5 text-sm text-white shadow">{copy.room.lockHint}</p>
      )}

      <div className="absolute right-3 top-3 flex items-start gap-2">
        <a href="#/parent" target="_blank" rel="noopener noreferrer" className="rounded-full glass-pill px-3 py-1 text-sm text-slate-600 shadow ring-1 ring-slate-200 hover:bg-white">
          👩 家长端
        </a>
        <div className="relative">
          <DevTools device={device} />
        </div>
      </div>

      <div className="pointer-events-none absolute inset-x-0 bottom-4 flex flex-col items-center gap-2 px-4">
        {s.lastError && (
          <p className="pointer-events-auto flex items-center gap-2 rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600 shadow ring-1 ring-rose-100">
            {s.lastError}
            {needMic && (
              <button type="button" onClick={() => void retryMic()} className="rounded-full bg-rose-500 px-2 py-0.5 text-white">
                {copy.room.micRetry}
              </button>
            )}
          </p>
        )}
        {prompt && (
          <p className={`flex items-center gap-2 rounded-full px-4 py-2 text-base font-medium shadow-lg ${prompt.enabled ? 'bg-[#2f6bff] text-white' : 'bg-white/90 text-slate-500'}`}>
            {prompt.enabled && <kbd className="rounded-md bg-white px-2 py-0.5 text-sm font-bold text-[#2f6bff] shadow-inner">E</kbd>}
            {prompt.label}
          </p>
        )}
        <p className="rounded-full glass-pill px-4 py-1.5 text-sm text-slate-600 shadow">{base}</p>
      </div>
    </>
  )
}
