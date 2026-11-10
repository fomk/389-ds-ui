import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { baseCompile } from '@intlify/message-compiler'
import { describe, expect, it } from 'vitest'

// Every message must compile: a syntax error (e.g. an unescaped "@") only
// shows up at runtime as a blank component otherwise.
const dir = join(__dirname, 'locales')

function flatten(obj: Record<string, unknown>, prefix = ''): Array<[string, string]> {
  return Object.entries(obj).flatMap(([k, v]) =>
    v && typeof v === 'object' ? flatten(v as Record<string, unknown>, `${prefix}${k}.`) : [[`${prefix}${k}`, String(v)]],
  ) as Array<[string, string]>
}

describe('locale messages', () => {
  for (const file of readdirSync(dir).filter((f) => f.endsWith('.json'))) {
    it(`${file} compiles`, () => {
      const problems: string[] = []
      for (const [key, msg] of flatten(JSON.parse(readFileSync(join(dir, file), 'utf8')))) {
        baseCompile(msg, { onError: (e) => problems.push(`${key}: ${e.message}`) })
      }
      expect(problems).toEqual([])
    })
  }
})
