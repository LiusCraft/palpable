import { Canvas } from '@react-three/fiber'
import { useStore } from 'zustand'
import { Plush } from '../plush/Plush'
import { copy } from '../ui/copy'
import { Button, Card, Chip } from '../ui/components'
import type { VirtualDevice } from './device'
import { startMic, useDevices } from './devices'

/** 一只玩偶的控制面板：连接、麦克风、按住说话、点一下唤醒。 */
export function DevicePanel({ device }: { device: VirtualDevice }) {
  const s = useStore(device.store)
  const { active, setActive } = useDevices()
  const isActive = active === s.mac

  const toggleMic = async () => {
    if (!s.micEnabled) {
      try {
        await startMic()
      } catch (e) {
        device.setMicEnabled(false)
        alert(copy.device.errorMic(e instanceof Error ? e.message : String(e)))
        return
      }
      setActive(s.mac)
    }
    device.setMicEnabled(!s.micEnabled)
  }

  return (
    <Card>
      <div className="flex items-center justify-between">
        <div className="text-sm text-slate-500">{s.mac}</div>
        <Chip tone={s.link === 'online' ? 'green' : 'gray'}>{copy.link[s.link]}</Chip>
      </div>
      <div className="h-56">
        <Canvas camera={{ position: [0, 0.6, 4], fov: 45 }}>
          <ambientLight intensity={0.8} />
          <directionalLight position={[3, 4, 5]} intensity={1.4} />
          <Plush light={s.light} onTap={() => device.wake()} />
        </Canvas>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Chip>{copy.light[s.light]}</Chip>
        {s.channel && <Chip tone="green">{s.channel.name}</Chip>}
        {s.peer && <Chip tone="pink">{s.peer.name}</Chip>}
        {isActive && <Chip tone="blue">{copy.device.active}</Chip>}
      </div>
      {s.toast && <p className="mt-2 text-sm text-emerald-600">{s.toast.text}</p>}
      {s.lastError && <p className="mt-2 text-sm text-rose-600">{s.lastError}</p>}
      <div className="mt-3 flex flex-wrap gap-2">
        <Button onClick={() => (s.link === 'offline' ? device.connect() : device.disconnect())}>{s.link === 'offline' ? '连接' : '断开'}</Button>
        {s.link === 'online' && (
          <>
            <Button tone="pink" onClick={() => void toggleMic()}>
              {s.micEnabled ? copy.desk.micStop : copy.desk.micOff}
            </Button>
            {!isActive && (
              <Button tone="ghost" onClick={() => setActive(s.mac)}>
                {copy.device.useMic}
              </Button>
            )}
          </>
        )}
      </div>
    </Card>
  )
}
