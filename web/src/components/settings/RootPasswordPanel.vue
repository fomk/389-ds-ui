<script setup lang="ts">
/** Change of the Directory Manager password (POST /root-password). */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, useDialog, useMessage } from 'naive-ui'
import { api, call } from '@/api/client'
import { localizeError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'

const MIN = 8

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()

const password = ref('')
const repeat = ref('')
const busy = ref(false)
/** ds-ui itself binds with the changed password: its secret is stale now. */
const secretStale = ref(false)

// Simple bind: ds-ui may bind as Directory Manager with this password.
const bindsWithPassword = computed(() => server.capabilities?.ldapAuth === 'simple')

const tooShort = computed(() => password.value.length > 0 && password.value.length < MIN)
const mismatch = computed(() => repeat.value.length > 0 && repeat.value !== password.value)
const valid = computed(() => password.value.length >= MIN && password.value === repeat.value && !password.value.startsWith('{'))

function confirm(): Promise<boolean> {
  if (!bindsWithPassword.value) return Promise.resolve(true)
  return new Promise((resolve) => {
    dialog.warning({
      title: t('rootPassword.confirmTitle'),
      content: t('rootPassword.confirmSimple'),
      positiveText: t('rootPassword.change'),
      negativeText: t('edit.cancel'),
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false),
    })
  })
}

async function submit() {
  if (!valid.value || !(await confirm())) return
  busy.value = true
  try {
    const res = await call(api.POST('/api/v1/root-password', { body: { password: password.value } }))
    password.value = repeat.value = ''
    secretStale.value = res.credentialsUpdated
    message.success(t('rootPassword.changed', { dn: res.rootDN }))
  } catch (e) {
    const err = localizeError(e)
    message.error(`${t('rootPassword.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NCard size="small" :title="t('pages.root-password.title')">
    <p class="description">{{ t('pages.root-password.description') }}</p>
    <NAlert v-if="secretStale" type="warning" class="block" :title="t('rootPassword.secretTitle')">
      {{ t('rootPassword.secretText') }}
      <code>DSUI_LDAP_BIND_PASSWORD_FILE</code>
    </NAlert>
    <NForm class="form" label-placement="top" @submit.prevent="submit">
      <NFormItem
        :label="t('rootPassword.new')"
        :validation-status="tooShort ? 'error' : undefined"
        :feedback="tooShort ? t('rootPassword.tooShort', { n: MIN }) : undefined"
      >
        <NInput
          v-model:value="password"
          type="password"
          show-password-on="click"
          :input-props="{ autocomplete: 'new-password' }"
        />
      </NFormItem>
      <NFormItem
        :label="t('rootPassword.repeat')"
        :validation-status="mismatch ? 'error' : undefined"
        :feedback="mismatch ? t('rootPassword.mismatch') : undefined"
      >
        <NInput
          v-model:value="repeat"
          type="password"
          show-password-on="click"
          :input-props="{ autocomplete: 'new-password' }"
        />
      </NFormItem>
      <NButton type="primary" attr-type="submit" :disabled="!valid" :loading="busy">{{ t('rootPassword.change') }}</NButton>
    </NForm>
  </NCard>
</template>

<style scoped>
.description {
  margin: 0 0 16px;
  max-width: 720px;
}

.block {
  margin-bottom: 16px;
}

.form {
  max-width: 420px;
}
</style>
