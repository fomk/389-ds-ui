import { getCurrentScope, onScopeDispose } from 'vue'

/** The outcome of a load: its value, or stale when a newer load started. */
export type Latest<T> = { stale: false; value: T } | { stale: true }

/**
 * Keeps only the latest of overlapping loads. When the input changes while
 * a request is in flight (another entry, suffix, file), the answer for the
 * old input can arrive after the new one; applying it would show the wrong
 * object. run() aborts the previous request and reports a superseded one as
 * stale, so callers apply only the latest result:
 *
 *   const latest = useLatest()
 *   async function load() {
 *     const r = await latest.run((signal) => call(api.GET('/…', { params, signal })))
 *     if (r.stale) return
 *     data.value = r.value
 *   }
 *
 * Errors of the latest load are thrown; those of superseded loads (including
 * the abort itself) are swallowed. The pending request is aborted when the
 * component goes away.
 */
export function useLatest() {
  let seq = 0
  let controller: AbortController | undefined
  if (getCurrentScope()) onScopeDispose(() => controller?.abort())

  return {
    async run<T>(fn: (signal: AbortSignal) => Promise<T>): Promise<Latest<T>> {
      const mine = ++seq
      controller?.abort()
      const own = new AbortController()
      controller = own
      try {
        const value = await fn(own.signal)
        return mine === seq ? { stale: false, value } : { stale: true }
      } catch (e) {
        if (mine !== seq) return { stale: true }
        throw e
      } finally {
        if (controller === own) controller = undefined
      }
    },
  }
}
