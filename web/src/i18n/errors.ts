import { ApiError } from '@/api/client'
import { i18n } from './index'

/**
 * The API returns technical English details (RFC 9457 + ldapCode). The UI
 * shows a localized summary derived from the LDAP result code or HTTP status
 * and keeps the technical detail as secondary text.
 */
const ldapResultNames: Record<number, string> = {
  32: 'noSuchObject',
  49: 'invalidCredentials',
  50: 'insufficientAccessRights',
  51: 'busy',
  52: 'unavailable',
  53: 'unwillingToPerform',
  19: 'constraintViolation',
  21: 'invalidAttributeSyntax',
  65: 'objectClassViolation',
  64: 'namingViolation',
  34: 'invalidDNSyntax',
  68: 'entryAlreadyExists',
  66: 'notAllowedOnNonLeaf',
}

export interface LocalizedError {
  summary: string
  detail?: string
}

export function localizeError(e: unknown): LocalizedError {
  const { t, te } = i18n.global
  if (e instanceof ApiError) {
    const name = e.problem.ldapCode !== undefined ? ldapResultNames[e.problem.ldapCode] : undefined
    const key = name ? `errors.ldap.${name}` : undefined
    let summary: string
    if (key && te(key)) summary = t(key)
    else if (e.status === 401) summary = t('errors.http.unauthorized')
    else if (e.status === 403 && !name) summary = t('errors.http.forbidden')
    else if (e.status === 429) summary = t('errors.http.tooMany')
    else if (e.status === 503) summary = t('errors.http.unavailable')
    else if (e.status === 501) summary = t('errors.http.notImplemented')
    else summary = t('errors.http.generic', { status: e.status })
    return { summary, detail: e.problem.detail }
  }
  // fetch() itself failed: the ds-ui API is down or unreachable.
  if (e instanceof TypeError) return { summary: t('errors.network'), detail: e.message }
  return { summary: String(e) }
}
