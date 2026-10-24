import { pairKey, useWorld, type LatLng } from '../../map/world'
import { copy } from '../../ui/copy'
import { MAP_H, MAP_W, toXY } from './geo'
import { usePositions } from './sensors'

// 公园小地图（第一人称右下角的那张）：草地、小河、树与小路是装饰；区域轮廓与所有玩偶的位置来自 /world。
// 只读，走动在 3D 里进行。

const SKIN_FACE: Record<string, { emoji: string; color: string }> = {
  bear: { emoji: '🐻', color: '#c89a6a' },
  rabbit: { emoji: '🐰', color: '#fbe3ea' },
  dragon: { emoji: '🐲', color: '#a8dc8f' },
  cat: { emoji: '🐱', color: '#cfd4dc' },
}

const TREES: [number, number, number][] = [
  [210, 150, 16], [250, 130, 20], [300, 160, 14], [560, 140, 18], [610, 170, 15], [660, 140, 20],
  [700, 230, 16], [230, 420, 18], [270, 460, 15], [640, 420, 17], [690, 460, 21], [600, 500, 14],
  [420, 120, 13], [480, 500, 16], [190, 300, 14], [720, 330, 15],
  [60, 120, 12], [90, 560, 14], [800, 100, 13], [790, 560, 15],
]

const HOUSES: [number, number, string][] = [
  [40, 70, '#f6c48f'], [100, 50, '#9ec5f8'], [780, 40, '#f4a6b8'], [40, 600, '#b9e3a5'], [800, 610, '#f6c48f'],
]

function Tree({ x, y, r }: { x: number; y: number; r: number }) {
  return (
    <g>
      <ellipse cx={x + 3} cy={y + r * 0.9} rx={r * 0.9} ry={r * 0.35} fill="#000" opacity={0.08} />
      <rect x={x - 2} y={y} width={4} height={r * 0.8} fill="#9b7653" rx={1} />
      <circle cx={x} cy={y - r * 0.2} r={r} fill="#5fb85a" />
      <circle cx={x - r * 0.35} cy={y - r * 0.45} r={r * 0.55} fill="#7ccf6b" />
    </g>
  )
}

