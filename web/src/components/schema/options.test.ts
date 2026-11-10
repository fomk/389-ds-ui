import { describe, expect, it } from 'vitest'
import { ruleOptions } from './options'

describe('ruleOptions', () => {
  it('splits matching rules by use and drops unnamed/duplicate ones', () => {
    const r = ruleOptions([
      { oid: '2.5.13.2', names: ['caseIgnoreMatch'] },
      { oid: '2.5.13.3', names: ['caseIgnoreOrderingMatch'] },
      { oid: '2.5.13.4', names: ['caseIgnoreSubstringsMatch'] },
      { oid: '1.2.3', names: [] },
      { oid: '2.5.13.2.1', names: ['caseignorematch'] },
    ])
    expect(r.equality.map((o) => o.value)).toEqual(['caseIgnoreMatch'])
    expect(r.ordering.map((o) => o.value)).toEqual(['caseIgnoreOrderingMatch'])
    expect(r.substr.map((o) => o.value)).toEqual(['caseIgnoreSubstringsMatch'])
  })
})
