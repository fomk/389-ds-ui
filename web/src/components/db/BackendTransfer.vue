<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NCheckbox, NForm, NFormItem, NInput, NSelect, NSpace, useMessage } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'
import { useTasksStore } from '@/stores/tasks'
import TaskList from '@/components/TaskList.vue'

const props = defineProps<{ backend: Schemas['Backend'] }>()

const { t } = useI18n()
const message = useMessage()
const server = useServerStore()
const tasks = useTasksStore()

const exportFile = ref('')
const replication = ref(false)
const importFile = ref<string | null>(null)
const confirm = ref('')
const files = ref<Schemas['Info'][]>([])
const canSeeFiles = computed(() => server.capabilities?.files ?? false)

onMounted(async () => {
  if (canSeeFiles.value) files.value = await call(api.GET('/api/v1/ldif-files')).catch(() => [])
})
const fileOptions = computed(() => files.value.map((f) => ({ label: f.name, value: f.name })))

async function startExport() {
  try {
    tasks.upsert(
      await call(api.POST('/api/v1/backends/{name}/export', {
        params: { path: { name: props.backend.name } },
        body: { file: exportFile.value.trim() || undefined, replication: replication.value },
      })),
    )
    message.info(t('transfer.exportStarted'))
  } catch (e) {
    message.error(`${t('transfer.error')}: ${localizeError(e).summary}`)
  }
}

async function startImport() {
  if (!importFile.value) return
  try {
    tasks.upsert(
      await call(api.POST('/api/v1/backends/{name}/import', {
        params: { path: { name: props.backend.name } },
        body: { file: importFile.value },
      })),
    )
    confirm.value = ''
    message.info(t('transfer.importStarted'))
  } catch (e) {
    message.error(`${t('transfer.error')}: ${localizeError(e).summary}`)
  }
}
</script>

<template>
  <NCard size="small" :title="t('transfer.export')" class="block">
    <p class="hint">{{ t('transfer.exportHint') }}</p>
    <NForm label-placement="left" label-width="auto" @submit.prevent="startExport">
      <NFormItem :label="t('transfer.file')">
        <NInput v-model:value="exportFile" :placeholder="`${backend.name}_<time>.ldif`" :input-props="{ class: 'mono' }" class="file" />
      </NFormItem>
      <NCheckbox v-model:checked="replication">{{ t('transfer.replication') }}</NCheckbox>
      <div class="actions"><NButton type="primary" attr-type="submit">{{ t('transfer.export') }}</NButton></div>
    </NForm>
  </NCard>

  <NCard size="small" :title="t('transfer.import')" class="block">
    <NAlert type="warning" class="hint-alert">{{ t('transfer.importHint') }}</NAlert>
    <NAlert v-if="!canSeeFiles" type="info">{{ t('ldifFiles.noFiles') }}</NAlert>
    <NSpace v-else vertical>
      <NSelect v-model:value="importFile" :options="fileOptions" :placeholder="t('transfer.chooseFile')" class="file" />
      <NInput v-model:value="confirm" :placeholder="t('transfer.importConfirm', { name: backend.name })" class="file" />
      <NButton type="error" :disabled="!importFile || confirm !== backend.name" @click="startImport">{{ t('transfer.import') }}</NButton>
    </NSpace>
  </NCard>

  <TaskList :target="undefined" :limit="5" />
</template>

<style scoped>
.block {
  margin-bottom: 16px;
  max-width: 640px;
}

.hint {
  margin-top: 0;
  font-size: 13px;
  opacity: 0.8;
}

.hint-alert {
  margin-bottom: 12px;
}

.file {
  max-width: 420px;
}

.actions {
  margin-top: 12px;
}
</style>
