/**
 * Internationalisation.
 *
 * Adding a language = adding one file: src/i18n/locales/<code>.json
 * (code is a BCP 47 language tag such as "de" or "pt-BR"). It is discovered
 * automatically. Optionally register Naive UI's own strings in ./naive.ts.
 * See docs/i18n.md.
 */
import { computed, watch } from 'vue'
import { createI18n } from 'vue-i18n'
import type en from './locales/en.json'

/** English is the reference locale: its keys define the message schema. */
export type MessageSchema = typeof en

declare module 'vue-i18n' {
  // Key autocompletion for t(). vue-i18n still accepts any string, so key
  // existence is enforced by scripts/check-locales.mjs (npm run check:locales).
  export interface DefineLocaleMessage extends MessageSchema {}
}

export const FALLBACK_LOCALE = 'en'
const STORAGE_KEY = 'dsui.locale'

const modules = import.meta.glob<MessageSchema>('./locales/*.json', { eager: true, import: 'default' })

const messages: Record<string, MessageSchema> = {}
for (const [path, msgs] of Object.entries(modules)) {
  const code = path.match(/\/([^/]+)\.json$/)?.[1]
  if (code) messages[code] = msgs
}

export interface LocaleInfo {
  code: string
  /** Native name of the language, from meta.name of the locale file. */
  name: string
}

export const availableLocales: LocaleInfo[] = Object.entries(messages)
  .map(([code, m]) => ({ code, name: m.meta?.name ?? code }))
  .sort((a, b) => a.name.localeCompare(b.name))

/**
 * Plural selection for any language via Intl.PluralRules.
 * Plural forms in messages are separated by "|" and follow the CLDR category
 * order zero → one → two → few → many → other, keeping only the categories the
 * language has. English: "one | other"; Russian: "one | few | many | other".
 * Missing trailing forms fall back to the last one given.
 */
const CATEGORY_ORDER: Intl.LDMLPluralRule[] = ['zero', 'one', 'two', 'few', 'many', 'other']

function pluralRule(locale: string) {
  const rules = new Intl.PluralRules(locale)
  const present = rules.resolvedOptions().pluralCategories
  const categories = CATEGORY_ORDER.filter((c) => present.includes(c))
  return (choice: number, choicesLength: number) => {
    const index = categories.indexOf(rules.select(Math.abs(choice)))
    return Math.min(index < 0 ? choicesLength - 1 : index, choicesLength - 1)
  }
}

function readStored(): string | undefined {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? undefined
  } catch {
    return undefined
  }
}

function store(code: string) {
  try {
    localStorage.setItem(STORAGE_KEY, code)
  } catch {
    // Private mode or blocked storage: the choice just is not remembered.
  }
}

/** Picks the best available locale for a list of preferred language tags. */
export function negotiateLocale(preferred: readonly string[], available = Object.keys(messages)): string {
  for (const tag of preferred) {
    const exact = available.find((c) => c.toLowerCase() === tag.toLowerCase())
    if (exact) return exact
    const base = tag.split('-')[0]!.toLowerCase()
    const byBase = available.find((c) => c.toLowerCase() === base || c.toLowerCase().startsWith(base + '-'))
    if (byBase) return byBase
  }
  return FALLBACK_LOCALE
}

const initial = (() => {
  const stored = readStored()
  if (stored && stored in messages) return stored
  return negotiateLocale(navigator.languages?.length ? navigator.languages : [navigator.language])
})()

export const i18n = createI18n<[MessageSchema], string, false>({
  legacy: false,
  locale: initial,
  fallbackLocale: FALLBACK_LOCALE,
  messages,
  pluralRules: Object.fromEntries(Object.keys(messages).map((code) => [code, pluralRule(code)])),
  missingWarn: import.meta.env.DEV,
  fallbackWarn: import.meta.env.DEV,
})

/** Current locale code (reactive). */
export const currentLocale = computed(() => i18n.global.locale.value)

export function setLocale(code: string) {
  if (!(code in messages)) return
  i18n.global.locale.value = code
  store(code)
}

watch(
  currentLocale,
  (code) => {
    document.documentElement.lang = code
  },
  { immediate: true },
)
