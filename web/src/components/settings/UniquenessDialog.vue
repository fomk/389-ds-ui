<script setup lang="ts">
/** Another Attribute Uniqueness instance: a plug-in entry of its own, created disabled. */
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NDynamicTags, NForm, NFormItem, NInput, NModal, NSpace } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ created: [plugin: Schemas['Plugin']] }>()

const { t } = useI18n()
const name = ref('')
const attributes = ref<string[]>([])
const subtrees = ref<string[]>([])
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (v) {
    name.value = ''
    attributes.value = []
    subtrees.value = []
    error.value = undefined
  }
})

async function run() {
  busy.value = true
  error.value = undefined
  try {
    const p = await call(
      api.POST('/api/v1/plugins/uniqueness', {
        body: { name: name.value.trim(), attributes: attributes.value, subtrees: subtrees.value.length ? subtrees.value : undefined },
      }),
    )
    show.value = false
    emit('created', p)
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="t('uniqueness.add')" :style="{ width: 'min(560px, 92vw)' }" :mask-closable="!busy">
    <p class="hint">{{ t('uniqueness.addHint') }}</p>
    <NForm label-placement="top" @submit.prevent="run">
      <NFormItem :label="t('uniqueness.name')">
        <NInput v-model:value="name" placeholder="mail uniqueness" />
      </NFormItem>
      <NFormItem :label="t('uniqueness.attributes')">
        <NDynamicTags v-model:value="attributes" />
      </NFormItem>
      <NFormItem :label="t('uniqueness.subtrees')" :feedback="t('uniqueness.subtreesHint')">
        <NDynamicTags v-model:value="subtrees" />
      </NFormItem>
    </NForm>
    <NAlert v-if="error" type="error">{{ error.summary }}<div v-if="error.detail" class="mono">{{ error.detail }}</div></NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="busy" :disabled="!name.trim() || !attributes.length" @click="run">{{ t('create.create') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 12.5px;
  opacity: 0.75;
}
</style>
