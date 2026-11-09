<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCheckbox, NForm, NFormItem, NInput, NModal, NSpace, useMessage } from 'naive-ui'
import { api, call, type Entry } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const props = defineProps<{ entry: Entry }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()
const message = useMessage()
const password = ref('')
const confirm = ref('')
const generate = ref(false)
const generated = ref('')
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (v) {
    password.value = confirm.value = generated.value = ''
    generate.value = false
    error.value = undefined
  }
})

const mismatch = computed(() => !generate.value && confirm.value !== '' && password.value !== confirm.value)
const canApply = computed(() => generate.value || (password.value !== '' && password.value === confirm.value))

async function run() {
  busy.value = true
  error.value = undefined
  try {
    const res = await call(
      api.POST('/api/v1/entries/password', {
        body: { dn: props.entry.dn, password: generate.value ? undefined : password.value },
      }),
    )
    message.success(t('password.done'))
    emit('changed')
    if (res.generated) generated.value = res.generated
    else show.value = false
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}

async function copy() {
  try {
    await navigator.clipboard.writeText(generated.value)
    message.success(t('browser.copied'))
  } catch {
    // Clipboard unavailable: the password stays visible for manual copying.
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="t('password.title')" :style="{ width: 'min(520px, 92vw)' }" :mask-closable="!busy">
    <template v-if="generated">
      <p>{{ t('password.generated') }}</p>
      <NSpace align="center">
        <code class="generated">{{ generated }}</code>
        <NButton size="small" @click="copy">{{ t('password.copy') }}</NButton>
      </NSpace>
    </template>
    <NForm v-else label-placement="top" @submit.prevent="canApply && run()">
      <p class="mono dn">{{ entry.dn }}</p>
      <NCheckbox v-model:checked="generate" class="gen">{{ t('password.generate') }}</NCheckbox>
      <template v-if="!generate">
        <NFormItem :label="t('password.new')">
          <NInput v-model:value="password" type="password" show-password-on="click" :input-props="{ autocomplete: 'new-password' }" />
        </NFormItem>
        <NFormItem :label="t('password.confirm')" :validation-status="mismatch ? 'error' : undefined" :feedback="mismatch ? t('password.mismatch') : undefined">
          <NInput v-model:value="confirm" type="password" show-password-on="click" :input-props="{ autocomplete: 'new-password' }" />
        </NFormItem>
      </template>
    </NForm>
    <NAlert v-if="error" type="error" :title="t('password.error')" class="top">
      {{ error.summary }}
      <div v-if="error.detail" class="mono">{{ error.detail }}</div>
    </NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton v-if="generated" type="primary" @click="show = false">OK</NButton>
        <template v-else>
          <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
          <NButton type="primary" :loading="busy" :disabled="!canApply" @click="run">{{ t('password.apply') }}</NButton>
        </template>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.dn {
  overflow-wrap: anywhere;
  margin-top: 0;
}

.gen {
  margin-bottom: 16px;
}

.generated {
  font-size: 15px;
  padding: 4px 8px;
  user-select: all;
}

.top {
  margin-top: 12px;
}
</style>
