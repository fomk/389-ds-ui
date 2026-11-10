<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSpace,
  NSwitch,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import ConfirmByName from '@/components/ConfirmByName.vue'
import { localizeError } from '@/i18n/errors'

type Backend = Schemas['Backend']

const props = defineProps<{ backend: Backend }>()
const emit = defineEmits<{ changed: [backend: Backend]; deleted: [] }>()

const { t } = useI18n()
const message = useMessage()
const vars = useThemeVars()

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

// Deleting a suffix destroys data: the dialog asks to type the backend name.
const confirming = ref(false)
const deleting = ref(false)
async function remove() {
  deleting.value = true
  try {
    await call(api.DELETE('/api/v1/backends/{name}', { params: { path: { name: props.backend.name } } }))
    message.success(t('db.deleted', { suffix: props.backend.suffix }))
    confirming.value = false
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

  <div class="danger">
    <div class="danger-text">
      <div class="danger-title">{{ t('db.danger') }}</div>
      <div class="danger-desc">{{ t('db.dangerShort') }}</div>
    </div>
    <NButton type="error" ghost @click="confirming = true">{{ t('db.deleteButton') }}</NButton>
  </div>

  <ConfirmByName
    v-model:show="confirming"
    :title="t('db.danger')"
    :warning="t('db.dangerText', { suffix: backend.suffix })"
    :name="backend.name"
    :action="t('db.deleteButtonFinal')"
    :busy="deleting"
    @confirm="remove"
  />
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  max-width: 560px;
  margin-top: 28px;
  padding: 12px 16px;
  border: 1px solid v-bind('vars.errorColor');
  border-radius: 6px;
}

.danger-title {
  font-weight: 600;
}

.danger-desc {
  font-size: 13px;
  color: v-bind('vars.textColor3');
}
</style>
