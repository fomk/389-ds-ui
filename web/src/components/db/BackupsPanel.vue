<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NDataTable, NEmpty, NIcon, NInput, NSpace, useDialog, useMessage, type DataTableColumns } from 'naive-ui'
import { Archive, Download, RotateCcw, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { formatBytes } from '@/ldap/values'
import { useServerStore } from '@/stores/server'
import { useTasksStore } from '@/stores/tasks'
import TaskList from '@/components/TaskList.vue'

type FileInfo = Schemas['Info']

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()
const tasks = useTasksStore()

const canSeeFiles = computed(() => server.capabilities?.files ?? false)
const list = ref<FileInfo[]>([])
const loading = ref(false)
const name = ref('')

async function load() {
  if (!canSeeFiles.value) return
  loading.value = true
  try {
    list.value = await call(api.GET('/api/v1/backups'))
  } catch (e) {
    message.error(`${t('backups.error')}: ${localizeError(e).summary}`)
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(canSeeFiles, load)

// Reload the list when a backup or restore task finishes.
watch(
  () => tasks.tasks.filter((x) => (x.type === 'backup' || x.type === 'restore') && x.status !== 'running').length,
  load,
)

async function create() {
  try {
    const task = await call(api.POST('/api/v1/backups', { body: { name: name.value.trim() || undefined } }))
    tasks.upsert(task)
    name.value = ''
    message.info(t('backups.created'))
  } catch (e) {
    message.error(`${t('backups.error')}: ${localizeError(e).summary}`)
  }
}

function restore(b: FileInfo) {
  dialog.warning({
    title: t('backups.restore'),
    content: t('backups.restoreConfirm', { name: b.name }),
    positiveText: t('backups.restore'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        tasks.upsert(await call(api.POST('/api/v1/backups/{name}/restore', { params: { path: { name: b.name } } })))
        message.info(t('backups.restoreStarted'))
      } catch (e) {
        message.error(`${t('backups.error')}: ${localizeError(e).summary}`)
      }
    },
  })
}

function remove(b: FileInfo) {
  dialog.warning({
    title: t('backups.delete'),
    content: t('backups.deleteConfirm', { name: b.name }),
    positiveText: t('backups.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/backups/{name}', { params: { path: { name: b.name } } }))
        message.success(t('backups.deleted', { name: b.name }))
        await load()
      } catch (e) {
        message.error(`${t('backups.error')}: ${localizeError(e).summary}`)
      }
    },
  })
}

const dateFmt = computed(() => new Intl.DateTimeFormat(currentLocale.value, { dateStyle: 'medium', timeStyle: 'short' }))
const icon = (c: unknown) => () => h(NIcon, null, () => h(c as never))
const columns = computed<DataTableColumns<FileInfo>>(() => [
  { key: 'name', title: t('backups.name'), render: (r) => h('span', { class: 'mono' }, r.name) },
  { key: 'modified', title: t('backups.date'), render: (r) => dateFmt.value.format(new Date(r.modified)) },
  { key: 'size', title: t('backups.size'), align: 'right', render: (r) => formatBytes(r.size, currentLocale.value) },
  {
    key: 'actions',
    title: '',
    width: 280,
    render: (r) =>
      h(NSpace, { size: 4, justify: 'end' }, () => [
        h(NButton, { size: 'tiny', onClick: () => restore(r) }, { icon: icon(RotateCcw), default: () => t('backups.restore') }),
        h(NButton, { size: 'tiny', tag: 'a', href: `/api/v1/backups/${encodeURIComponent(r.name)}/download` }, { icon: icon(Download), default: () => t('backups.download') }),
        h(NButton, { size: 'tiny', quaternary: true, title: t('backups.delete'), onClick: () => remove(r) }, { icon: icon(Trash2) }),
      ]),
  },
])

const backupTypes = ['backup', 'restore', 'delete-backup']
const backupTasks = computed(() => tasks.tasks.some((x) => backupTypes.includes(x.type)))
</script>

<template>
  <NSpace class="toolbar" align="center">
    <label class="name-label" for="backup-name">{{ t('backups.nameLabel') }}</label>
    <NInput
      v-model:value="name"
      size="small"
      :placeholder="t('backups.namePlaceholder')"
      :input-props="{ id: 'backup-name', class: 'mono' }"
      class="name"
      @keydown.enter="create"
    />
    <NButton type="primary" size="small" @click="create">
      <template #icon><NIcon><Archive /></NIcon></template>
      {{ t('backups.create') }}
    </NButton>
  </NSpace>
  <NAlert v-if="!canSeeFiles" type="info" class="block">{{ t('backups.noFiles') }}</NAlert>
  <NAlert v-else type="warning" :show-icon="true" class="block">{{ t('backups.sensitive') }}</NAlert>
  <NDataTable
    v-if="canSeeFiles"
    :columns="columns"
    :data="list"
    :loading="loading"
    :row-key="(r: FileInfo) => r.name"
    size="small"
  >
    <template #empty><NEmpty :description="t('backups.none')" size="small" /></template>
  </NDataTable>
  <div v-if="backupTasks" class="tasks">
    <TaskList :types="backupTypes" :limit="8" />
  </div>
</template>

<style scoped>
.toolbar {
  margin-bottom: 12px;
}

.name-label {
  font-size: 13px;
}

.name {
  width: 240px;
}

.block {
  margin-bottom: 12px;
}

.tasks {
  margin-top: 16px;
}
</style>
