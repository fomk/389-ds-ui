<script setup lang="ts">
/** A plug-in maintenance task (fixup, rebuild): parameters dialog and start. */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NFormItem, NIcon, NInput, NModal, NRadioButton, NRadioGroup, NSpace, useMessage } from 'naive-ui'
import { Play } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'
import { useTasksStore } from '@/stores/tasks'

const props = defineProps<{ task: Schemas['PluginTask'] }>()
const { t } = useI18n()
const message = useMessage()
const server = useServerStore()
const tasks = useTasksStore()

const show = ref(false)
const params = ref<Record<string, string>>({})
const busy = ref(false)
const error = ref<LocalizedError>()

function open() {
  error.value = undefined
  params.value = Object.fromEntries(
    props.task.params.map((p) => [p.attr, p.default ?? (p.kind === 'dn' && p.required ? (server.info?.server.namingContexts[0] ?? '') : '')]),
  )
  show.value = true
}

async function start() {
  busy.value = true
  error.value = undefined
  try {
    const task = await call(api.POST('/api/v1/plugin-tasks/{id}', { params: { path: { id: props.task.id } }, body: { params: params.value } }))
    tasks.upsert(task)
    message.info(t('pluginTasks.started', { task: t(`pluginTasks.${props.task.id}.title`) }))
    show.value = false
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NButton size="small" @click="open">
    <template #icon><NIcon><Play /></NIcon></template>
    {{ t(`pluginTasks.${task.id}.title`) }}
  </NButton>
  <NModal v-model:show="show" preset="card" :title="t(`pluginTasks.${task.id}.title`)" :style="{ width: 'min(560px, 92vw)' }" :mask-closable="!busy">
    <p class="hint">{{ t(`pluginTasks.${task.id}.description`) }}</p>
    <NFormItem v-for="p in task.params" :key="p.attr" :label="t(`pluginTasks.params.${p.attr}`) + (p.required ? ' *' : '')">
      <NRadioGroup v-if="p.kind === 'enum'" v-model:value="params[p.attr]">
        <NRadioButton v-for="v in p.values ?? []" :key="v" :value="v">{{ v }}</NRadioButton>
      </NRadioGroup>
      <NInput v-else v-model:value="params[p.attr]" :input-props="{ class: 'mono', spellcheck: false }" />
    </NFormItem>
    <NAlert v-if="error" type="error">{{ error.summary }}</NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="busy" @click="start">{{ t('pluginTasks.start') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 13px;
}
</style>
