import { describe, expect, it } from 'vitest'
import {
  changedAttrs,
  durationUnits,
  fieldKey,
  flagsSet,
  flagsValue,
  pickUnit,
  sameValues,
  sizeUnits,
  unknownBits,
} from './fields'

describe('pickUnit', () => {
  it('chooses the largest exact unit', () => {
    expect(pickUnit(21474836480, sizeUnits).key).toBe('GB')
    expect(pickUnit(2097152, sizeUnits).key).toBe('MB')
    expect(pickUnit(1000, sizeUnits).key).toBe('B')
    expect(pickUnit(8640000, durationUnits).key).toBe('d')
    expect(pickUnit(3600, durationUnits).key).toBe('h')
    expect(pickUnit(90, durationUnits).key).toBe('s')
  })
  it('keeps special values in the base unit', () => {
    expect(pickUnit(0, durationUnits).key).toBe('s')
    expect(pickUnit(-1, sizeUnits).key).toBe('B')
  })
})

describe('flags', () => {
  const flags = [
    { name: 'trace', value: 1 },
    { name: 'default', value: 16384 },
  ]
  it('splits and joins bit masks, keeping unknown bits', () => {
    expect(flagsSet(16385, flags)).toEqual(['trace', 'default'])
    expect(unknownBits(16384 + 1 + 1024, flags)).toBe(1024)
    expect(flagsValue(['default'], flags, 1024)).toBe(17408)
  })
})

describe('change detection', () => {
  it('ignores blanks', () => {
    expect(sameValues([''], [])).toBe(true)
    expect(sameValues([' on '], ['on'])).toBe(true)
    expect(changedAttrs({ a: ['1'], b: ['x'], c: [] }, { a: ['1'], b: ['y'], c: [''] })).toEqual(['b'])
  })
})

describe('fieldKey', () => {
  it('uses shared texts when the field names one', () => {
    expect(fieldKey('log-access', { attr: 'nsslapd-accesslog-mode', type: 'string', text: 'log.mode' })).toBe('pages._fields.log.mode')
    expect(fieldKey('ldapi', { attr: 'nsslapd-ldapilisten', type: 'bool' })).toBe('pages.ldapi.fields.nsslapd-ldapilisten')
  })
})
