import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './styles.css'

import App from './App.vue'
import { router } from './router'
import { i18n } from './i18n'

import { onUnauthorized } from './api/client'
import { useAuthStore } from './stores/auth'

const app = createApp(App).use(createPinia()).use(i18n).use(router)
// A request answered 401: the session ended; go to the sign-in page.
onUnauthorized(() => {
  useAuthStore().expired()
  const here = router.currentRoute.value
  if (here.name !== 'login') void router.replace({ name: 'login', query: { next: here.fullPath } })
})
app.mount('#app')
