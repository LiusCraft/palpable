import { copy } from '../ui/copy'

interface FluffProps {
  cx: number
  cy: number
  rx: number
  ry: number
  fill: string
  fur?: string
  rotate?: number
}

// 固定的纤维纹理让首屏不依赖 WebGL，也不会在重渲染时随机闪动。
function fiberPath(rx: number, ry: number): string {
  const count = Math.min(1000, Math.round(rx * ry / 7))
  const paths: string[] = []
  for (let index = 0; index < count; index++) {
    const angle = index * 2.399963
    const radius = Math.sqrt((index + 0.5) / count) * 0.99
    const x = Math.cos(angle) * radius * rx
    const y = Math.sin(angle) * radius * ry
    const length = 1.2 + index % 5 * 0.45
    paths.push(`M${x.toFixed(1)} ${y.toFixed(1)}q${(-length * 0.7).toFixed(1)} ${(-length).toFixed(1)} .5 ${(-length * 1.5).toFixed(1)}`)
  }
  return paths.join('')
}

function Fluff({ cx, cy, rx, ry, fill, fur = '#fff8eb', rotate = 0 }: FluffProps) {
  return (
    <g transform={`translate(${cx} ${cy}) rotate(${rotate})`}>
      <ellipse rx={rx} ry={ry} fill={fill} filter="url(#companion-fur)" />
      <path d={fiberPath(rx, ry)} fill="none" stroke={fur} strokeWidth=".85" strokeLinecap="round" opacity=".31" />
    </g>
  )
}

function Eye({ x, y, size = 10 }: { x: number; y: number; size?: number }) {
  return <g><ellipse cx={x} cy={y} rx={size} ry={size * 1.22} fill="url(#companion-eye)" /><ellipse cx={x - size * .25} cy={y - size * .4} rx={size * .28} ry={size * .33} fill="#fff" opacity=".9" /><circle cx={x + size * .35} cy={y + size * .45} r={size * .12} fill="#c6ac99" opacity=".6" /></g>
}

function Smile({ x, y, scale = 1 }: { x: number; y: number; scale?: number }) {
  return <g transform={`translate(${x} ${y}) scale(${scale})`} fill="none" stroke="#5a3b30" strokeWidth="2.3" strokeLinecap="round"><path d="M0 0v12m-17-1c1 10 12 11 17 1 5 10 16 9 18-1" /><path d="M-14-5C-16-16 15-16 14-5 10 8-8 8-14-5Z" fill="url(#companion-nose)" stroke="none" /><path d="M-7-9h11" stroke="#fff" strokeWidth="1.7" opacity=".48" /></g>
}

function Collar({ x, y, width, color, scale = 1 }: { x: number; y: number; width: number; color: string; scale?: number }) {
  const curve = `M${-width / 2} -7Q0 18 ${width / 2} -7`
  return (
    <g transform={`translate(${x} ${y}) scale(${scale})`}>
      <path d={curve} fill="none" stroke="#3c3330" strokeWidth="8" opacity=".18" />
      <g className="companion-pendant-glow" fill={color} stroke={color} filter="url(#companion-glow)"><path d={curve} fill="none" strokeWidth="14" /><circle cy="31" r="20" stroke="none" /></g>
      <path d={curve} fill="none" stroke={color} strokeWidth="6" strokeLinecap="round" />
      <path d={curve} fill="none" stroke="#edffff" strokeWidth="2.5" strokeLinecap="round" />
      <path d="M0 9v8" stroke="#fff" strokeWidth="3" />
      <circle cy="31" r="15" fill={color} stroke="#dfffff" strokeWidth="2" />
      <g transform="translate(0 31)" fill="#fff"><circle cx="-6" cy="-6" r="3" /><circle cx="6" cy="-6" r="3" /><path d="M-8 0a8 8 0 0 1 16 0v2a8 7 0 0 1-16 0Z" /><circle cx="-3" r="1.3" fill={color} /><circle cx="3" r="1.3" fill={color} /><ellipse cy="4" rx="2" ry="1.5" fill={color} /></g>
    </g>
  )
}

