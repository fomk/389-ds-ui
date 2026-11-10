<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSpace,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError } from '@/i18n/errors'

type Backend = Schemas['Backend']

const props = defineProps<{ backend: Backend }>()
const emit = defineEmits<{ changed: [backend: Backend]; deleted: [] }>()

const { t } = useI18n()
const message = useMessage()

const MB = 1024 * 1024
const form = ref({ readOnly: false, requireIndex: false, state: 'backend', referral: '', cacheMB: 0, dnCacheMB: 0 })
function reset() {
  const b = props.backend
  form.value = {
    readOnly: b.readOnly,
    requireIndex: b.requireIndex,
    state: b.state || 'backend',
    referral: b.referral ?? '',
    cacheMB: Math.round((b.cacheMemSize / MB) * 10) / 10,
    dnCacheMB: Math.round((b.dnCacheMemSize / MB) * 10) / 10,
  }
}
watch(() => props.backend, reset, { immediate: true })

const states = computed(() =>
  (['backend', 'disabled', 'referral', 'referral on update'] as const).map((s) => ({ value: s, label: t(`db.states.${s}`) })),
)
const saving = ref(false)

async function save() {
  saving.value = true
  try {
    const b = props.backend
    const f = form.value
    const body: Schemas['BackendPatch'] = {}
    if (f.readOnly !== b.readOnly) body.readOnly = f.readOnly
    if (f.requireIndex !== b.requireIndex) body.requireIndex = f.requireIndex
    if (f.state !== b.state) body.state = f.state as Schemas['BackendPatch']['state']
    if (f.referral !== (b.referral ?? '')) body.referral = f.referral
    const cache = Math.round(f.cacheMB * MB)
    const dnCache = Math.round(f.dnCacheMB * MB)
    if (Math.abs(cache - b.cacheMemSize) > MB / 20) body.cacheMemSize = cache
    if (Math.abs(dnCache - b.dnCacheMemSize) > MB / 20) body.dnCacheMemSize = dnCache
    const updated = await call(api.PATCH('/api/v1/backends/{name}', { params: { path: { name: b.name } }, body }))
    message.success(t('db.saved'))
    emit('changed', updated)
  } catch (e) {
    message.error(`${t('db.saveError')}: ${localizeError(e).summary}`)
  } finally {
    saving.value = false
  }
}

// Deleting a suffix destroys data: require typing the backend name.
const confirmName = ref('')
const deleting = ref(false)
async function remove() {
  deleting.value = true
  try {
    await call(api.DELETE('/api/v1/backends/{name}', { params: { path: { name: props.backend.name } } }))
    message.success(t('db.deleted', { suffix: props.backend.suffix }))
    emit('deleted')
  } catch (e) {
    message.error(`${t('db.deleteError')}: ${localizeError(e).summary}`)
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <NAlert v-if="!backend.rootExists" type="warning" class="block">
    {{ t('db.rootMissing', { suffix: backend.suffix }) }}
  </NAlert>

  <NForm label-placement="left" label-width="auto" class="form" @submit.prevent="save">
    <NFormItem :label="t('db.state')">
      <NSelect v-model:value="form.state" :options="states" class="state" />
    </NFormItem>
    <NFormItem v-if="form.state.startsWith('referral')" :label="t('db.referral')">
      <NInput v-model:value="form.referral" :input-props="{ class: 'mono', spellcheck: false }" placeholder="ldap://other.example.org:389/" />
    </NFormItem>
    <NFormItem :label="t('db.readOnly')" :feedback="t('db.readOnlyHint')">
      <NSwitch v-model:value="form.readOnly" />
    </NFormItem>
    <NFormItem :label="t('db.requireIndex')" :feedback="t('db.requireIndexHint')">
      <NSwitch v-model:value="form.requireIndex" />
    </NFormItem>
    <NFormItem :label="t('db.entryCache')">
      <NInputNumber v-model:value="form.cacheMB" :min="0.5" :step="64" class="num">
        <template #suffix>{{ t('db.mb') }}</template>
      </NInputNumber>
    </NFormItem>
    <NFormItem :label="t('db.dnCache')">
      <NInputNumber v-model:value="form.dnCacheMB" :min="0.5" :step="16" class="num">
        <template #suffix>{{ t('db.mb') }}</template>
      </NInputNumber>
    </NFormItem>
    <NSpace>
      <NButton type="primary" attr-type="submit" :loading="saving">{{ t('db.save') }}</NButton>
      <NButton @click="reset">{{ t('edit.cancel') }}</NButton>
    </NSpace>
  </NForm>

  <NCard size="small" :title="t('db.danger')" class="danger">
    <p>{{ t('db.dangerText', { suffix: backend.suffix }) }}</p>
    <NSpace align="center">
      <NInput v-model:value="confirmName" :placeholder="t('db.typeToConfirm', { name: backend.name })" class="confirm" />
      <NButton type="error" :disabled="confirmName !== backend.name" :loading="deleting" @click="remove">
        {{ t('actions.delete') }}
      </NButton>
    </NSpace>
  </NCard>
</template>

<style scoped>
.block {
  margin-bottom: 16px;
}

.form {
  max-width: 560px;
}

.state {
  width: 280px;
}

.num {
  width: 200px;
}

.danger {
  margin-top: 28px;
  max-width: 560px;
}

.confirm {
  width: 260px;
}
</style>
