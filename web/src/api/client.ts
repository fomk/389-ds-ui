import createClient, { type Middleware } from 'openapi-fetch'
import type { components, paths } from './schema'

export type Schemas = components['schemas']
export type ServerInfo = Schemas['ServerInfo']
export type SystemInfo = Schemas['SystemInfo']
export type SystemState = Schemas['SystemState']
export type Capabilities = Schemas['Capabilities']

/** RFC 9457 problem document returned by the API on errors. */
export interface Problem {
  title?: string
  status?: number
  detail?: string
  ldapCode?: number
  ldapMatchedDN?: string
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly problem: Problem,
  ) {
    super(problem.detail || problem.title || `HTTP ${status}`)
  }
}

// Mutating requests must carry this header (CSRF guard on the server).
const csrf: Middleware = {
  onRequest({ request }) {
    if (!['GET', 'HEAD', 'OPTIONS'].includes(request.method)) {
      request.headers.set('X-Requested-With', 'ds-ui')
    }
    return request
  },
}

export const api = createClient<paths>({ baseUrl: '' })
api.use(csrf)

/** Unwraps an openapi-fetch result, throwing ApiError on failure. */
export async function call<T>(p: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  const { data, error, response } = await p
  if (error !== undefined || data === undefined) {
    throw new ApiError(response.status, (error as Problem) ?? { status: response.status })
  }
  return data
}
