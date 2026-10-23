import { ContactShadows } from '@react-three/drei'
import { Canvas } from '@react-three/fiber'
import { useStore } from 'zustand'
import type { VirtualDevice } from '../../device-sim/device'
import { useDevices } from '../../device-sim/devices'
import { Plush } from '../../plush/Plush'
import { copy } from '../../ui/copy'

// 设备皮套（不同风格）：同一个核心，换一个皮套，换一种个性。对应效果图第 8 格。
// 点一款给当前玩偶换上：设备重新上线并上报新皮套，服务端切换到对应风格的小伙伴。

export function Skins({ device }: { device: VirtualDevice }) {
  const skin = useStore(device.store, (s) => s.skin)
  const changeSkin = useDevices((s) => s.changeSkin)
  const list = copy.skins.list
  return (
    <div>
      <div className="h-56 rounded-2xl bg-gradient-to-b from-sky-50 to-white">
        <Canvas camera={{ position: [0, 0.3, 7.2], fov: 40 }}>
          <ambientLight intensity={0.85} />
          <directionalLight position={[3, 4, 5]} intensity={1.4} />
          {list.map((s, i) => (
            <Plush
              key={s.skin}
              skin={s.skin}
              light={s.skin === skin ? 'listening' : 'idle'}
              position={[(i - (list.length - 1) / 2) * 2.1, 0.1, 0]}
              scale={0.8}
              onTap={() => changeSkin(device.mac, s.skin)}
            />
          ))}
          <ContactShadows position={[0, -1.05, 0]} opacity={0.3} scale={12} blur={2.4} />
        </Canvas>
      </div>
      <div className="mt-2 grid grid-cols-4 gap-2 text-center">
        {list.map((s) => (
          <button
            key={s.skin}
            type="button"
            onClick={() => changeSkin(device.mac, s.skin)}
            className={`rounded-xl px-2 py-2 ${s.skin === skin ? 'bg-[#2f6bff] text-white' : 'bg-white text-slate-600 ring-1 ring-slate-200'}`}
          >
            <div className="text-sm font-semibold">{s.name}</div>
            <div className={`text-xs ${s.skin === skin ? 'text-white/80' : 'text-slate-400'}`}>{s.desc}</div>
          </button>
        ))}
      </div>
    </div>
  )
}
