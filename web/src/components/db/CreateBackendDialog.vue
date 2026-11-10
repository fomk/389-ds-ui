<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCheckbox, NForm, NFormItem, NInput, NModal, NSpace, useMessage } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ created: [backend: Schemas['Backend']] }>()

const { t } = useI18n()
const message = useMessage()
const name = ref('')
const suffix = ref('')
const createRoot = ref(true)
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (v) {
    name.value = suffix.value = ''
    createRoot.value = true
    error.value = undefined
  }
})

async function run() {
  busy.value = true
  error.value = undefined
  try {
    const b = await call(
      api.POST('/api/v1/backends', { body: { name: name.value.trim(), suffix: suffix.value.trim(), createRoot: createRoot.value } }),
    )
    message.success(t('db.created', { suffix: b.suffix }))
    show.value = false
    emit('created', b)
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="t('db.create')" :style="{ width: 'min(520px, 92vw)' }" :mask-closable="!busy">
    <NForm label-placement="top" @submit.prevent="run">
      <NFormItem :label="t('db.suffix')" :feedback="t('db.suffixHint')">
        <NInput v-model:value="suffix" :input-props="{ class: 'mono', spellcheck: false }" placeholder="dc=example,dc=org" />
      </NFormItem>
      <NFormItem :label="t('db.name')" :feedback="t('db.nameHint')">
        <NInput v-model:value="name" :input-props="{ class: 'mono', spellcheck: false }" placeholder="exampleroot" />
      </NFormItem>
      <NCheckbox v-model:checked="createRoot">{{ t('db.createRoot') }}</NCheckbox>
    </NForm>
    <NAlert v-if="error" type="error" :title="t('db.createError')" class="top">
      {{ error.summary }}
      <div v-if="error.detail" class="mono">{{ error.detail }}</div>
    </NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="busy" :disabled="!name.trim() || !suffix.trim()" @click="run">{{ t('create.create') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.top {
  margin-top: 12px;
}
</style>
