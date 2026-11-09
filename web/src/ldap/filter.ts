/** LDAP filter helpers (RFC 4515). */

/** Escapes a value for use inside a filter: * ( ) \ and NUL. */
export function escapeFilterValue(v: string): string {
  return v.replace(/[*()\\\0]/g, (c) => '\\' + c.charCodeAt(0).toString(16).padStart(2, '0'))
}

/**
 * Filter for "find a person or group by name". Substring matches only use
 * attributes that 389-ds indexes for substrings by default (cn, sn, givenName,
 * mail); uid has only an equality index, so it is matched exactly. One
 * unindexed component would make the whole OR a full scan (seconds on large
 * directories).
 */
export function quickFilter(text: string): string {
  const v = escapeFilterValue(text.trim())
  const sub = ['cn', 'sn', 'givenName', 'mail'].map((a) => `(${a}=*${v}*)`).join('')
  return `(|${sub}(uid=${v}))`
}
