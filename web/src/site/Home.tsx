import { useEffect, useState, type MouseEvent, type ReactNode } from 'react'
import { copy } from '../ui/copy'
import { CompanionArt } from './CompanionArt'
import './Home.css'

const site = copy.site

function scrollToSection(event: MouseEvent<HTMLAnchorElement>, id: string): void {
  event.preventDefault()
  document.getElementById(id)?.scrollIntoView({
    behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
  })
}

function BearLogo({ className = '' }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={className} fill="none" aria-hidden="true">
      <path d="M9 26C1 14 9 4 17 9l7 6M42 14l6-6c9-5 17 6 10 17" fill="#3e9aca" stroke="currentColor" strokeWidth="5" />
      <path d="M32 12C17 12 5 24 5 38c0 14 11 21 27 21s27-7 27-21c0-14-12-26-27-26Z" fill="#fff" stroke="currentColor" strokeWidth="5" />
      <ellipse cx="22" cy="34" rx="2.7" ry="3.2" fill="currentColor" />
      <ellipse cx="43" cy="34" rx="2.7" ry="3.2" fill="currentColor" />
      <ellipse cx="32" cy="41" rx="5" ry="3.7" fill="currentColor" />
      <ellipse cx="14" cy="41" rx="4" ry="2.2" fill="#ffdfe8" />
      <ellipse cx="50" cy="41" rx="4" ry="2.2" fill="#ffdfe8" />
    </svg>
  )
}

function Arrow({ className = '' }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m9 5 7 7-7 7" />
    </svg>
  )
}

function PlayLink({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <a href="#/play" className={`site-play-link ${className}`}>
      <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
        <path d="M6 4.9c0-1.5 1.6-2.4 2.9-1.6l12 7.1a1.9 1.9 0 0 1 0 3.2l-12 7.1A1.9 1.9 0 0 1 6 19.1Z" />
      </svg>
      <span>{children}</span>
      <Arrow />
    </a>
  )
}

