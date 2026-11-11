import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, call, type Schemas } from '@/api/client'

/** The signed-in user (or that sign-in is off). */
export const useAuthStore = defineStore('auth', () => {
  const me = ref<Schemas['Me']>()
  let loading: Promise<void> | undefined

  const enabled = computed(() => me.value?.enabled ?? false)
  const user = computed(() => me.value?.user)
  /** Allowed into the application: sign-in is off, or someone is signed in. */
  const ready = computed(() => !!me.value && (!me.value.enabled || !!me.value.user))
  /** Operators may look at the administration but not change it. */
  const readOnly = computed(() => me.value?.user?.role === 'operator')

  function load(force = false): Promise<void> {
    if (loading && !force) return loading
    loading = call(api.GET('/api/v1/auth/me')).then((m) => {
      me.value = m
    })
    loading.catch(() => (loading = undefined))
    return loading
  }

  async function login(username: string, password: string) {
    me.value = await call(api.POST('/api/v1/auth/login', { body: { username, password } }))
    loading = Promise.resolve()
  }

  async function logout() {
    try {
      await call(api.POST('/api/v1/auth/logout'))
    } finally {
      me.value = { enabled: true }
      loading = Promise.resolve()
    }
  }

  /** The server said 401: the session is gone. */
  function expired() {
    if (me.value?.enabled) me.value = { enabled: true }
  }

  return { me, enabled, user, ready, readOnly, load, login, logout, expired }
})
