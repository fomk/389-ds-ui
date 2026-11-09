/** Parsing and formatting of LDAP attribute values for display. */

/** Syntax OID of UTCTime; everything else time-like is GeneralizedTime. */
export const UTC_TIME_SYNTAX = '1.3.6.1.4.1.1466.115.121.1.53'

/**
 * Parses GeneralizedTime ("20261109141908Z", "20261109141908.123Z",
 * "202611091419+0300") or, with utcTime=true, UTCTime ("261109141908Z").
 * The two formats cannot be told apart by length, so the caller passes the
 * attribute syntax. Returns null if the value is invalid.
 */
export function parseLdapTime(v: string, utcTime = false): Date | null {
  const year = utcTime ? '(\\d{2})' : '(\\d{4})'
  const re = new RegExp(`^${year}(\\d{2})(\\d{2})(\\d{2})(\\d{2})?(\\d{2})?(?:[.,](\\d+))?(Z|[+-]\\d{2}(?:\\d{2})?)?$`)
  const m = re.exec(v.trim())
  if (!m) return null
  let y = Number(m[1])
  if (utcTime) y += y < 50 ? 2000 : 1900
  const [mo, d, hh, mi, ss] = [m[2], m[3], m[4], m[5] ?? '0', m[6] ?? '0'].map(Number) as [number, number, number, number, number]
  // Date.UTC silently rolls over out-of-range fields (month 13 → next year).
  if (mo < 1 || mo > 12 || d < 1 || d > 31 || hh > 23 || mi > 59 || ss > 60) return null
  const ms = m[7] ? Math.round(Number(`0.${m[7]}`) * 1000) : 0
  let t = Date.UTC(y, mo - 1, d, hh, mi, ss, ms)
  const tz = m[8]
  if (tz && tz !== 'Z') {
    const sign = tz[0] === '-' ? -1 : 1
    const offset = Number(tz.slice(1, 3)) * 60 + Number(tz.slice(3, 5) || 0)
    t -= sign * offset * 60_000
  }
  const date = new Date(t)
  return Number.isNaN(date.getTime()) ? null : date
}

/** Size in bytes of a base64 string. */
export function base64Size(b64: string): number {
  const pad = b64.endsWith('==') ? 2 : b64.endsWith('=') ? 1 : 0
  return Math.floor((b64.length * 3) / 4) - pad
}

export function formatBytes(n: number, locale: string): string {
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: i ? 1 : 0 }).format(v)} ${units[i]}`
}

/** Triggers a browser download of base64 data. */
export function downloadBase64(b64: string, filename: string, type = 'application/octet-stream') {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  const url = URL.createObjectURL(new Blob([bytes], { type }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}
