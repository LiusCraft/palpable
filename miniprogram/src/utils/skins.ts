// 皮套外观：家长端用的头像与配色。未知皮套（或还没起名）用彩蛋头像。

export interface SkinLook {
  label: string
  image: string
  /** 卡片底色 */
  tint: string
  /** 强调色（描边、标签） */
  accent: string
}

const looks: Record<string, SkinLook> = {
  dragon: { label: '小龙', image: '/assets/plush/dragon.svg', tint: '#E8F6DF', accent: '#5FAE48' },
  rabbit: { label: '小兔', image: '/assets/plush/rabbit.svg', tint: '#FFEFF3', accent: '#F27A9B' },
  bear: { label: '小熊', image: '/assets/plush/bear.svg', tint: '#FBEBDA', accent: '#C27C3E' },
  cat: { label: '小猫', image: '/assets/plush/cat.svg', tint: '#ECECF4', accent: '#6D6D8C' },
  panda: { label: '熊猫', image: '/assets/plush/panda.svg', tint: '#F1F1EE', accent: '#4A4A55' },
}

const fallback: SkinLook = { label: '小伙伴', image: '/assets/plush/egg.svg', tint: '#FFF3D6', accent: '#E0A422' }

export function skinLook(skin: string): SkinLook {
  return looks[skin] ?? fallback
}
