/**
 * Template defaults: "{givenName} {sn}", "/home/{uid}", "{next:uidNumber}".
 * {attr} is replaced with the first value of another field, {next:attr} with a
 * number suggested by the server.
 */

const TOKEN = /\{(next:)?([A-Za-z][A-Za-z0-9-]*)\}/g

/** Attributes whose next free number a default needs. */
export function nextNumberAttrs(defaults: string[]): string[] {
  const out = new Set<string>()
  for (const d of defaults) for (const m of d.matchAll(TOKEN)) if (m[1]) out.add(m[2]!)
  return [...out]
}

/**
 * Renders a default. Missing values become empty strings; the result is
 * trimmed and runs of spaces collapsed ("{givenName} {sn}" with only sn set
 * gives "Doe", not " Doe").
 */
export function renderDefault(
  template: string,
  value: (attr: string) => string | undefined,
  next: (attr: string) => number | undefined,
): string {
  const out = template.replace(TOKEN, (_, isNext: string | undefined, attr: string) => {
    if (isNext) return String(next(attr) ?? '')
    return value(attr) ?? ''
  })
  return out.replace(/ {2,}/g, ' ').trim()
}

/**
 * Applies defaults to rows that the user has not edited: a row is
 * "untouched" while its first value is empty or still equals the value we
 * filled in last time. Returns the new auto-filled values.
 */
export function applyDefaults(
  rows: Array<{ name: string; values: string[] }>,
  defaults: Record<string, string>,
  previous: Record<string, string>,
  next: (attr: string) => number | undefined,
): Record<string, string> {
  const byName = new Map(rows.map((r) => [r.name.toLowerCase(), r]))
  const value = (attr: string) => byName.get(attr.toLowerCase())?.values[0]
  const filled: Record<string, string> = {}
  for (const [attr, tpl] of Object.entries(defaults)) {
    const row = byName.get(attr.toLowerCase())
    if (!row) continue
    const current = row.values[0] ?? ''
    if (current !== '' && current !== previous[attr]) continue // edited by the user
    const v = renderDefault(tpl, value, next)
    row.values[0] = v
    filled[attr] = v
  }
  return filled
}
