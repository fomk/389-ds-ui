/** Client-side DN helpers (display and navigation only; the server validates). */

/** Splits "uid=x,ou=y,dc=z" into ["uid=x", "ou=y,dc=z"], honouring backslash escapes. */
export function splitRDN(dn: string): [rdn: string, parent: string] {
  let escaped = false
  for (let i = 0; i < dn.length; i++) {
    const c = dn[i]
    if (escaped) escaped = false
    else if (c === '\\') escaped = true
    else if (c === ',' || c === ';') return [dn.slice(0, i).trim(), dn.slice(i + 1).trim()]
  }
  return [dn.trim(), '']
}

/** Case- and whitespace-insensitive comparison key (approximation of DN matching). */
export function dnKey(dn: string): string {
  const parts: string[] = []
  let rest = dn
  while (rest) {
    const [rdn, parent] = splitRDN(rest)
    parts.push(
      rdn
        .split('+')
        .map((ava) => ava.replace(/\s*=\s*/, '=').trim())
        .join('+')
        .toLowerCase(),
    )
    rest = parent
  }
  return parts.join(',')
}

/**
 * Ancestors of dn below (and including) the given root, top-down, excluding dn
 * itself. Returns null if dn is not under root.
 */
export function ancestorsUnder(dn: string, root: string): string[] | null {
  const rootKey = dnKey(root)
  const chain: string[] = []
  let [, parent] = splitRDN(dn)
  if (dnKey(dn) === rootKey) return []
  while (parent) {
    chain.unshift(parent)
    if (dnKey(parent) === rootKey) return chain
    ;[, parent] = splitRDN(parent)
  }
  return null
}

/** Escapes an attribute value for use in an RDN (RFC 4514 §2.4). */
export function escapeRDNValue(v: string): string {
  let out = v.replace(/[\\,+"<>;=]/g, (c) => '\\' + c)
  if (out.startsWith('#') || out.startsWith(' ')) out = '\\' + out
  if (out.endsWith(' ') && !out.endsWith('\\ ')) out = out.slice(0, -1) + '\\ '
  return out
}
