import { useRef, useState, type CSSProperties } from 'react'

type PetKey = 'squirrel' | 'cat' | 'rabbit' | 'dog' | 'tiger' | 'elephant'

type Pet = {
  key: PetKey
  animal: string
  name: string
  subtitle: string
  traits: string[]
  cutout: string
  boxImage: string
  theme: {
    background: string
    surface: string
    ink: string
    muted: string
    accent: string
    accentInk: string
  }
}

const pets: Pet[] = [
  {
    key: 'squirrel',
    animal: '松鼠',
    name: '松鼠比比',
    subtitle: '把每一次好奇，都藏进一颗闪闪发亮的坚果里。',
    traits: ['活泼好动，点子很多', '喜欢藏坚果，也喜欢分享', '听见你叫它，就会竖起耳朵'],
    cutout: '/characters/Squirrel.png',
    boxImage: '/characters/box-squirrel.webp',
    theme: { background: '#123a2b', surface: '#1d503b', ink: '#fff9ee', muted: '#c4dacb', accent: '#f4b35f', accentInk: '#1b2b21' },
  },
  {
    key: 'cat',
    animal: '小猫',
    name: '小猫潘布',
    subtitle: '软绵绵的陪伴，和一声比一声大的呼噜。',
    traits: ['慵懒粘人，喜欢贴贴', '呼噜声超大，陪你发呆', '偶尔淘气，心里一直有你'],
    cutout: '/characters/cat.png',
    boxImage: '/characters/box-cat.webp',
    theme: { background: '#eddaa5', surface: '#f4e7c5', ink: '#302719', muted: '#635332', accent: '#f4c964', accentInk: '#302414' },
  },
  {
    key: 'rabbit',
    animal: '兔子',
    name: '小兔诺诺',
    subtitle: '轻轻蹦到你身边，把害羞变成一朵小小的勇气。',
    traits: ['胆小害羞，需要温柔邀请', '爱吃胡萝卜，耳朵会发光', '熟悉之后，会悄悄和你说秘密'],
    cutout: '/characters/rabbit.png',
    boxImage: '/characters/box-rabbit.webp',
    theme: { background: '#9f5e76', surface: '#c27c94', ink: '#fff8f1', muted: '#f4ccd7', accent: '#f8c8d4', accentInk: '#412431' },
  },
  {
    key: 'dog',
    animal: '狗狗',
    name: '狗狗旺财',
    subtitle: '把每一次出发都变成冒险，尾巴先替你开心起来。',
    traits: ['忠诚勇敢，永远站你这边', '精力旺盛，最爱一起探索', '会记住你的声音和每个约定'],
    cutout: '/characters/dog.png',
    boxImage: '/characters/box-dog.webp',
    theme: { background: '#356988', surface: '#4a83a4', ink: '#f9fbf0', muted: '#c7e0e7', accent: '#ffc96c', accentInk: '#233440' },
  },
  {
    key: 'tiger',
    animal: '老虎',
    name: '老虎泰哥',
    subtitle: '看起来凶凶的，其实是最会把你逗笑的大猫。',
    traits: ['外冷内热，熟了就是大猫', '活力橙色，元气永不掉线', '擅长鼓励你挑战新游戏'],
    cutout: '/characters/tiger.png',
    boxImage: '/characters/box-tiger.webp',
    theme: { background: '#c85d29', surface: '#db7438', ink: '#fff8ed', muted: '#ffe0b6', accent: '#ffe179', accentInk: '#352116' },
  },
  {
    key: 'elephant',
    animal: '大象',
    name: '大象墩墩',
    subtitle: '慢慢听、稳稳记，把每一个重要的小瞬间都放在心里。',
    traits: ['温和稳重，给你大大的安全感', '记忆力超群，不忘每个故事', '声音低低的，像一片云在说话'],
    cutout: '/characters/elephant.png',
    boxImage: '/characters/box-elephant.webp',
    theme: { background: '#666879', surface: '#7c7d92', ink: '#fbfaf5', muted: '#d8d9e3', accent: '#f3b8c5', accentInk: '#342831' },
  },
]