export function ParkMap({ viewer }: { viewer: string }) {
  const world = useWorld()
  const pos = usePositions((p) => p.pos)

  // 自己的位置以本页为准（比 /world 的推送新）；别人的按 /world。
  const avatars = Object.values(world.devices)
    .filter((d) => d.online || d.device === viewer)
    .map((d) => ({ ...d, at: d.device === viewer ? (pos[viewer] ?? d.pos) : d.pos }))
    .filter((d): d is typeof d & { at: LatLng } => !!d.at)

  const zone = world.zones[0]
  const zonePath = zone?.polygon.map((p, i) => `${i ? 'L' : 'M'}${toXY(p).x},${toXY(p).y}`).join(' ') + ' Z'
  const zoneTop = zone ? toXY(zone.polygon[0]) : null

  return (
    <svg viewBox={`0 0 ${MAP_W} ${MAP_H}`} className="h-full w-full select-none" data-testid="park-map" aria-label="公园地图">
      <defs>
        <linearGradient id="land" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#eef6e4" />
          <stop offset="1" stopColor="#e3efd6" />
        </linearGradient>
        <linearGradient id="grass" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#bfe6a6" />
          <stop offset="1" stopColor="#a6d98c" />
        </linearGradient>
        <filter id="soft" x="-20%" y="-20%" width="140%" height="140%">
          <feDropShadow dx="0" dy="2" stdDeviation="2" floodOpacity="0.18" />
        </filter>
      </defs>

      <rect width={MAP_W} height={MAP_H} fill="url(#land)" />
      {/* 外围道路与房子 */}
      <path d={`M0 330 H${MAP_W}`} stroke="#f7efe2" strokeWidth={26} />
      <path d={`M430 0 V${MAP_H}`} stroke="#f7efe2" strokeWidth={18} />
      {HOUSES.map(([x, y, c], i) => (
        <g key={i} filter="url(#soft)">
          <rect x={x - 16} y={y - 6} width={32} height={22} fill="#fffaf2" rx={2} />
          <path d={`M${x - 20} ${y - 4} L${x} ${y - 20} L${x + 20} ${y - 4} Z`} fill={c} />
        </g>
      ))}

      {/* 公园区域 */}
      {zone && <path d={zonePath} fill="url(#grass)" stroke="#7cc06a" strokeWidth={3} strokeDasharray="10 6" />}
      {/* 小河与池塘 */}
      <path d="M150 560 C 260 470, 330 520, 420 420 S 560 260, 720 180" stroke="#8fd3f4" strokeWidth={26} fill="none" strokeLinecap="round" />
      <path d="M150 560 C 260 470, 330 520, 420 420 S 560 260, 720 180" stroke="#c3ecfb" strokeWidth={8} fill="none" strokeLinecap="round" opacity={0.7} />
      <ellipse cx={360} cy={230} rx={46} ry={26} fill="#9fdcf6" />
      {/* 园路 */}
      <path d="M40 330 C 160 330, 220 300, 330 330 S 520 380, 640 300" stroke="#f3e3c3" strokeWidth={10} fill="none" strokeLinecap="round" />
      {TREES.map(([x, y, r], i) => (
        <Tree key={i} x={x} y={y} r={r} />
      ))}
      {zone && zoneTop && (
        <g transform={`translate(${toXY(zone.polygon[1]).x / 2 + zoneTop.x / 2}, ${zoneTop.y + 26})`}>
          <rect x={-46} y={-14} width={92} height={28} rx={14} fill="#3bb273" filter="url(#soft)" />
          <text textAnchor="middle" dy={5} fontSize={14} fill="#fff" fontWeight={700}>
            {copy.park.title}
          </text>
        </g>
      )}

      {/* 玩偶 */}
      {avatars.map((d) => {
        const { x, y } = toXY(d.at)
        const face = SKIN_FACE[d.skin] ?? SKIN_FACE.bear
        const mine = d.device === viewer
        const rel = world.friends[pairKey(viewer, d.device)]
        return (
          <g key={d.device} transform={`translate(${x}, ${y})`}>
            {d.speaking && (
              <circle r={30} fill="none" stroke="#3ad07a" strokeWidth={4} opacity={0.8}>
                <animate attributeName="r" values="26;36;26" dur="1.1s" repeatCount="indefinite" />
                <animate attributeName="opacity" values="0.9;0.2;0.9" dur="1.1s" repeatCount="indefinite" />
              </circle>
            )}
            <circle r={24} fill={face.color} stroke={mine ? '#2f6bff' : '#ffffff'} strokeWidth={4} filter="url(#soft)" />
            <text textAnchor="middle" dy={8} fontSize={24}>
              {face.emoji}
            </text>
            <g transform="translate(0, 40)">
              <rect x={-34} y={-11} width={68} height={22} rx={11} fill="#ffffff" opacity={0.92} />
              <text textAnchor="middle" dy={5} fontSize={12} fill="#1d3557" fontWeight={600}>
                {mine ? '我' : d.name || '玩偶'}
              </text>
            </g>
            {!mine && (
              <g transform="translate(22, -22)">
                <rect x={-4} y={-11} width={rel === 'confirmed' ? 36 : 50} height={20} rx={10} fill={rel === 'confirmed' ? '#3bb273' : rel === 'pending' ? '#f5a524' : '#f0566b'} />
                <text x={rel === 'confirmed' ? 14 : 21} dy={4} textAnchor="middle" fontSize={11} fill="#fff" fontWeight={700}>
                  {rel === 'confirmed' ? '好友' : rel === 'pending' ? '待确认' : copy.park.blocked}
                </text>
              </g>
            )}
          </g>
        )
      })}
    </svg>
  )
}
