import { Canvas } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import { useDevice } from './device-sim/useDevice'
import { Plush } from './plush/Plush'

const LINK_TEXT = { offline: '未连接', connecting: '连接中…', online: '已上线' } as const

export default function App() {
  const { link, light, lastError, sessionId, micOn, connect, disconnect, wake, setMic } = useDevice()
  const deviceId = new URLSearchParams(location.search).get('device') ?? 'plush-001'

  return (
    <div className="relative h-full">
      <Canvas camera={{ position: [0, 0.6, 4], fov: 45 }}>
        <ambientLight intensity={0.8} />
        <directionalLight position={[3, 4, 5]} intensity={1.4} />
        <Plush light={light} onTap={wake} />
        <OrbitControls enablePan={false} />
      </Canvas>

      <div className="absolute left-4 top-4 rounded-2xl bg-white/80 p-4 text-sm shadow">
        <div className="font-bold">可感 Palpable · 设备 {deviceId}</div>
        <div>状态：{LINK_TEXT[link]}（灯：{light}）</div>
        {sessionId && <div className="text-xs text-gray-500">session {sessionId}</div>}
        {lastError && <div className="text-red-600">{lastError}</div>}
        <button
          className="mt-2 rounded-full bg-indigo-500 px-4 py-1 text-white"
          onClick={() => (link === 'offline' ? connect(deviceId) : disconnect())}
        >
          {link === 'offline' ? '连接网关' : '断开'}
        </button>
        {link === 'online' && (
          <button
            className="ml-2 mt-2 rounded-full bg-pink-500 px-4 py-1 text-white"
            onClick={() => void setMic(!micOn)}
          >
            {micOn ? '停止说话' : '开始说话'}
          </button>
        )}
      </div>
    </div>
  )
}
