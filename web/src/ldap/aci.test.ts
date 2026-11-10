import { describe, expect, it } from 'vitest'
import { ACI_TEMPLATE, check, tokenize } from './aci'

const good =
  '(targetattr="cn || sn")(targetfilter="(objectClass=person)")(version 3.0; acl "self read"; allow (read, search,compare) userdn="ldap:///self" and ip="10.0.0.*";)'

describe('tokenize', () => {
  it('keeps the text and classifies words', () => {
    const tokens = tokenize(good)
    expect(tokens.map((t) => t.text).join('')).toBe(good)
    const kinds = Object.fromEntries(tokens.filter((t) => t.kind !== 'space').map((t) => [t.text, t.kind]))
    expect(kinds['targetattr']).toBe('keyword')
    expect(kinds['"cn || sn"']).toBe('string')
    expect(kinds['allow']).toBe('perm')
    expect(kinds['search']).toBe('right')
    expect(kinds['and']).toBe('op')
    expect(kinds['userdn']).toBe('keyword')
  })
  it('keeps parentheses inside strings', () => {
    expect(tokenize('(targetfilter="(uid=*)")').filter((t) => t.kind === 'string').map((t) => t.text)).toEqual(['"(uid=*)"'])
  })
})

describe('check', () => {
  it('accepts valid ACIs', () => {
    expect(check(good)).toEqual([])
    expect(check(ACI_TEMPLATE)).toEqual([])
    expect(check('(version 3.0; acl "anon"; deny (all) userdn="ldap:///anyone";)')).toEqual([])
  })
  it('reports the usual mistakes', () => {
    const keys = (s: string) => check(s).map((p) => p.key)
    expect(keys('')).toEqual(['empty'])
    expect(keys('(targetattr="cn)(version 3.0; acl "x"; allow (read) userdn="ldap:///self";)')).toContain('quote')
    expect(keys('(targetattr="cn"(version 3.0; acl "x"; allow (read) userdn="ldap:///self";)')).toContain('parens')
    expect(keys('(targetattr="cn")(version 2.0; acl "x"; allow (read) userdn="ldap:///self";)')).toContain('version')
    expect(keys('(targetattr="cn")(version 3.0; acl x; allow (read) userdn="ldap:///self";)')).toContain('name')
    expect(keys('(targetattr="cn")(version 3.0; acl "x"; permit (read) userdn="ldap:///self";)')).toContain('perm')
    expect(check('(targetattr="cn")(version 3.0; acl "x"; allow (read, fly) userdn="ldap:///self";)')).toContainEqual({
      key: 'right',
      params: { right: 'fly' },
    })
    expect(keys('(targetattr="cn")(version 3.0; acl "x"; allow (read);)')).toContain('bindRule')
    expect(keys('(targetattr="cn")(version 3.0; acl "x"; allow (read) userdn="ldap:///self")')).toContain('end')
    expect(check('(targetattrs="cn")(version 3.0; acl "x"; allow (read) userdn="ldap:///self";)')).toContainEqual({
      key: 'target',
      params: { target: 'targetattrs' },
    })
  })
})
