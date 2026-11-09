<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCheckbox, NInput, NModal, NSpace, NTable, useThemeVars } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ applied: [] }>()

const { t } = useI18n()
const vars = useThemeVars()
const text = ref('')
const stopOnError = ref(true)
const busy = ref(false)
const error = ref<LocalizedError>()
const summary = ref<Schemas['ApplySummary']>()

watch(show, (v) => {
  if (v) {
    summary.value = undefined
    error.value = undefined
  }
})

function loadFile() {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = '.ldif,.txt,text/plain'
  input.onchange = async () => {
    const file = input.files?.[0]
    if (file) text.value = await file.text()
  }
  input.click()
}

async function run() {
  busy.value = true
  error.value = undefined
  summary.value = undefined
  try {
    summary.value = await call(api.POST('/api/v1/ldif/apply', { body: { ldif: text.value, stopOnError: stopOnError.value } }))
    if (summary.value.applied) emit('applied')
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="t('ldif.importTitle')" :style="{ width: 'min(860px, 94vw)' }" :mask-closable="!busy">
    <p class="hint">{{ t('ldif.importHint') }}</p>
    <NInput
      v-model:value="text"
      type="textarea"
      :autosize="{ minRows: 10, maxRows: 24 }"
      :input-props="{ class: 'mono', spellcheck: false }"
      placeholder="dn: ou=New,dc=example,dc=com&#10;objectClass: organizationalUnit&#10;ou: New"
    />
    <NSpace class="opts" align="center">
      <NButton size="small" @click="loadFile">{{ t('ldif.loadFile') }}</NButton>
      <NCheckbox v-model:checked="stopOnError">{{ t('ldif.stopOnError') }}</NCheckbox>
    </NSpace>

    <NAlert v-if="error" type="error" :title="t('ldif.error')" class="block">
      {{ error.summary }}
      <div v-if="error.detail" class="mono">{{ error.detail }}</div>
    </NAlert>

    <template v-if="summary">
      <NAlert :type="summary.failed ? 'warning' : 'success'" class="block">
        {{ t('ldif.summary', { applied: summary.applied, failed: summary.failed }) }}
      </NAlert>
      <div class="results">
      <NTable size="small" :single-line="false">
        <tbody>
          <tr v-for="(r, i) in summary.results" :key="i">
            <td class="op">{{ r.op }}</td>
            <td class="mono">{{ r.dn }}</td>
            <td :class="r.error ? 'err' : 'ok'">{{ r.error || 'OK' }}</td>
          </tr>
        </tbody>
      </NTable>
      </div>
    </template>

    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ summary ? 'OK' : t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="busy" :disabled="!text.trim()" @click="run">{{ t('ldif.apply') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.hint {
  margin-top: 0;
  font-size: 13px;
  opacity: 0.8;
}

.opts {
  margin: 10px 0;
}

.block {
  margin-top: 12px;
}

.results {
  margin-top: 10px;
  max-height: 280px;
  overflow: auto;
}

.op {
  width: 70px;
}

.err {
  color: v-bind('vars.errorColor');
}

.ok {
  color: v-bind('vars.successColor');
}
</style>
