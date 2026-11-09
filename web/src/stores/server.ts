import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, call, type Capabilities, type SystemInfo, type SystemState } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const STATE_POLL_MS = 5000

/** Server identity, deployment capabilities and live state. */
export const useServerStore = defineStore('server', () => {
  const info = ref<SystemInfo>()
  const infoError = ref<LocalizedError>()
  /** The ds-ui API itself did not answer (as opposed to the directory server). */
  const apiDown = ref(false)
  const capabilities = ref<Capabilities>()
  const state = ref<SystemState>()
  let timer: ReturnType<typeof setInterval> | undefined

  const reachable = computed(() => state.value?.ldap === 'ok')

  async function loadInfo() {
    try {
      info.value = await call(api.GET('/api/v1/system/info'))
      infoError.value = undefined
    } catch (e) {
      infoError.value = localizeError(e)
    }
  }

  async function loadCapabilities() {
    capabilities.value = await call(api.GET('/api/v1/system/capabilities'))
  }

  async function refreshState() {
    const wasReachable = reachable.value
    try {
      state.value = await call(api.GET('/api/v1/system/state'))
      apiDown.value = false
    } catch {
      state.value = { ldap: 'unreachable', server: 'unknown', restartRequired: false }
      apiDown.value = true
    }
    // Server came back (e.g. after a restart): identity may have changed.
    if (reachable.value && (!wasReachable || !info.value)) {
      await loadInfo()
    }
  }

  async function start() {
    await Promise.allSettled([loadCapabilities(), refreshState()])
    timer ??= setInterval(refreshState, STATE_POLL_MS)
  }

  function stop() {
    clearInterval(timer)
    timer = undefined
  }

  return { info, infoError, apiDown, capabilities, state, reachable, start, stop, loadInfo, refreshState }
})
