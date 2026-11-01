// 小程序静态检查：tsc 管不到 WXML / JSON，这里补上最容易在真机上才暴露的问题。
//   1. JSON 能解析；2. WXML 里用到的组件都在 usingComponents 里注册过；
//   3. WXML 插值里不调用函数（WXML 不支持，setData 也会丢掉函数）；
//   4. 引用的 /assets 文件存在；5. 文案不引导填写孩子的个人信息（AGENTS.md 硬约束）。
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { join, dirname, relative } from 'node:path'

const root = new URL('../src/', import.meta.url).pathname
const problems = []
const walk = (d) =>
  readdirSync(d).flatMap((n) => {
    const p = join(d, n)
    if (n === 'miniprogram_npm' || n === 'node_modules') return []
    return statSync(p).isDirectory() ? walk(p) : [p]
  })
const files = walk(root)
const rel = (p) => relative(root, p)

const readJSON = (p) => {
  try {
    return JSON.parse(readFileSync(p, 'utf8'))
  } catch (e) {
    problems.push(`${rel(p)}: invalid JSON (${e.message})`)
    return {}
  }
}
for (const f of files.filter((f) => f.endsWith('.json'))) readJSON(f)

const app = readJSON(join(root, 'app.json'))
const globalComps = Object.keys(app.usingComponents ?? {})
const native = new Set(['view', 'text', 'image', 'button', 'input', 'block', 'navigator', 'scroll-view', 'swiper', 'swiper-item', 'slot', 'template', 'textarea', 'icon', 'rich-text', 'label', 'form', 'picker', 'switch', 'radio', 'checkbox', 'web-view', 'map', 'canvas', 'video', 'cover-view', 'cover-image', 'progress', 'open-data', 'page-meta', 'navigation-bar', 'wxs', 'import', 'include'])

for (const w of files.filter((f) => f.endsWith('.wxml'))) {
  const src = readFileSync(w, 'utf8')
  const json = readJSON(w.replace(/\.wxml$/, '.json'))
  const known = new Set([...globalComps, ...Object.keys(json.usingComponents ?? {})])
  for (const [, tag] of src.matchAll(/<([a-z][a-z0-9-]*)[\s/>]/g)) {
    if (!native.has(tag) && !known.has(tag)) problems.push(`${rel(w)}: <${tag}> not registered in usingComponents`)
  }
  for (const [expr] of src.matchAll(/\{\{[^}]*\}\}/g)) {
    if (/[A-Za-z_$][\w$]*\s*\(/.test(expr)) problems.push(`${rel(w)}: function call in template ${expr}`)
  }
}

for (const f of files.filter((f) => /\.(wxml|ts|wxss|json)$/.test(f))) {
  for (const [, asset] of readFileSync(f, 'utf8').matchAll(/["'(](\/assets\/[^"')]+)["')]/g)) {
    if (!existsSync(join(root, asset))) problems.push(`${rel(f)}: missing asset ${asset}`)
  }
}

// 自定义组件的路径要真的指向组件文件（npm 包名路径由构建 npm 生成，不在这里查）。
for (const j of files.filter((f) => f.endsWith('.json'))) {
  const comps = readJSON(j).usingComponents ?? {}
  for (const [name, path] of Object.entries(comps)) {
    if (path.startsWith('.') || path.startsWith('/')) {
      const base = path.startsWith('/') ? join(root, path) : join(dirname(j), path)
      if (!existsSync(`${base}.json`) || !(existsSync(`${base}.ts`) || existsSync(`${base}.js`))) {
        problems.push(`${rel(j)}: <${name}> path ${path} has no component files`)
      }
    }
  }
}

for (const p of app.pages ?? []) {
  for (const ext of ['ts', 'wxml', 'json']) {
    if (!existsSync(join(root, `${p}.${ext}`))) problems.push(`app.json: page ${p} has no .${ext}`)
  }
}

const copy = readFileSync(join(root, 'utils/copy.ts'), 'utf8')
const solicit = /(请|告诉|输入|填写|留下)[^'"\n]{0,8}(住址|地址|学校|电话|手机号|身份证)/
for (const line of copy.split('\n')) {
  if (!line.trim().startsWith('//') && solicit.test(line)) problems.push(`utils/copy.ts: copy asks for personal info: ${line.trim()}`)
}

if (problems.length) {
  console.error(problems.map((p) => `✗ ${p}`).join('\n'))
  process.exit(1)
}
console.log(`✓ ${files.length} files checked`)