function Rabbit() {
  return (
    <g>
      <Fluff cx={158} cy={479} rx={86} ry={88} fill="url(#companion-ivory)" />
      <Fluff cx={80} cy={487} rx={30} ry={62} fill="url(#companion-ivory)" rotate={19} />
      <Fluff cx={229} cy={480} rx={29} ry={62} fill="url(#companion-ivory)" rotate={-18} />
      <Fluff cx={109} cy={550} rx={43} ry={31} fill="url(#companion-ivory)" rotate={-8} />
      <Fluff cx={193} cy={551} rx={43} ry={31} fill="url(#companion-ivory)" rotate={8} />
      <g transform="rotate(-25 125 278)"><Fluff cx={111} cy={228} rx={34} ry={91} fill="url(#companion-ivory)" /><Fluff cx={111} cy={224} rx={19} ry={66} fill="url(#companion-pink)" fur="#ffe6e8" /></g>
      <g transform="rotate(-12 178 279)"><Fluff cx={180} cy={224} rx={33} ry={89} fill="url(#companion-ivory)" /><Fluff cx={180} cy={222} rx={18} ry={65} fill="url(#companion-pink)" fur="#ffe6e8" /></g>
      <Fluff cx={155} cy={352} rx={103} ry={92} fill="url(#companion-ivory)" rotate={-7} />
      <ellipse cx="99" cy="383" rx="20" ry="14" fill="url(#companion-blush)" transform="rotate(-8 99 383)" />
      <ellipse cx="215" cy="369" rx="19" ry="14" fill="url(#companion-blush)" />
      <Fluff cx={160} cy={385} rx={44} ry={33} fill="url(#companion-cream)" />
      <Eye x={113} y={351} size={9} /><Eye x={193} y={341} size={9} />
      <g transform="rotate(-7 161 375)"><Smile x={161} y={375} scale={.72} /></g>
      <Collar x={159} y={436} width={157} color="#ff74cb" scale={.95} />
    </g>
  )
}

function Bear() {
  return (
    <g>
      <Fluff cx={412} cy={440} rx={137} ry={132} fill="url(#companion-honey)" fur="#fff0cf" />
      <Fluff cx={411} cy={459} rx={88} ry={97} fill="url(#companion-belly)" fur="#ffedca" />
      <Fluff cx={294} cy={449} rx={42} ry={94} fill="url(#companion-honey)" fur="#ffedcc" rotate={15} />
      <Fluff cx={532} cy={449} rx={41} ry={91} fill="url(#companion-honey)" fur="#ffedcc" rotate={-15} />
      <Fluff cx={340} cy={548} rx={65} ry={43} fill="url(#companion-honey)" fur="#fff1cc" rotate={-8} />
      <Fluff cx={485} cy={548} rx={65} ry={43} fill="url(#companion-honey)" fur="#fff1cc" rotate={8} />
      <Fluff cx={316} cy={143} rx={49} ry={52} fill="url(#companion-honey)" fur="#fff0d7" rotate={-17} />
      <Fluff cx={507} cy={142} rx={49} ry={52} fill="url(#companion-honey)" fur="#fff0d7" rotate={17} />
      <Fluff cx={318} cy={147} rx={29} ry={33} fill="url(#companion-inner-ear)" fur="#eec9a0" />
      <Fluff cx={505} cy={146} rx={29} ry={33} fill="url(#companion-inner-ear)" fur="#eec9a0" />
      <Fluff cx={411} cy={231} rx={142} ry={125} fill="url(#companion-honey)" fur="#fff0cf" />
      <ellipse cx="330" cy="281" rx="26" ry="19" fill="url(#companion-blush)" transform="rotate(8 330 281)" />
      <ellipse cx="493" cy="281" rx="26" ry="19" fill="url(#companion-blush)" transform="rotate(-8 493 281)" />
      <Fluff cx={412} cy={282} rx={66} ry={48} fill="url(#companion-cream)" />
      <Eye x={355} y={244} size={12} /><Eye x={469} y={244} size={12} />
      <Smile x={412} y={271} scale={1.12} />
      <Collar x={412} y={351} width={224} color="#40b9ff" />
      <path d="m331 552 3 15m9-17 2 15m130-15-2 15m10-13-1 14" stroke="#986139" strokeWidth="1.5" strokeLinecap="round" opacity=".3" />
    </g>
  )
}

