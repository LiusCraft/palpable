import type { ThreeEvent } from '@react-three/fiber'
import { useStore } from 'zustand'
import type { VirtualDevice } from '../device-sim/device'
import { Plush } from './Plush'

/** 把一台虚拟设备的状态接到玩偶上：灯色优先设备工具指定的颜色，其次空闲时显示所在频道的颜色。点一下默认是触摸唤醒。 */
export function DevicePlush({ device, position, scale, onTap }: { device: VirtualDevice; position?: [number, number, number]; scale?: number; onTap?: (e: ThreeEvent<MouseEvent>) => void }) {
  const s = useStore(device.store)
  const channelColor = s.channel && s.light === 'idle' ? s.channel.color : undefined
  return (
    <Plush
      skin={s.skin}
      light={s.light}
      color={s.lightOverride?.color ?? channelColor}
      speaking={s.speaking}
      shakeAt={s.shakeAt}
      onTap={onTap ?? (() => device.wake())}
      position={position}
      scale={scale}
    />
  )
}
