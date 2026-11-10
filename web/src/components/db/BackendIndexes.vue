<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NCheckbox,
  NCheckboxGroup,
  NDataTable,
  NForm,
  NFormItem,
  NIcon,
  NModal,
  NSelect,
  NSpace,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import { Pencil, Plus, RefreshCw, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError } from '@/i18n/errors'
import { useSchemaStore } from '@/stores/schema'
import { useTasksStore } from '@/stores/tasks'

type Index = Schemas['Index']

const props = defineProps<{ backend: string }>()

const { t, te } = useI18n()
// Unknown index types (server-internal ones) are shown as they are.
const typeName = (ty: string) => (te(`indexes.typeNames.${ty}`) ? t(`indexes.typeNames.${ty}`) : ty)
const message = useMessage()
const dialog = useDialog()
const schema = useSchemaStore()
const tasks = useTasksStore()

const list = ref<Index[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    list.value = await call(api.GET('/api/v1/backends/{name}/indexes', { params: { path: { name: props.backend } } }))
  } catch (e) {
    message.error(`${t('indexes.error')}: ${localizeError(e).summary}`)
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  load()
  schema.load()
})
watch(() => props.backend, load)

async function reindex(attributes: string[]) {
  try {
    const task = await call(
      api.POST('/api/v1/backends/{name}/reindex', { params: { path: { name: props.backend } }, body: { attributes } }),
    )
    tasks.upsert(task)
    message.info(t('indexes.reindexStarted'))
  } catch (e) {
    message.error(`${t('indexes.error')}: ${localizeError(e).summary}`)
  }
}

function remove(ix: Index) {
  dialog.warning({
    title: t('indexes.delete'),
    content: t('indexes.deleteConfirm', { attr: ix.attribute }),
    positiveText: t('indexes.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/backends/{name}/indexes/{attr}', { params: { path: { name: props.backend, attr: ix.attribute } } }))
        message.success(t('indexes.deleted', { attr: ix.attribute }))
        await load()
      } catch (e) {
        message.error(`${t('indexes.error')}: ${localizeError(e).summary}`)
      }
    },
  })
}

// Add / edit dialog.
const editing = ref<{ attribute: string; types: string[]; isNew: boolean }>()
const typeOptions = ['eq', 'pres', 'sub', 'approx']
const attrOptions = computed(() => {
  const have = new Set(list.value.map((i) => i.attribute.toLowerCase()))
  return (schema.schema?.attributeTypes ?? [])
    .filter((a) => (a.usage ?? 'userApplications') === 'userApplications')
    .map((a) => a.names[0] ?? a.oid)
    .filter((n) => !have.has(n.toLowerCase()))
    .sort((a, b) => a.localeCompare(b))
    .map((n) => ({ label: n, value: n }))
})
const saving = ref(false)

async function saveIndex() {
  const e = editing.value
  if (!e?.attribute || !e.types.length) return
  saving.value = true
  try {
    if (e.isNew) {
      await call(api.POST('/api/v1/backends/{name}/indexes', {
        params: { path: { name: props.backend } },
        body: { attribute: e.attribute, types: e.types, matchingRules: [], system: false },
      }))
      message.success(t('indexes.added', { attr: e.attribute }))
    } else {
      await call(api.PUT('/api/v1/backends/{name}/indexes/{attr}', {
        params: { path: { name: props.backend, attr: e.attribute } },
        body: { types: e.types },
      }))
      message.success(t('indexes.updated'))
    }
    editing.value = undefined
    await load()
  } catch (err) {
    message.error(`${t('indexes.error')}: ${localizeError(err).summary}`)
  } finally {
    saving.value = false
  }
}

const icon = (c: unknown) => () => h(NIcon, null, () => h(c as never))
const columns = computed<DataTableColumns<Index>>(() => [
  {
    key: 'attribute',
    title: t('indexes.attribute'),
    render: (r) => [
      h('span', { class: 'mono' }, r.attribute),
      r.system ? h(NTag, { size: 'tiny', bordered: false, style: 'margin-left:8px' }, () => t('indexes.system')) : null,
    ],
  },
  {
    key: 'types',
    title: t('indexes.types'),
    render: (r) =>
      h(NSpace, { size: 4 }, () => r.types.map((ty) => h(NTag, { size: 'small', bordered: false, type: 'info' }, () => typeName(ty)))),
  },
  { key: 'matchingRules', title: t('indexes.matchingRules'), render: (r) => r.matchingRules.join(', ') },
  {
    key: 'actions',
    title: '',
    width: 120,
    render: (r) =>
      h(NSpace, { size: 2, justify: 'end' }, () => [
        h(NButton, { size: 'tiny', quaternary: true, title: t('indexes.reindex'), onClick: () => reindex([r.attribute]) }, { icon: icon(RefreshCw) }),
        h(NButton, { size: 'tiny', quaternary: true, title: t('indexes.edit'), onClick: () => (editing.value = { attribute: r.attribute, types: [...r.types], isNew: false }) }, { icon: icon(Pencil) }),
        r.system ? null : h(NButton, { size: 'tiny', quaternary: true, title: t('indexes.delete'), onClick: () => remove(r) }, { icon: icon(Trash2) }),
      ]),
  },
])
</script>

<template>
  <NSpace class="toolbar">
    <NButton type="primary" size="small" @click="editing = { attribute: '', types: ['eq'], isNew: true }">
      <template #icon><NIcon><Plus /></NIcon></template>
      {{ t('indexes.add') }}
    </NButton>
    <NButton size="small" @click="reindex([])">
      <template #icon><NIcon><RefreshCw /></NIcon></template>
      {{ t('indexes.reindexAll') }}
    </NButton>
  </NSpace>
  <NDataTable :columns="columns" :data="list" :loading="loading" :row-key="(r: Index) => r.attribute" size="small" :max-height="460" />

  <NModal
    :show="!!editing"
    preset="card"
    :title="editing?.isNew ? t('indexes.add') : t('indexes.edit')"
    :style="{ width: 'min(480px, 92vw)' }"
    @update:show="(v) => !v && (editing = undefined)"
  >
    <NForm v-if="editing" label-placement="top">
      <NFormItem :label="t('indexes.attribute')">
        <NSelect v-if="editing.isNew" v-model:value="editing.attribute" filterable :options="attrOptions" />
        <span v-else class="mono">{{ editing.attribute }}</span>
      </NFormItem>
      <NFormItem :label="t('indexes.types')">
        <NCheckboxGroup v-model:value="editing.types">
          <NSpace>
            <NCheckbox v-for="ty in typeOptions" :key="ty" :value="ty">{{ t(`indexes.typeNames.${ty}`) }}</NCheckbox>
          </NSpace>
        </NCheckboxGroup>
      </NFormItem>
    </NForm>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="editing = undefined">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!editing?.attribute || !editing?.types.length" @click="saveIndex">
          {{ t('db.save') }}
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.toolbar {
  margin-bottom: 10px;
}
</style>
