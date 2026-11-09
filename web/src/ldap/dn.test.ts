import { describe, expect, it } from 'vitest'
import { ancestorsUnder, dnKey, escapeRDNValue, splitRDN } from './dn'

describe('splitRDN', () => {
  it('splits at the first unescaped comma', () => {
    expect(splitRDN('uid=jdoe,ou=People,dc=example,dc=com')).toEqual(['uid=jdoe', 'ou=People,dc=example,dc=com'])
    expect(splitRDN('cn=Doe\\, John,ou=People')).toEqual(['cn=Doe\\, John', 'ou=People'])
    expect(splitRDN('dc=com')).toEqual(['dc=com', ''])
  })
})

describe('dnKey', () => {
  it('ignores case and spaces around separators', () => {
    expect(dnKey('UID=JDoe, OU=People , dc = example,dc=com')).toBe(dnKey('uid=jdoe,ou=people,dc=example,dc=com'))
  })
})

describe('ancestorsUnder', () => {
  it('lists ancestors top-down from the root', () => {
    expect(ancestorsUnder('uid=x,ou=People,dc=example,dc=com', 'dc=example,dc=com')).toEqual([
      'dc=example,dc=com',
      'ou=People,dc=example,dc=com',
    ])
  })
  it('returns [] for the root itself and null outside it', () => {
    expect(ancestorsUnder('dc=example,dc=com', 'DC=Example,DC=com')).toEqual([])
    expect(ancestorsUnder('cn=config', 'dc=example,dc=com')).toBeNull()
  })
})

describe('escapeRDNValue', () => {
  it('escapes special characters and leading/trailing markers', () => {
    expect(escapeRDNValue('Doe, John')).toBe('Doe\\, John')
    expect(escapeRDNValue('a+b=c')).toBe('a\\+b\\=c')
    expect(escapeRDNValue('#1 ')).toBe('\\#1\\ ')
    expect(escapeRDNValue('Иван')).toBe('Иван')
  })
  it('round-trips through splitRDN', () => {
    const rdn = 'cn=' + escapeRDNValue('Doe, John')
    expect(splitRDN(rdn + ',ou=People')).toEqual([rdn, 'ou=People'])
  })
})