function Nav() {
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState('top')

  useEffect(() => {
    const update = () => {
      const current = site.nav.findLast((item) => {
        const section = document.getElementById(item.id)
        return section && section.getBoundingClientRect().top <= window.innerHeight * 0.35
      })
      setActive(current?.id ?? 'top')
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    update()
    window.addEventListener('scroll', update, { passive: true })
    window.addEventListener('keydown', closeOnEscape)
    return () => {
      window.removeEventListener('scroll', update)
      window.removeEventListener('keydown', closeOnEscape)
    }
  }, [])

  return (
    <header className="site-header">
      <a href="#top" onClick={(event) => scrollToSection(event, 'top')} className="site-brand" aria-label={`${copy.brand.name} ${copy.brand.en} 首页`}>
        <BearLogo className="site-brand-mark" />
        <span className="site-brand-cn">{copy.brand.name}</span>
        <span className="site-brand-en">{copy.brand.en}</span>
      </a>
      <button className="site-menu-toggle" type="button" aria-expanded={open} aria-controls="site-navigation" aria-label={open ? site.closeMenu : site.openMenu} onClick={() => setOpen(!open)}>
        <span />
        <span />
      </button>
      <nav id="site-navigation" className={`site-navigation${open ? ' is-open' : ''}`} aria-label={site.navigationLabel}>
        {site.nav.map((item) => (
          <a key={item.id} href={`#${item.id}`} aria-current={active === item.id ? 'location' : undefined} onClick={(event) => {
            scrollToSection(event, item.id)
            setOpen(false)
          }}>
            {item.label}
          </a>
        ))}
      </nav>
    </header>
  )
}

function Brush({ className = '' }: { className?: string }) {
  return (
    <svg viewBox="0 0 240 26" className={className} fill="none" stroke="currentColor" strokeLinecap="round" aria-hidden="true">
      <path d="M5 22C70 5 151 5 235 3" strokeWidth="2.7" />
      <path d="M14 24C86 9 153 7 211 6" strokeWidth="0.7" opacity=".45" />
    </svg>
  )
}

function Hero() {
  return (
    <section id="top" className="site-hero" aria-labelledby="site-hero-title">
      <div className="site-hero-scene">
        <img src="/hero-bg.png" className="site-hero-background" alt="" fetchPriority="high" width="1536" height="1024" />
        <div className="site-hero-haze" />
        <Nav />
        <div className="site-hero-copy">
          <h1 id="site-hero-title" className="site-hero-title">
            <span className="site-hero-name">{copy.brand.name}</span>
            <span>{site.title}</span>
            <span>{site.title2}</span>
          </h1>
          <p className="site-hero-lead">{site.lead}</p>
          <PlayLink>{site.ctaPlay}</PlayLink>
        </div>
        <div className="site-scribble site-scribble-slogan" aria-hidden="true">
          <span>{site.notes.sloganLines[0]}</span>
          <span>{site.notes.sloganLines[1]}</span>
          <Brush />
          <svg className="site-scribble-spark" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><path d="m12 2-2 6m9 4-5-1m-3 9-1-5M3 10l4 1" /></svg>
        </div>
        <div className="site-scribble site-scribble-hello" aria-hidden="true">
          <span>{site.notes.helloLines[0]}</span>
          <span>{site.notes.helloLines[1]}</span>
          <Brush />
          <svg className="site-hello-ticks" viewBox="0 0 280 100" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="m8 57 15 18M260 44l10-16M248 22l2-13" /></svg>
        </div>
        <div className="site-companions">
          <CompanionArt />
        </div>
      </div>
    </section>
  )
}

function FeatureIcon({ name, className = '' }: { name: string; className?: string }) {
  const common = { className, viewBox: '0 0 48 48', fill: 'none', stroke: 'currentColor', strokeWidth: 2.3, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const }
  switch (name) {
    case 'voice':
      return <svg {...common} aria-hidden="true"><path d="M35 24c0-9-6-16-15-16S5 15 5 24c0 3 1 6 2 8l-3 9 9-3c2 1 5 2 8 2 8 0 14-7 14-16Z" /><path d="M14 27c3 4 8 4 11 0M39 20c2 3 2 7 0 10m5-14c3 5 3 12 0 17" /></svg>
    case 'friends':
      return <svg {...common} aria-hidden="true"><circle cx="15" cy="16" r="7" fill="currentColor" stroke="none" /><circle cx="35" cy="16" r="7" fill="currentColor" stroke="none" /><path d="M3 36c0-8 5-12 12-12s12 4 12 12v2H3Zm23-9c3-2 5-3 9-3 7 0 12 5 12 12v2H30v-2c0-4-1-7-4-9Z" fill="currentColor" stroke="none" /><path d="m23 21 3 3 3-3" stroke="#fff" strokeWidth="2" /></svg>
    case 'zone':
      return <svg {...common} aria-hidden="true"><path d="M24 44S9 29 9 18a15 15 0 0 1 30 0c0 11-15 26-15 26Z" fill="currentColor" stroke="none" /><circle cx="24" cy="18" r="6" fill="#fff" stroke="none" /><path d="M12 15c1-5 5-9 10-9" stroke="#fff" opacity=".45" /></svg>
    case 'safe':
      return <svg {...common} aria-hidden="true"><path d="M24 4c5 4 11 5 17 6v13c0 10-8 17-17 21C15 40 7 33 7 23V10c6-1 12-2 17-6Z" fill="currentColor" stroke="none" /><path d="m18 24 5 5 9-13" stroke="#fff" strokeWidth="3" /><path d="M11 14v8c0 5 2 8 5 11" stroke="#fff" opacity=".45" /></svg>
    default:
      return <svg {...common} aria-hidden="true"><path d="M24 3c3 13 7 17 18 21-11 3-15 8-18 21C21 32 17 27 6 24 17 20 21 16 24 3Z" fill="currentColor" stroke="none" /><path d="m24 8 3 10M25 35l2-5" stroke="#fff" opacity=".7" /><path d="m37 35 2 5 4 1-4 2-2 4-1-4-4-2 4-1Z" fill="#fff" stroke="none" /></svg>
  }
}

function Features() {
  return (
    <section id="features" className="site-features" aria-label={site.nav[1].label}>
      <svg className="site-feature-wave" viewBox="0 0 1536 44" preserveAspectRatio="none" aria-hidden="true"><path d="M0 25C77 1 158 15 302 13L1051 12c222-5 353 0 485 30v2H0Z" fill="currentColor" /></svg>
      <div className="site-feature-inner">
        <ul className="site-feature-list">
          {site.features.map((feature) => (
            <li key={feature.key} className="site-feature">
              <span className="site-feature-icon" style={{ backgroundColor: feature.tone, color: feature.fg }}><FeatureIcon name={feature.key} /></span>
              <h2>{feature.title}</h2>
              <p>{feature.text}</p>
            </li>
          ))}
        </ul>
        <a href="#how" onClick={(event) => scrollToSection(event, 'how')} className="site-explore">
          <span>{site.more[0]}<br />{site.more[1]}<svg viewBox="0 0 40 20" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m3 13 32-5m-9-5 10 5-8 8" /></svg></span>
          <BearLogo />
        </a>
      </div>
    </section>
  )
}

function SectionTitle({ children, sub }: { children: ReactNode; sub?: string }) {
  return <div className="site-section-heading"><h2>{children}</h2>{sub && <p>{sub}</p>}</div>
}

export function Home() {
  return (
    <div className="palpable-site">
      <a className="site-skip-link" href="#actions">{site.skipToContent}</a>
      <main>
        <Hero />
        <Features />
        <section id="demo" className="site-section site-demo" aria-label={site.demo.title}>
          <SectionTitle sub={site.demo.sub}>{site.demo.title}</SectionTitle>
          <video className="site-demo-video" src="/palpable-demo.mp4" poster="/palpable-demo-poster.jpg" controls playsInline preload="none" width="1280" height="720">
            {site.demo.fallback}
          </video>
        </section>
        <section id="actions" className="site-section site-actions">
          <SectionTitle sub={site.actions.sub}>{site.actions.title}</SectionTitle>
          <div className="site-action-list">
            {site.actions.items.map((action) => (
              <div key={action.title} className="site-action">
                <span className="site-action-doodle" aria-hidden="true">{action.icon}</span>
                <h3>{action.title}</h3>
                <p>{action.text}</p>
              </div>
            ))}
          </div>
        </section>
        <section id="how" className="site-how-band">
          <div className="site-section site-how">
            <div className="site-how-intro">
              <span className="site-section-eyebrow">{site.how.eyebrow}</span>
              <SectionTitle sub={site.how.sub}>{site.how.title}</SectionTitle>
              <PlayLink>{site.how.cta}</PlayLink>
              <p className="site-browser-note">{site.requirement}</p>
            </div>
            <ol className="site-steps">
              {site.how.steps.map((step, index) => (
                <li key={step.title}><span className="site-step-number">{index + 1}</span><div><h3>{step.title}</h3><p>{step.text}</p></div></li>
              ))}
            </ol>
          </div>
        </section>
        <section id="why" className="site-section site-why">
          <SectionTitle sub={site.why.sub}>{site.why.title}</SectionTitle>
          <dl className="site-reasons">
            {site.why.items.map((item) => <div key={item.title}><dt>{item.title}</dt><dd>{item.text}</dd></div>)}
          </dl>
        </section>
        <section id="safe" className="site-section site-safety">
          <div className="site-safety-intro"><span className="site-safety-symbol"><FeatureIcon name="safe" /></span><SectionTitle sub={site.safe.sub}>{site.safe.title}</SectionTitle><a className="site-text-link" href="#/parent">{site.ctaParent}<Arrow /></a></div>
          <div><ul className="site-safety-list">
            {site.safe.items.map((item) => <li key={item.title}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m5 12 4 4L19 6" /></svg><div><h3>{item.title}</h3><p>{item.text}</p></div></li>)}
          </ul><p className="site-privacy-note">{site.safe.note}</p></div>
        </section>
        <section id="about" className="site-about-band">
          <div className="site-section site-about">
            <BearLogo className="site-about-bear" />
            <SectionTitle sub={site.about.text}>{site.about.title}</SectionTitle>
            <div className="site-final-cta"><PlayLink>{site.ctaPlay}</PlayLink><a href="#/parent" className="site-text-link">{site.ctaParent}<Arrow /></a></div>
          </div>
        </section>
      </main>
      <footer className="site-footer"><a href="#top" onClick={(event) => scrollToSection(event, 'top')}>{copy.brand.name}<span>{copy.brand.en}</span></a><p>{site.footer}</p><a href="#safe" onClick={(event) => scrollToSection(event, 'safe')}>{site.safetyLink}</a></footer>
    </div>
  )
}
