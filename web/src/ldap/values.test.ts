import { describe, expect, it } from 'vitest'
import { base64Size, formatBytes, parseLdapTime } from './values'

describe('parseLdapTime', () => {
  it('parses GeneralizedTime in UTC', () => {
    expect(parseLdapTime('20261109141908Z')?.toISOString()).toBe('2026-11-09T14:19:08.000Z')
  })
  it('parses fractions and offsets', () => {
    expect(parseLdapTime('20261109141908.5Z')?.toISOString()).toBe('2026-11-09T14:19:08.500Z')
    expect(parseLdapTime('20261109171908+0300')?.toISOString()).toBe('2026-11-09T14:19:08.000Z')
    expect(parseLdapTime('202611091419-0130')?.toISOString()).toBe('2026-11-09T15:49:00.000Z')
  })
  it('parses UTCTime with a two-digit year', () => {
    expect(parseLdapTime('261109141908Z', true)?.getUTCFullYear()).toBe(2026)
    expect(parseLdapTime('991231235959Z', true)?.getUTCFullYear()).toBe(1999)
    // The same digits read as GeneralizedTime are a different date (year 2611).
    expect(parseLdapTime('261109141908Z')?.getUTCFullYear()).toBe(2611)
  })
  it('rejects garbage', () => {
    expect(parseLdapTime('yesterday')).toBeNull()
    expect(parseLdapTime('')).toBeNull()
    expect(parseLdapTime('20261399000000Z')).toBeNull()
    expect(parseLdapTime('20261109250000Z')).toBeNull()
  })
})

describe('sizes', () => {
  it('computes decoded base64 size', () => {
    expect(base64Size(btoa('abc'))).toBe(3)
    expect(base64Size(btoa('abcd'))).toBe(4)
    expect(base64Size(btoa('abcde'))).toBe(5)
  })
  it('formats bytes', () => {
    expect(formatBytes(512, 'en')).toBe('512 B')
    expect(formatBytes(2048, 'en')).toBe('2 KB')
  })
})
