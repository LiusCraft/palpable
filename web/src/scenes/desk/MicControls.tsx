import { useStore } from 'zustand'
import type { VirtualDevice } from '../../device-sim/device'
import { startMic, useDevices } from '../../device-sim/devices'
import { copy } from '../../ui/copy'
import { Button, Wave } from '../../ui/components'

/** 麦克风控制：打开后免按键（设备端 VAD），也可按住说话；麦克风同一时刻只在一只玩偶上。 */
export function MicControls({ device }: { device: VirtualDevice }) {
  const s = useStore(device.store)
  const { active, setActive } = useDevices()
  const mine = active === s.mac
  const online = s.link === 'online'

  const toggle = async () => {
    if (!s.micEnabled) {
      try {
        await startMic()
      } catch (e) {
        device.store.setState({ lastError: copy.device.errorMic(e instanceof Error ? e.message : String(e)) })
        return
      }
      setActive(s.mac)
    }
    device.setMicEnabled(!s.micEnabled)
  }

  const hold = async (on: boolean) => {
    if (on) {
      try {
        await startMic()
      } catch {
        return
      }
      setActive(s.mac)
    }
    device.setPushToTalk(on)
  }

  return (
    <div className="mt-3 flex flex-wrap items-center gap-2">
      <Button tone={s.micEnabled ? 'ghost' : 'pink'} disabled={!online} onClick={() => void toggle()}>
        {s.micEnabled ? copy.desk.micStop : copy.desk.micOff}
      </Button>
      <button
        type="button"
        disabled={!online}
        onPointerDown={() => void hold(true)}
        onPointerUp={() => void hold(false)}
        onPointerLeave={() => s.talking && void hold(false)}
        className="rounded-full bg-sky-50 px-4 py-1.5 text-sm text-sky-700 ring-1 ring-sky-200 disabled:opacity-40"
      >
        按住说话
      </button>
      {s.micEnabled && mine && (
        <span className="inline-flex items-center gap-1 text-xs text-slate-500">
          <Wave active={s.talking} tone="pink" />
          {s.talking ? copy.desk.micOn : ''}
        </span>
      )}
      {s.micEnabled && !mine && (
        <Button tone="ghost" onClick={() => setActive(s.mac)}>
          {copy.device.useMic}
        </Button>
      )}
    </div>
  )
}
