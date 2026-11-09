import { describe, expect, it } from 'vitest'
import { escapeFilterValue, quickFilter } from './filter'

describe('escapeFilterValue', () => {
  it('escapes filter metacharacters', () => {
    expect(escapeFilterValue('a*b(c)d\\e')).toBe('a\\2ab\\28c\\29d\\5ce')
    expect(escapeFilterValue('cn=Doe\\, John,dc=x')).toBe('cn=Doe\\5c, John,dc=x')
  })
})

describe('quickFilter', () => {
  it('uses substring matches only on indexed attributes and exact uid', () => {
    expect(quickFilter(' jo*')).toBe('(|(cn=*jo\\2a*)(sn=*jo\\2a*)(givenName=*jo\\2a*)(mail=*jo\\2a*)(uid=jo\\2a))')
  })
})
