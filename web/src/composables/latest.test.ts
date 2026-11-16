import { describe, expect, it } from 'vitest'
import { useLatest } from './latest'

/** A promise resolved by hand, and whether its signal was aborted. */
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => ((resolve = res), (reject = rej)))
  return { promise, resolve, reject }
}

describe('useLatest', () => {
  it('applies only the latest of two overlapping loads, whatever order they finish in', async () => {
    const latest = useLatest()
    const a = deferred<string>()
    const b = deferred<string>()
    let aSignal: AbortSignal | undefined
    const first = latest.run((s) => ((aSignal = s), a.promise))
    const second = latest.run(() => b.promise)
    expect(aSignal?.aborted).toBe(true) // the old request is cancelled
    b.resolve('B')
    a.resolve('A') // the old answer arrives last
    expect(await second).toEqual({ stale: false, value: 'B' })
    expect(await first).toEqual({ stale: true })
  })

  it('throws the error of the latest load and swallows superseded ones', async () => {
    const latest = useLatest()
    const a = deferred<string>()
    const first = latest.run(() => a.promise)
    const second = latest.run(() => Promise.reject(new Error('B failed')))
    a.reject(new DOMException('aborted', 'AbortError'))
    await expect(second).rejects.toThrow('B failed')
    expect(await first).toEqual({ stale: true })
  })

  it('a single load is never stale', async () => {
    const latest = useLatest()
    expect(await latest.run(async () => 42)).toEqual({ stale: false, value: 42 })
    expect(await latest.run(async () => 43)).toEqual({ stale: false, value: 43 })
  })
})
