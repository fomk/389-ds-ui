// Fails if any installed npm package has a license outside the allowlist
// (docs/decisions.md, ADR-004). Catches cases like PrimeVue 5 switching to a
// commercial "SEE LICENSE IN" license.
import { readFileSync } from 'node:fs'

const allowed = new Set([
  'MIT', 'ISC', '0BSD', 'BSD-2-Clause', 'BSD-3-Clause', 'Apache-2.0', 'MPL-2.0',
  'BlueOak-1.0.0', 'CC0-1.0', 'Python-2.0',
])

// SPDX expressions such as "(MIT OR Apache-2.0)" pass if any alternative is allowed.
const ok = (license) =>
  typeof license === 'string' &&
  license.replace(/[()]/g, '').split(/\s+OR\s+/).some((l) => allowed.has(l.trim()))

const lock = JSON.parse(readFileSync(new URL('../package-lock.json', import.meta.url)))
const bad = Object.entries(lock.packages)
  .filter(([path]) => path.startsWith('node_modules/'))
  .filter(([, meta]) => !ok(meta.license))
  .map(([path, meta]) => `${path.replace(/^.*node_modules\//, '')}@${meta.version}: ${meta.license ?? 'UNKNOWN'}`)

if (bad.length) {
  console.error('Packages with disallowed or unknown licenses:\n  ' + bad.join('\n  '))
  process.exit(1)
}
console.log(`licenses ok (${Object.keys(lock.packages).length - 1} packages)`)
