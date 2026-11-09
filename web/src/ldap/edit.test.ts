import { describe, expect, it } from 'vitest'
import { diffEntry, ensureRequired, toEditAttrs, validateRows, type EditAttr, type EntryAttribute } from './edit'

const attrs: EntryAttribute[] = [
  { name: 'objectClass', values: ['top', 'person'] },
  { name: 'cn', values: ['John'] },
  { name: 'sn', values: ['Doe'] },
  { name: 'mail', values: ['a@x', 'b@x'] },
  { name: 'userPassword', values: ['{SSHA}'], redacted: true },
  { name: 'createTimestamp', values: ['20260101000000Z'], operational: true },
  { name: 'jpegPhoto', values: ['/9j/'], binary: true },
]

const rows = () => toEditAttrs(attrs, () => true)
const clone = (r: EditAttr[]) => r.map((x) => ({ ...x, values: [...x.values] }))

describe('toEditAttrs', () => {
  it('skips objectClass, operational and redacted attributes', () => {
    expect(rows().map((r) => r.name)).toEqual(['cn', 'sn', 'mail', 'jpegPhoto'])
  })
})

describe('ensureRequired', () => {
  it('adds empty rows for missing MUST attributes', () => {
    const r = ensureRequired(rows(), ['cn', 'sn', 'uid', 'objectClass'], () => false)
    expect(r.at(-1)).toEqual({ name: 'uid', values: [''], binary: false })
    expect(r.filter((x) => x.name === 'cn')).toHaveLength(1)
  })
})

describe('diffEntry', () => {
  it('returns nothing when unchanged (value order ignored)', () => {
    const edited = clone(rows())
    edited[2]!.values = ['b@x', 'a@x']
    expect(diffEntry(['top', 'person'], ['person', 'top'], rows(), edited)).toEqual([])
  })

  it('replaces changed, deletes removed/emptied, adds new attributes', () => {
    const orig = rows()
    const edited = clone(orig).filter((r) => r.name !== 'jpegPhoto')
    edited[0]!.values = ['John Doe']
    edited[2]!.values = ['', '']
    edited.push({ name: 'description', values: ['hello', ''], binary: false })
    edited.push({ name: 'title', values: [''], binary: false })
    expect(diffEntry(['top', 'person'], ['top', 'person'], orig, edited)).toEqual([
      { op: 'delete', attr: 'jpegPhoto' },
      { op: 'replace', attr: 'cn', values: ['John Doe'], binary: undefined },
      { op: 'delete', attr: 'mail' },
      { op: 'add', attr: 'description', values: ['hello'], binary: undefined },
    ])
  })

  it('puts the objectClass change first and keeps binary flags', () => {
    const edited = clone(rows())
    edited[3]!.values = ['AAAA']
    const changes = diffEntry(['top', 'person'], ['top', 'person', 'posixAccount'], rows(), edited)
    expect(changes[0]).toEqual({ op: 'replace', attr: 'objectClass', values: ['top', 'person', 'posixAccount'] })
    expect(changes[1]).toEqual({ op: 'replace', attr: 'jpegPhoto', values: ['AAAA'], binary: true })
  })
})

describe('validateRows', () => {
  const info = (n: string) =>
    ({ uidnumber: { singleValue: true, kind: 'integer' }, cn: { kind: 'string' } })[n.toLowerCase()]
  it('reports required, single-value, integer and not-allowed problems', () => {
    const r: EditAttr[] = [
      { name: 'cn', values: [''], binary: false },
      { name: 'cn;lang-ru', values: [''], binary: false },
      { name: 'uidNumber', values: ['12', 'x'], binary: false },
      { name: 'foo', values: ['1'], binary: false },
    ]
    const problems = validateRows(r, ['cn'], (n) => n !== 'foo', info)
    expect(problems).toEqual([
      { row: 'cn', kind: 'required' },
      { row: 'uidNumber', kind: 'singleValue' },
      { row: 'uidNumber', kind: 'integer' },
      { row: 'foo', kind: 'notAllowed' },
    ])
  })
})
