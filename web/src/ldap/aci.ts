/**
 * 389-ds ACI text: tokens for highlighting and a light syntax check that
 * catches the usual mistakes before the server does (it has the last word):
 *
 *   (target…="…")… (version 3.0; acl "name"; allow|deny (rights) bindrule;)
 */

export type TokenKind = 'keyword' | 'string' | 'punct' | 'op' | 'right' | 'perm' | 'text' | 'space'
export interface Token {
  kind: TokenKind
  text: string
}

export const TARGET_KEYWORDS = [
  'target',
  'targetattr',
  'targetfilter',
  'targattrfilters',
  'targetscope',
  'target_to',
  'target_from',
  'targetcontrol',
  'extop',
]
export const BIND_KEYWORDS = [
  'userdn',
  'groupdn',
  'roledn',
  'userattr',
  'authmethod',
  'ip',
  'dns',
  'timeofday',
  'dayofweek',
  'ssf',
]
export const RIGHTS = ['read', 'write', 'add', 'delete', 'search', 'compare', 'selfwrite', 'proxy', 'moddn', 'all']

const KEYWORDS = new Set([...TARGET_KEYWORDS, ...BIND_KEYWORDS, 'version', 'acl'])
const OPS = new Set(['and', 'or', 'not'])
const PERMS = new Set(['allow', 'deny'])

/** Splits an ACI into tokens; the concatenated texts equal the input. */
export function tokenize(aci: string): Token[] {
  const out: Token[] = []
  let i = 0
  while (i < aci.length) {
    const ch = aci[i]!
    if (ch === '"') {
      let j = i + 1
      while (j < aci.length && aci[j] !== '"') j += aci[j] === '\\' ? 2 : 1
      out.push({ kind: 'string', text: aci.slice(i, Math.min(j + 1, aci.length)) })
      i = j + 1
    } else if ('();,'.includes(ch)) {
      out.push({ kind: 'punct', text: ch })
      i++
    } else if (ch === '=' || ch === '!' || ch === '<' || ch === '>') {
      let j = i + 1
      while (j < aci.length && '=!<>'.includes(aci[j]!)) j++
      out.push({ kind: 'op', text: aci.slice(i, j) })
      i = j
    } else if (/\s/.test(ch)) {
      let j = i + 1
      while (j < aci.length && /\s/.test(aci[j]!)) j++
      out.push({ kind: 'space', text: aci.slice(i, j) })
      i = j
    } else {
      let j = i + 1
      while (j < aci.length && !/[\s();,"=!<>]/.test(aci[j]!)) j++
      const word = aci.slice(i, j)
      const lower = word.toLowerCase()
      const kind: TokenKind = KEYWORDS.has(lower)
        ? 'keyword'
        : OPS.has(lower)
          ? 'op'
          : PERMS.has(lower)
            ? 'perm'
            : RIGHTS.includes(lower)
              ? 'right'
              : 'text'
      out.push({ kind, text: word })
      i = j
    }
  }
  return out
}

/** Problem keys under aci.problems in the locale files, with parameters. */
export interface Problem {
  key: string
  params?: Record<string, string>
}

/** Finds structural mistakes; an empty list does not guarantee the server accepts it. */
export function check(aci: string): Problem[] {
  const problems: Problem[] = []
  const text = aci.trim()
  if (!text) return [{ key: 'empty' }]

  // Quotes and parentheses (outside quotes).
  let depth = 0
  let inQuote = false
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (inQuote) {
      if (ch === '\\') i++
      else if (ch === '"') inQuote = false
    } else if (ch === '"') inQuote = true
    else if (ch === '(') depth++
    else if (ch === ')' && --depth < 0) break
  }
  if (inQuote) problems.push({ key: 'quote' })
  if (depth !== 0) problems.push({ key: 'parens' })

  const tokens = tokenize(text).filter((t) => t.kind !== 'space')
  const words = tokens.map((t) => t.text.toLowerCase())

  const v = words.indexOf('version')
  if (v < 0 || words[v + 1] !== '3.0') problems.push({ key: 'version' })
  const a = words.indexOf('acl')
  if (a < 0 || tokens[a + 1]?.kind !== 'string') problems.push({ key: 'name' })
  const p = tokens.findIndex((t) => t.kind === 'perm')
  if (p < 0) problems.push({ key: 'perm' })
  else {
    // allow (read, search) …
    if (tokens[p + 1]?.text !== '(') problems.push({ key: 'rights' })
    else {
      for (let j = p + 2; j < tokens.length && tokens[j]!.text !== ')'; j++) {
        const t = tokens[j]!
        if (t.kind === 'text') problems.push({ key: 'right', params: { right: t.text } })
      }
      const close = tokens.findIndex((t, j) => j > p && t.text === ')')
      const rest = tokens.slice(close + 1)
      if (!rest.some((t) => t.kind === 'keyword' && BIND_KEYWORDS.includes(t.text.toLowerCase()))) {
        problems.push({ key: 'bindRule' })
      }
    }
  }
  if (!text.endsWith(';)')) problems.push({ key: 'end' })

  // Target part: "(keyword op value)" before "(version".
  const head = v >= 0 ? tokens.slice(0, v - 1) : []
  for (let j = 0; j < head.length; j++) {
    if (head[j]!.text === '(' && head[j + 1]) {
      const kw = head[j + 1]!
      if (kw.kind !== 'keyword' || !TARGET_KEYWORDS.includes(kw.text.toLowerCase())) {
        problems.push({ key: 'target', params: { target: kw.text } })
      }
    }
  }
  return problems
}

/** A starting point for a new ACI. */
export const ACI_TEMPLATE =
  '(targetattr="*")(version 3.0; acl "Describe the rule"; allow (read, search, compare) userdn="ldap:///self";)'
