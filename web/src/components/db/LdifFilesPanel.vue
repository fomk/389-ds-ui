<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NDataTable, NEmpty, NIcon, NSpace, useDialog, useMessage, type DataTableColumns } from 'naive-ui'
import { Download, Trash2, Upload } from '@lucide/vue'
import { api, ApiError, call, type Problem, type Schemas } from '@/api/client'
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

const ldifTypes = ['export', 'import', 'upload-ldif', 'delete-ldif']
const canSeeFiles = computed(() => server.capabilities?.files ?? false)
const list = ref<FileInfo[]>([])
const loading = ref(false)
const uploading = ref(false)

async function load() {
  if (!canSeeFiles.value) return
  loading.value = true
  try {
    list.value = await call(api.GET('/api/v1/ldif-files'))
  } catch (e) {
    message.error(`${t('ldifFiles.error')}: ${localizeError(e).summary}`)
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(canSeeFiles, load)
// An export task writes a new file.
watch(() => tasks.tasks.filter((x) => x.type === 'export' && x.status !== 'running').length, load)

defineExpose({ reload: load, files: list })

function upload() {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = '.ldif,text/plain'
  input.onchange = async () => {
    const file = input.files?.[0]
    if (!file) return
    uploading.value = true
    try {
      // Streamed upload (not part of the typed client): PUT with the raw body.
      const res = await fetch(`/api/v1/ldif-files/${encodeURIComponent(file.name)}`, {
        method: 'PUT',
        headers: { 'X-Requested-With': 'ds-ui', 'Content-Type': 'application/octet-stream' },
        body: file,
      })
      if (!res.ok) throw new ApiError(res.status, (await res.json().catch(() => ({}))) as Problem)
      message.success(t('ldifFiles.uploaded', { name: file.name }))
      await load()
    } catch (e) {
      message.error(`${t('ldifFiles.error')}: ${localizeError(e).summary}`)
    } finally {
      uploading.value = false
    }
  }
  input.click()
}

function remove(f: FileInfo) {
  dialog.warning({
    title: t('backups.delete'),
    content: t('ldifFiles.deleteConfirm', { name: f.name }),
    positiveText: t('backups.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/ldif-files/{name}', { params: { path: { name: f.name } } }))
        message.success(t('ldifFiles.deleted', { name: f.name }))
        await load()
      } catch (e) {
        message.error(`${t('ldifFiles.error')}: ${localizeError(e).summary}`)
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
    width: 160,
    render: (r) =>
      h(NSpace, { size: 4, justify: 'end' }, () => [
        h(NButton, { size: 'tiny', tag: 'a', href: `/api/v1/ldif-files/${encodeURIComponent(r.name)}` }, { icon: icon(Download), default: () => t('backups.download') }),
        h(NButton, { size: 'tiny', quaternary: true, title: t('backups.delete'), onClick: () => remove(r) }, { icon: icon(Trash2) }),
      ]),
  },
])
</script>

<template>
  <NAlert v-if="!canSeeFiles" type="info">{{ t('ldifFiles.noFiles') }}</NAlert>
  <template v-else>
    <NSpace class="toolbar">
      <NButton type="primary" size="small" :loading="uploading" @click="upload">
        <template #icon><NIcon><Upload /></NIcon></template>
        {{ t('ldifFiles.upload') }}
      </NButton>
    </NSpace>
    <NDataTable :columns="columns" :data="list" :loading="loading" :row-key="(r: FileInfo) => r.name" size="small">
      <template #empty><NEmpty :description="t('ldifFiles.none')" size="small" /></template>
    </NDataTable>
    <div v-if="tasks.tasks.some((x) => ldifTypes.includes(x.type))" class="tasks">
      <TaskList :types="ldifTypes" :limit="8" />
    </div>
  </template>
</template>

<style scoped>
.toolbar {
  margin-bottom: 12px;
}

.tasks {
  margin-top: 16px;
}
</style>