function getThemeStyle(pet: Pet): CSSProperties {
  return {
    '--showcase-bg': pet.theme.background,
    '--showcase-surface': pet.theme.surface,
    '--showcase-ink': pet.theme.ink,
    '--showcase-muted': pet.theme.muted,
    '--showcase-accent': pet.theme.accent,
    '--showcase-accent-ink': pet.theme.accentInk,
  } as CSSProperties
}

export function Showcase({ onAdopt }: { onAdopt: () => void }) {
  const menu = useRef<HTMLDetailsElement>(null)
  const [selected, setSelected] = useState(0)
  const pet = pets[selected] ?? pets[0]

  const choosePet = (index: number) => {
    setSelected((index + pets.length) % pets.length)
    if (menu.current) menu.current.open = false
  }

  return (
    <main className="showcase" style={getThemeStyle(pet)}>
      <span className="showcase-word" aria-hidden="true">PALPITABLE</span>

      <header className="showcase-header">
        <button type="button" className="showcase-brand" onClick={() => choosePet(0)} aria-label="回到松鼠比比">
          <span className="showcase-brand-mark">P</span>
          <span className="showcase-brand-word">Palpitable</span>
          <span className="showcase-brand-dot" aria-hidden="true" />
        </button>

        <nav className="showcase-nav" aria-label="主导航">
          <details ref={menu} className="showcase-menu" onKeyDown={(event) => { if (event.key === 'Escape' && menu.current) { menu.current.open = false; menu.current.querySelector('summary')?.focus() } }}>
            <summary>宠物系列 <span aria-hidden="true">⌄</span></summary>
            <div className="showcase-menu-popover">
              {pets.map((item, index) => (
                <button key={item.key} type="button" onClick={() => choosePet(index)} className={index === selected ? 'is-active' : ''}>
                  <span>{item.animal}</span>
                  <small>{item.name}</small>
                </button>
              ))}
            </div>
          </details>
          <a href="#contact">联系我们</a>
          <a className="showcase-buy" href="#series">购买渠道 <span aria-hidden="true">↗</span></a>
        </nav>
      </header>

      <section className="showcase-hero" aria-live="polite">
        <div className="showcase-copy">
          <p className="showcase-eyebrow"><span className="showcase-eyebrow-line" />可感语音玩具 · 盲盒系列</p>
          <h1>{pet.name}</h1>
          <p className="showcase-subtitle">{pet.subtitle}</p>
          <ul className="showcase-traits">
            {pet.traits.map((trait) => <li key={trait}>{trait}</li>)}
          </ul>
          <button className="showcase-cta" type="button" onClick={onAdopt}>领养伙伴</button>
        </div>

        <div className="showcase-art-wrap">
          <div className="showcase-stage">
            {pets.map((item, index) => (
              <img
                key={item.key}
                className={`showcase-cutout ${selected === index ? 'is-visible' : ''}`}
                src={item.cutout}
                alt={selected === index ? `${item.name} 毛绒形象` : ''}
                aria-hidden={selected !== index}
                decoding="async"
                fetchPriority={index === 0 ? 'high' : 'auto'}
              />
            ))}
          </div>
        </div>
      </section>

      <section id="series" className="showcase-tray" aria-label="宠物系列选择">
        <div className="showcase-grid">
          {pets.map((item, index) => (
            <button
              key={item.key}
              type="button"
              className={`showcase-box ${index === selected ? 'is-selected' : ''}`}
              onClick={() => choosePet(index)}
              aria-pressed={index === selected}
            >
              <span className="showcase-box-image">
                <img src={item.boxImage} alt={`${item.name} 盲盒`} decoding="async" />
              </span>
              <span className="showcase-box-name">{item.name}</span>
            </button>
          ))}
        </div>
      </section>
      <span id="contact" className="showcase-contact-anchor" aria-hidden="true" />
    </main>
  )
}
