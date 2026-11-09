/**
 * Naive UI ships its own strings (date pickers, pagination, empty states).
 * Map our locale codes to Naive UI locales here. A language without an entry
 * still works — Naive UI components then fall back to English.
 * Available locales: https://www.naiveui.com/en-US/os-theme/docs/i18n
 */
import { dateEnUS, dateRuRU, enUS, ruRU, type NDateLocale, type NLocale } from 'naive-ui'

export const naiveLocales: Record<string, { locale: NLocale; dateLocale: NDateLocale }> = {
  en: { locale: enUS, dateLocale: dateEnUS },
  ru: { locale: ruRU, dateLocale: dateRuRU },
}

export function naiveLocaleFor(code: string) {
  return naiveLocales[code] ?? naiveLocales[code.split('-')[0]!] ?? naiveLocales.en!
}