function Dragon() {
  return (
    <g>
      <path d="M782 358c61-2 61 43 53 77 30-6 47 11 27 29-10 10-33 12-59 11Z" fill="url(#companion-green)" filter="url(#companion-fur)" />
      <path d="m806 360 21-8c8-3 17 18 12 24l-16 10m10 20 23-3c10-1 17 19 11 25l-26 8m0 11 25 9-9 17-26-2" fill="url(#companion-orange)" filter="url(#companion-fur)" />
      <Fluff cx={709} cy={477} rx={106} ry={98} fill="url(#companion-green)" fur="#e5f8bc" />
      <Fluff cx={710} cy={495} rx={66} ry={69} fill="url(#companion-lime)" fur="#f5ffcf" />
      <Fluff cx={610} cy={485} rx={30} ry={64} fill="url(#companion-green)" fur="#dbf4ad" rotate={18} />
      <Fluff cx={809} cy={486} rx={29} ry={62} fill="url(#companion-green)" fur="#dbf4ad" rotate={-18} />
      <Fluff cx={651} cy={557} rx={48} ry={34} fill="url(#companion-green)" fur="#e7f5bd" rotate={-10} />
      <Fluff cx={768} cy={557} rx={48} ry={34} fill="url(#companion-green)" fur="#e7f5bd" rotate={10} />
      <path d="M690 259c5-47 29-48 35-2m20 4c12-43 35-29 32 13m16 15c28-29 41-3 26 20" fill="url(#companion-orange)" filter="url(#companion-fur)" />
      <Fluff cx={708} cy={359} rx={118} ry={111} fill="url(#companion-green)" fur="#e8f5b7" rotate={-3} />
      <ellipse cx="707" cy="391" rx="91" ry="75" fill="#518a39" opacity=".45" />
      <Fluff cx={706} cy={389} rx={84} ry={71} fill="url(#companion-cream)" />
      <Fluff cx={708} cy={315} rx={104} ry={44} fill="url(#companion-green)" fur="#e7f5b4" rotate={-3} />
      <Eye x={657} y={304} size={8} /><Eye x={757} y={299} size={8} />
      <ellipse cx="690" cy="333" rx="4" ry="3" fill="#4f7134" /><ellipse cx="719" cy="331" rx="4" ry="3" fill="#4f7134" />
      <ellipse cx="652" cy="409" rx="20" ry="14" fill="url(#companion-blush)" /><ellipse cx="761" cy="409" rx="20" ry="14" fill="url(#companion-blush)" />
      <Eye x={676} y={385} size={8} /><Eye x={736} y={385} size={8} />
      <Smile x={706} y={406} scale={.69} />
      <Collar x={709} y={465} width={178} color="#58ffd4" scale={.93} />
    </g>
  )
}

function Panda() {
  return (
    <g>
      <Fluff cx={937} cy={511} rx={83} ry={75} fill="url(#companion-ivory)" />
      <Fluff cx={861} cy={514} rx={27} ry={54} fill="url(#companion-black)" fur="#b3b1b4" rotate={20} />
      <Fluff cx={1011} cy={516} rx={26} ry={52} fill="url(#companion-black)" fur="#b3b1b4" rotate={-22} />
      <Fluff cx={895} cy={564} rx={39} ry={29} fill="url(#companion-black)" fur="#b3b1b4" rotate={-9} />
      <Fluff cx={985} cy={564} rx={39} ry={29} fill="url(#companion-black)" fur="#b3b1b4" rotate={9} />
      <Fluff cx={871} cy={352} rx={30} ry={34} fill="url(#companion-black)" fur="#aeacaf" rotate={-20} />
      <Fluff cx={1007} cy={354} rx={30} ry={34} fill="url(#companion-black)" fur="#aeacaf" rotate={20} />
      <Fluff cx={938} cy={415} rx={100} ry={86} fill="url(#companion-ivory)" />
      <Fluff cx={895} cy={385} rx={17} ry={21} fill="url(#companion-black)" fur="#bab7ba" rotate={29} />
      <Fluff cx={983} cy={386} rx={17} ry={21} fill="url(#companion-black)" fur="#bab7ba" rotate={-29} />
      <circle cx="899" cy="382" r="4" fill="#e2dbe0" /><circle cx="979" cy="383" r="4" fill="#e2dbe0" />
      <ellipse cx="939" cy="402" rx="10" ry="6" fill="#4b4144" />
      <ellipse cx="938" cy="447" rx="73" ry="59" fill="#a69c9a" opacity=".35" />
      <Fluff cx={938} cy={449} rx={67} ry={54} fill="url(#companion-cream)" />
      <ellipse cx="894" cy="461" rx="17" ry="12" fill="url(#companion-blush)" /><ellipse cx="980" cy="461" rx="17" ry="12" fill="url(#companion-blush)" />
      <Eye x={911} y={441} size={7.5} /><Eye x={964} y={441} size={7.5} />
      <Smile x={938} y={457} scale={.6} />
      <Collar x={937} y={503} width={158} color="#66d6ff" scale={.84} />
      <Fluff cx={938} cy={553} rx={30} ry={24} fill="url(#companion-ivory)" />
    </g>
  )
}

