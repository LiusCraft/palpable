import { DevicePanel } from './device-sim/DevicePanel'
import { useDevices } from './device-sim/devices'
import { copy } from './ui/copy'
import { Button } from './ui/components'

export default function App() {
  const { devices, add } = useDevices()
  return (
    <div className="mx-auto max-w-5xl p-6">
      <h1 className="mb-4 text-2xl font-bold">
        {copy.brand.name} <span className="font-medium text-slate-500">{copy.brand.en}</span>
      </h1>
      <div className="grid gap-4 md:grid-cols-2">
        {devices.map((d) => (
          <DevicePanel key={d.mac} device={d} />
        ))}
      </div>
      <Button className="mt-4" tone="ghost" onClick={() => add('rabbit')}>
        {copy.device.add}
      </Button>
    </div>
  )
}
