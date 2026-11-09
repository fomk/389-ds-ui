/**
 * Edit model of an entry and the diff that turns it into LDAP modifications.
 * Pure functions, no UI: see edit.test.ts.
 */
import type { components } from '@/api/schema'

export type EntryAttribute = components['schemas']['Attribute']
export type Change = components['schemas']['Change']

/** One attribute row in the editor. */
export interface EditAttr {
  /** Attribute description as on the server, including options (cn;lang-ru). */
  name: string
  /** Text values, or base64 when binary. Empty strings are ignored on save. */
  values: string[]
  binary: boolean
  /** Values as loaded; undefined for attributes added in the editor. */
  original?: string[]
}

const lower = (s: string) => s.toLowerCase()

/**
 * Builds editor rows from an entry. Operational, server-managed and redacted
 * (password) attributes and objectClass are not edited here.
 */
export function toEditAttrs(attrs: EntryAttribute[], editable: (a: EntryAttribute) => boolean): EditAttr[] {
  return attrs
    .filter((a) => lower(a.name) !== 'objectclass' && !a.operational && !a.redacted && editable(a))
    .map((a) => ({ name: a.name, values: [...a.values], binary: !!a.binary, original: [...a.values] }))
}

/** Adds empty rows for required attributes that are missing. */
export function ensureRequired(rows: EditAttr[], must: string[], binary: (name: string) => boolean): EditAttr[] {
  const present = new Set(rows.map((r) => lower(r.name.split(';')[0]!)))
  const added = must
    .filter((m) => lower(m) !== 'objectclass' && !present.has(lower(m)))
    .map((m) => ({ name: m, values: [''], binary: binary(m) }))
  return [...rows, ...added]
}

const clean = (values: string[]) => values.filter((v) => v !== '')

function sameSet(a: string[], b: string[]) {
  if (a.length !== b.length) return false
  const s = new Set(a)
  return b.every((v) => s.has(v))
}

/**
 * Computes the modifications that turn the original entry into the edited one.
 * Changed attributes are replaced as a whole (simple and order-independent);
 * the server applies all changes of one request atomically.
 */
export function diffEntry(
  originalClasses: string[],
  classes: string[],
  originalRows: EditAttr[],
  rows: EditAttr[],
): Change[] {
  const changes: Change[] = []
  if (!sameSet(originalClasses.map(lower), classes.map(lower))) {
    changes.push({ op: 'replace', attr: 'objectClass', values: classes })
  }
  const edited = new Map(rows.map((r) => [lower(r.name), r]))
  for (const o of originalRows) {
    if (!edited.has(lower(o.name))) changes.push({ op: 'delete', attr: o.name })
  }
  for (const r of rows) {
    const values = clean(r.values)
    const binary = r.binary || undefined
    if (!r.original) {
      if (values.length) changes.push({ op: 'add', attr: r.name, values, binary })
      continue
    }
    if (values.length === 0) changes.push({ op: 'delete', attr: r.name })
    else if (!sameSet(values, r.original)) changes.push({ op: 'replace', attr: r.name, values, binary })
  }
  return changes
}

export interface RowProblem {
  row: string
  kind: 'required' | 'singleValue' | 'integer' | 'notAllowed'
}

/** Client-side checks; the server remains the authority. */
export function validateRows(
  rows: EditAttr[],
  must: string[],
  isAllowed: (name: string) => boolean,
  info: (name: string) => { singleValue?: boolean; kind?: string } | undefined,
): RowProblem[] {
  const problems: RowProblem[] = []
  const mustSet = new Set(must.map(lower))
  for (const r of rows) {
    const base = r.name.split(';')[0]!
    const values = clean(r.values)
    const meta = info(base)
    if (mustSet.has(lower(base)) && values.length === 0 && !r.name.includes(';')) {
      problems.push({ row: r.name, kind: 'required' })
    }
    if (meta?.singleValue && values.length > 1) problems.push({ row: r.name, kind: 'singleValue' })
    if (meta?.kind === 'integer' && values.some((v) => !/^-?\d+$/.test(v.trim()))) {
      problems.push({ row: r.name, kind: 'integer' })
    }
    if (!isAllowed(base)) problems.push({ row: r.name, kind: 'notAllowed' })
  }
  return problems
}
