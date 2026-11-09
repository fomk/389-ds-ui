// Locale consistency check (docs/i18n.md):
//  1. every locale file has exactly the keys of en.json (reference);
//  2. placeholders ({name}) match the reference;
//  3. every static key used in src/ via t('…') / te('…') exists in en.json.
// Locales listed in REQUIRED must be complete; others may be partial (missing
// keys fall back to English) and only report their coverage.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const REQUIRED = new Set(['en', 'ru'])
const root = new URL('..', import.meta.url).pathname
const localesDir = join(root, 'src/i18n/locales')

const flatten = (obj, prefix = '') =>
  Object.entries(obj).flatMap(([k, v]) =>
    v && typeof v === 'object' ? flatten(v, `${prefix}${k}.`) : [[`${prefix}${k}`, v]],
  )
const placeholders = (msg) => [...new Set([...String(msg).matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort().join(",")

const load = (code) => new Map(flatten(JSON.parse(readFileSync(join(localesDir, `${code}.json`), 'utf8'))))
const ref = load('en')
const errors = []

for (const file of readdirSync(localesDir).filter((f) => f.endsWith('.json'))) {
  const code = file.replace(/\.json$/, '')
  if (code === 'en') continue
  const msgs = load(code)
  const missing = [...ref.keys()].filter((k) => !msgs.has(k))
  const extra = [...msgs.keys()].filter((k) => !ref.has(k))
  if (missing.length && REQUIRED.has(code)) report.push(`${code}: missing ${missing.length} key(s): ${missing.join(', ')}`)
  if (extra.length) errors.push(`${code}: unknown key(s) not in en.json: ${extra.join(', ')}`)
  for (const [k, v] of msgs) {
    if (ref.has(k) && placeholders(v) !== placeholders(ref.get(k))) {
      errors.push(`${code}: ${k}: placeholders {${placeholders(v)}} differ from en {${placeholders(ref.get(k))}}`)
    }
  }
  if (!REQUIRED.has(code)) {
    const pct = Math.round((100 * (ref.size - missing.length)) / ref.size)
    console.log(`${code}: ${pct}% translated (missing keys fall back to English)`)
  }
}

// Static keys used in source code.
const walk = (dir) =>
  readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    return statSync(p).isDirectory() ? walk(p) : /\.(vue|ts)$/.test(f) ? [p] : []
  })
const keyRe = /\b(?:t|te)\(\s*['"]([\w.-]+)['"]/g
for (const file of walk(join(root, 'src'))) {
  const src = readFileSync(file, 'utf8')
  for (const m of src.matchAll(keyRe)) {
    if (!ref.has(m[1])) errors.push(`${relative(root, file)}: key "${m[1]}" is not defined in en.json`)
  }
}

if (errors.length) {
  console.error('Locale check failed:\n  ' + errors.join('\n  '))
  process.exit(1)
}
console.log(`locales ok (${ref.size} keys)`)
