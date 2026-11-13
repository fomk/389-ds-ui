<script setup lang="ts">
/** Sign-in with a directory account. */
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, useThemeVars } from 'naive-ui'
import LocaleSwitcher from '@/components/LocaleSwitcher.vue'
import { ApiError } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const vars = useThemeVars()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const busy = ref(false)
const error = ref<LocalizedError>()

async function submit() {
  if (!username.value.trim() || !password.value) return
  busy.value = true
  error.value = undefined
  try {
    await auth.login(username.value.trim(), password.value)
    password.value = ''
    const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') ? route.query.next : '/'
    await router.replace(next)
  } catch (e) {
    const known: Record<number, string> = { 401: 'login.invalid', 403: 'login.noAccess', 429: 'login.tooMany' }
    error.value = e instanceof ApiError && known[e.status] ? { summary: t(known[e.status]!) } : localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="page">
    <div class="corner"><LocaleSwitcher /></div>
    <NCard class="card" :bordered="true">
      <div class="brand">
        <img src="/favicon.svg" alt="" width="28" height="28" />
        <span>{{ t('app.title') }}</span>
      </div>
      <h1 class="title">{{ t('login.title') }}</h1>
      <NForm label-placement="top" @submit.prevent="submit">
        <NFormItem :label="t('login.username')">
          <NInput
            v-model:value="username"
            :placeholder="t('login.usernameHint')"
            :input-props="{ autocomplete: 'username', autofocus: true, spellcheck: false, name: 'username', 'aria-label': t('login.username') }"
            :disabled="busy"
          />
        </NFormItem>
        <NFormItem :label="t('login.password')">
          <NInput
            v-model:value="password"
            type="password"
            :placeholder="t('login.passwordHint')"
            show-password-on="click"
            :input-props="{ autocomplete: 'current-password', name: 'password', 'aria-label': t('login.password') }"
            :disabled="busy"
          />
        </NFormItem>
        <NAlert v-if="error" type="error" :show-icon="false" class="error">{{ error.summary }}<div v-if="error.detail" class="detail">{{ error.detail }}</div></NAlert>
        <NButton type="primary" attr-type="submit" block :loading="busy" :disabled="!username.trim() || !password">
          {{ t('login.submit') }}
        </NButton>
      </NForm>
    </NCard>
  </main>
</template>

<style scoped>
.page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  box-sizing: border-box;
  background: v-bind('vars.bodyColor');
}

.corner {
  position: fixed;
  top: 12px;
  right: 16px;
}

.card {
  width: min(400px, 100%);
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 600;
}

.title {
  margin: 18px 0 16px;
  font-size: 20px;
  font-weight: 600;
}

.error {
  margin-bottom: 12px;
}

.detail {
  font-size: 12px;
  opacity: 0.8;
}
</style>
