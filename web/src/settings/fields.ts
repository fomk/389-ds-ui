/**
 * Value helpers for declarative settings pages (internal/pages): unit
 * scaling for size/duration fields, bit masks for flags fields and change
 * detection. The server stores everything as strings; so does the form.
 */
import type { Schemas } from '@/api/client'

export type PageField = Schemas['PageField']
export type PageFlag = Schemas['PageFlag']

export interface Unit {
  /** Message key under pages._units. */
  key: string
  factor: number
}

export const sizeUnits: Unit[] = [
  { key: 'B', factor: 1 },
  { key: 'KB', factor: 1024 },
  { key: 'MB', factor: 1024 ** 2 },
  { key: 'GB', factor: 1024 ** 3 },
  { key: 'TB', factor: 1024 ** 4 },
]

export const durationUnits: Unit[] = [
  { key: 's', factor: 1 },
  { key: 'min', factor: 60 },
  { key: 'h', factor: 3600 },
  { key: 'd', factor: 86400 },
]

export function unitsFor(type: string): Unit[] | undefined {
  return type === 'size' ? sizeUnits : type === 'duration' ? durationUnits : undefined
}

/**
 * The largest unit that represents the value exactly. Zero and negative
 * values (which mean "unlimited"/"off" for most attributes) stay in the base
 * unit so they are shown as typed in the documentation.
 */
export function pickUnit(value: number, units: Unit[]): Unit {
  if (!(value > 0)) return units[0]!
  let best = units[0]!
  for (const u of units) {
    if (value % u.factor === 0) best = u
  }
  return best
}

/** Named bits that are set in value, in definition order. */
export function flagsSet(value: number, flags: PageFlag[]): string[] {
  return flags.filter((f) => (value & f.value) === f.value && f.value !== 0).map((f) => f.name)
}

/** Bits of value not covered by any named flag; they are kept on save. */
export function unknownBits(value: number, flags: PageFlag[]): number {
  return flags.reduce((v, f) => v & ~f.value, value)
}

export function flagsValue(names: string[], flags: PageFlag[], keep: number): number {
  return flags.reduce((v, f) => (names.includes(f.name) ? v | f.value : v), keep)
}

/** Values compared the way the server does it for our purposes: blanks ignored. */
export function clean(values: readonly string[] | undefined): string[] {
  return (values ?? []).map((v) => v.trim()).filter((v) => v !== '')
}

export function sameValues(a: readonly string[] | undefined, b: readonly string[] | undefined): boolean {
  const x = clean(a)
  const y = clean(b)
  return x.length === y.length && x.every((v, i) => v === y[i])
}

/** Attributes whose form values differ from the server's. */
export function changedAttrs(form: Record<string, string[]>, server: Record<string, string[]>): string[] {
  return Object.keys(form).filter((a) => !sameValues(form[a], server[a]))
}

/** Locale key prefix for a field's texts (label, help). */
export function fieldKey(pageId: string, f: PageField): string {
  return f.text ? `pages._fields.${f.text}` : `pages.${pageId}.fields.${f.attr}`
}
