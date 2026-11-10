import type { Schemas } from '@/api/client'

type Rule = Schemas['MatchingRule']
type Option = { label: string; value: string }

/**
 * Matching rules split by use, from their names: 389-ds names ordering rules
 * "…OrderingMatch" and substring rules "…SubstringsMatch"; the rest are
 * equality rules. Rules without a name are left out.
 */
export function ruleOptions(rules: Rule[]): { equality: Option[]; ordering: Option[]; substr: Option[] } {
  const out = { equality: [] as Option[], ordering: [] as Option[], substr: [] as Option[] }
  const seen = new Set<string>()
  for (const r of rules) {
    const n = r.names[0]
    if (!n || seen.has(n.toLowerCase())) continue
    seen.add(n.toLowerCase())
    const lower = n.toLowerCase()
    const target = lower.includes('substring') ? out.substr : lower.includes('ordering') ? out.ordering : out.equality
    target.push({ label: n, value: n })
  }
  for (const list of Object.values(out)) list.sort((a, b) => a.label.localeCompare(b.label))
  return out
}
