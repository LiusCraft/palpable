// 由 protocol/palpable.schema.json 生成前端协议类型，生成物不要手改。
import { compileFromFile } from 'json-schema-to-typescript'
import { mkdir, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..')
const schema = resolve(root, 'protocol', 'palpable.schema.json')
const out = resolve(root, 'web', 'src', 'shared', 'protocol.gen.ts')

const ts = await compileFromFile(schema, {
  bannerComment: '/* 由 make gen 生成，来源 protocol/palpable.schema.json，请勿手改 */',
  additionalProperties: false,
  unreachableDefinitions: true,
  cwd: dirname(schema),
})
await mkdir(dirname(out), { recursive: true })
await writeFile(out, ts)
console.log('wrote', out)
