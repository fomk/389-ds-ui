import { describe, expect, it } from 'vitest'
import { ApiError, call } from './client'

const res = (status: number) => new Response(null, { status })

describe('call', () => {
  it('resolves bodiless successes', async () => {
    await expect(call(Promise.resolve({ data: undefined, response: res(204) }))).resolves.toBeUndefined()
  })
  it('returns data', async () => {
    await expect(call(Promise.resolve({ data: { a: 1 }, response: res(200) }))).resolves.toEqual({ a: 1 })
  })
  it('throws ApiError with the problem document', async () => {
    const err = (await call(Promise.resolve({ error: { detail: 'nope', status: 404 }, response: res(404) })).catch((e: unknown) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(404)
    expect(err.message).toBe('nope')
  })
})
