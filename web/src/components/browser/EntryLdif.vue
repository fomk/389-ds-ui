<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NInput, NSpace, useMessage, useThemeVars } from 'naive-ui'
import { api, ApiError, call, fetchExport, type Entry } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const props = defineProps<{ entry: Entry }>()
const emit = defineEmits<{ saved: [entry: Entry]; reload: [] }>()

const { t } = useI18n()
const message = useMessage()
const vars = useThemeVars()

const text = ref('')
const original = ref('')
const editing = ref(false)
const saving = ref(false)
const error = ref<LocalizedError & { conflict?: boolean }>()

async function load() {
  error.value = undefined
  try {
    // Drop the "version: 1" header: a single record reads better without it.
    const ldif = (await fetchExport({ base: props.entry.dn, scope: 'base' })).replace(/^version: 1\n\n/, '')
    text.value = original.value = ldif.trimEnd() + '\n'
  } catch (e) {
    error.value = localizeError(e)
  }
}

onMounted(load)

async function save() {
  if (text.value === original.value) {
    editing.value = false
    return
  }
  saving.value = true
  error.value = undefined
  try {
    const updated = await call(
      api.PUT('/api/v1/entries/ldif', {
        params: { query: { dn: props.entry.dn }, header: { 'If-Match': props.entry.etag } },
        body: { ldif: text.value },
      }),
    )
    message.success(t('ldif.saved'))
    editing.value = false
    emit('saved', updated)
  } catch (e) {
    error.value = { ...localizeError(e), conflict: e instanceof ApiError && e.status === 412 }
  } finally {
    saving.value = false
  }
}

function cancel() {
  text.value = original.value
  editing.value = false
  error.value = undefined
}
</script>

<template>
  <NAlert v-if="error" type="error" :title="error.conflict ? undefined : t('ldif.error')" class="block">
    <template v-if="error.conflict">
      {{ t('edit.conflict') }}
      <NButton size="small" class="reload" @click="emit('reload')">{{ t('edit.reload') }}</NButton>
    </template>
    <template v-else>
      {{ error.summary }}
      <div v-if="error.detail" class="mono muted">{{ error.detail }}</div>
    </template>
  </NAlert>

  <NInput
    v-model:value="text"
    type="textarea"
    :readonly="!editing"
    :autosize="{ minRows: 8, maxRows: 40 }"
    :input-props="{ class: 'mono ldif', spellcheck: false }"
  />
  <div class="footer">
    <span class="muted note">{{ t('ldif.passwordNote') }}</span>
    <NSpace>
      <NButton v-if="!editing" size="small" @click="editing = true">{{ t('ldif.edit') }}</NButton>
      <template v-else>
        <NButton size="small" @click="cancel">{{ t('edit.cancel') }}</NButton>
        <NButton size="small" type="primary" :loading="saving" @click="save">{{ t('ldif.save') }}</NButton>
      </template>
    </NSpace>
  </div>
</template>

<style scoped>
.block {
  margin-bottom: 12px;
}

.reload {
  margin-left: 12px;
}

.footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-top: 10px;
}

.note {
  font-size: 12px;
}

.muted {
  color: v-bind('vars.textColor3');
}

:deep(.ldif) {
  font-size: 12.5px;
  line-height: 1.5;
  white-space: pre;
}
</style>
