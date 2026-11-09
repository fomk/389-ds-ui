import { describe, expect, it } from 'vitest'
import { applyDefaults, nextNumberAttrs, renderDefault } from './templates'

describe('renderDefault', () => {
  const values: Record<string, string> = { givenName: 'Ada', sn: 'Lovelace', uid: 'ada' }
  const v = (a: string) => values[a]
  const next = (a: string) => (a === 'uidNumber' ? 10007 : undefined)

  it('substitutes fields and next numbers', () => {
    expect(renderDefault('{givenName} {sn}', v, next)).toBe('Ada Lovelace')
    expect(renderDefault('/home/{uid}', v, next)).toBe('/home/ada')
    expect(renderDefault('{next:uidNumber}', v, next)).toBe('10007')
  })
  it('collapses spaces left by missing values', () => {
    expect(renderDefault('{givenName} {middle} {sn}', v, next)).toBe('Ada Lovelace')
    expect(renderDefault('{missing}', v, next)).toBe('')
  })
})

describe('nextNumberAttrs', () => {
  it('collects {next:attr} tokens once', () => {
    expect(nextNumberAttrs(['{next:uidNumber}', '{cn}', 'x{next:uidNumber}', '{next:gidNumber}'])).toEqual([
      'uidNumber',
      'gidNumber',
    ])
  })
})

describe('applyDefaults', () => {
  it('follows other fields until the user edits the value', () => {
    const rows = [
      { name: 'givenName', values: ['Ada'] },
      { name: 'sn', values: ['Lovelace'] },
      { name: 'cn', values: [''] },
    ]
    const defaults = { cn: '{givenName} {sn}' }
    let auto = applyDefaults(rows, defaults, {}, () => undefined)
    expect(rows[2]!.values[0]).toBe('Ada Lovelace')

    rows[1]!.values[0] = 'Byron'
    auto = applyDefaults(rows, defaults, auto, () => undefined)
    expect(rows[2]!.values[0]).toBe('Ada Byron')

    rows[2]!.values[0] = 'Countess of Lovelace' // user edit
    applyDefaults(rows, defaults, auto, () => undefined)
    expect(rows[2]!.values[0]).toBe('Countess of Lovelace')
  })
})