export function CompanionArt() {
  return (
    <svg viewBox="0 0 1060 620" role="img" aria-labelledby="companion-art-title companion-art-description" xmlns="http://www.w3.org/2000/svg">
      <title id="companion-art-title">{copy.site.art.title}</title>
      <desc id="companion-art-description">{copy.site.art.description}</desc>
      <defs>
        <radialGradient id="companion-honey" cx=".33" cy=".2" r=".83"><stop stopColor="#f8d5a6" /><stop offset=".4" stopColor="#e7b378" /><stop offset=".78" stopColor="#ca9159" /><stop offset="1" stopColor="#ae7444" /></radialGradient>
        <radialGradient id="companion-belly" cx=".4" cy=".25" r=".8"><stop stopColor="#f2d2a1" /><stop offset=".7" stopColor="#dfb37d" /><stop offset="1" stopColor="#c6905a" /></radialGradient>
        <radialGradient id="companion-inner-ear" cx=".4" cy=".6" r=".8"><stop stopColor="#a77148" /><stop offset="1" stopColor="#dab17f" /></radialGradient>
        <radialGradient id="companion-ivory" cx=".35" cy=".22" r=".85"><stop stopColor="#fffefa" /><stop offset=".45" stopColor="#f8f2e9" /><stop offset=".8" stopColor="#e9dfd5" /><stop offset="1" stopColor="#c9bdb1" /></radialGradient>
        <radialGradient id="companion-cream" cx=".4" cy=".23" r=".86"><stop stopColor="#fff7e9" /><stop offset=".6" stopColor="#f7e7d3" /><stop offset="1" stopColor="#e0c7ac" /></radialGradient>
        <radialGradient id="companion-pink" cx=".4" cy=".3" r=".8"><stop stopColor="#fbd5d8" /><stop offset="1" stopColor="#dca7b1" /></radialGradient>
        <radialGradient id="companion-blush"><stop stopColor="#f39b9e" stopOpacity=".85" /><stop offset=".75" stopColor="#f3a5a6" stopOpacity=".5" /><stop offset="1" stopColor="#f4a8a2" stopOpacity="0" /></radialGradient>
        <radialGradient id="companion-green" cx=".3" cy=".17" r=".85"><stop stopColor="#c4dc8c" /><stop offset=".36" stopColor="#a6cb6e" /><stop offset=".72" stopColor="#88af51" /><stop offset="1" stopColor="#648d3e" /></radialGradient>
        <radialGradient id="companion-lime" cx=".3" cy=".2" r=".9"><stop stopColor="#dbe9ae" /><stop offset="1" stopColor="#a8c270" /></radialGradient>
        <radialGradient id="companion-orange" cx=".3" cy=".2" r=".8"><stop stopColor="#ffd490" /><stop offset="1" stopColor="#e99a4f" /></radialGradient>
        <radialGradient id="companion-black" cx=".35" cy=".2" r=".9"><stop stopColor="#625a5c" /><stop offset=".48" stopColor="#423b3e" /><stop offset="1" stopColor="#27252c" /></radialGradient>
        <radialGradient id="companion-eye" cx=".34" cy=".2" r=".8"><stop stopColor="#645045" /><stop offset=".45" stopColor="#30251e" /><stop offset="1" stopColor="#151318" /></radialGradient>
        <linearGradient id="companion-nose" x2=".2" y2="1"><stop stopColor="#6e5240" /><stop offset="1" stopColor="#392a25" /></linearGradient>
        <filter id="companion-fur" x="-10%" y="-10%" width="120%" height="120%" colorInterpolationFilters="sRGB">
          <feTurbulence type="fractalNoise" baseFrequency=".58" numOctaves="3" seed="8" result="noise" />
          <feDiffuseLighting in="noise" surfaceScale="1.35" diffuseConstant="1.1" lightingColor="#fffaf0" result="fiber-light"><feDistantLight azimuth="235" elevation="56" /></feDiffuseLighting>
          <feComposite in="fiber-light" in2="SourceGraphic" operator="arithmetic" k1=".78" k2="0" k3=".32" k4="0" result="texture" />
          <feDisplacementMap in="texture" in2="noise" scale="3.2" xChannelSelector="R" yChannelSelector="G" />
        </filter>
        <filter id="companion-glow" x="-60%" y="-150%" width="220%" height="400%"><feGaussianBlur stdDeviation="6" /></filter>
        <filter id="companion-ground-shadow" x="-20%" y="-100%" width="140%" height="300%"><feGaussianBlur stdDeviation="8" /></filter>
        <filter id="companion-soft-shadow" x="-10%" y="-10%" width="120%" height="125%"><feDropShadow dx="2" dy="7" stdDeviation="5" floodColor="#514432" floodOpacity=".16" /></filter>
      </defs>
      <g fill="#434b36" opacity=".23" filter="url(#companion-ground-shadow)"><ellipse cx="164" cy="574" rx="112" ry="16" /><ellipse cx="412" cy="580" rx="160" ry="18" /><ellipse cx="710" cy="581" rx="122" ry="17" /><ellipse cx="940" cy="586" rx="105" ry="15" /></g>
      <g filter="url(#companion-soft-shadow)"><Rabbit /><Bear /><Dragon /><Panda /></g>
    </svg>
  )
}
