<script setup lang="ts">
/** VLV indexes of a backend: list, create, build (reindex task), delete. */
import { computed, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NDataTable,
  NDynamicInput,
  NEmpty,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSpace,
  useDialog,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import { Hammer, Plus, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError } from '@/i18n/errors'
import { useTasksStore } from '@/stores/tasks'

type VLV = Schemas['VLVIndex']

const props = defineProps<{ backend: string; suffix: string }>()

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()
const tasks = useTasksStore()

const list = ref<VLV[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    list.value = await call(api.GET('/api/v1/backends/{name}/vlv', { params: { path: { name: props.backend } } }))
  } catch (e) {
    fail(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.backend, load, { immediate: true })

function fail(e: unknown) {
  const err = localizeError(e)
  message.error(`${t('vlv.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
}

async function build(v: VLV) {
  try {
    const task = await call(
      api.POST('/api/v1/backends/{name}/reindex', { params: { path: { name: props.backend } }, body: { vlv: [v.name] } }),
    )
    tasks.upsert(task)
    message.info(t('vlv.buildStarted', { name: v.name }))
  } catch (e) {
    fail(e)
  }
}

function remove(v: VLV) {
  dialog.warning({
    title: t('vlv.delete'),
    content: t('vlv.deleteConfirm', { name: v.name }),
    positiveText: t('vlv.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/backends/{name}/vlv/{vlv}', { params: { path: { name: props.backend, vlv: v.name } } }))
        message.success(t('vlv.deleted', { name: v.name }))
        await load()
      } catch (e) {
        fail(e)
      }
    },
  })
}

// Create dialog.
type Draft = { name: string; base: string; scope: 'base' | 'one' | 'sub'; filter: string; sorts: string[] }
const draft = ref<Draft>()
const saving = ref(false)
const complete = computed(() => {
  const d = draft.value
  return !!d && !!d.name.trim() && !!d.base.trim() && !!d.filter.trim() && d.sorts.some((s) => s.trim())
})

function add() {
  draft.value = { name: '', base: props.suffix, scope: 'sub', filter: '(objectClass=*)', sorts: [''] }
}

async function create() {
  const d = draft.value
  if (!d || !complete.value) return
  saving.value = true
  try {
    const sorts = d.sorts.map((s) => s.trim()).filter(Boolean)
    await call(api.POST('/api/v1/backends/{name}/vlv', { params: { path: { name: props.backend } }, body: { ...d, sorts } }))
    const name = d.name.trim()
    draft.value = undefined
    await load()
    // A new VLV index is empty: build it right away.
    const v = list.value.find((x) => x.name === name)
    if (v) await build(v)
  } catch (e) {
    fail(e)
  } finally {
    saving.value = false
  }
}

const icon = (c: unknown) => () => h(NIcon, null, () => h(c as never))
const columns = computed<DataTableColumns<VLV>>(() => [
  { key: 'name', title: t('indexes.vlvName') },
  { key: 'base', title: t('indexes.vlvBase'), className: 'mono' },
  { key: 'scope', title: t('indexes.vlvScope'), render: (r) => t(`vlv.scopes.${r.scope}`) },
  { key: 'filter', title: t('indexes.vlvFilter'), className: 'mono' },
  { key: 'sorts', title: t('indexes.vlvSort'), className: 'mono', render: (r) => r.sorts.join('; ') },
  {
    key: 'actions',
    title: '',
    width: 76,
    render: (r) =>
      h('div', { class: 'row-actions' }, [
        h(NButton, { size: 'tiny', quaternary: true, title: t('vlv.build'), 'aria-label': t('vlv.build'), onClick: () => build(r) }, { icon: icon(Hammer) }),
        h(NButton, { size: 'tiny', quaternary: true, type: 'error', title: t('vlv.delete'), 'aria-label': t('vlv.delete'), onClick: () => remove(r) }, { icon: icon(Trash2) }),
      ]),
  },
])
</script>

<template>
  <NSpace class="toolbar">
    <NButton type="primary" size="small" @click="add">
      <template #icon><NIcon><Plus /></NIcon></template>
      {{ t('vlv.add') }}
    </NButton>
  </NSpace>
  <NDataTable :columns="columns" :data="list" :loading="loading" size="small" :row-key="(r: VLV) => r.name">
    <template #empty><NEmpty :description="t('indexes.none')" size="small" /></template>
  </NDataTable>

  <NModal
    :show="!!draft"
    preset="card"
    :title="t('vlv.add')"
    :style="{ width: 'min(560px, 92vw)' }"
    :mask-closable="!saving"
    @update:show="(v: boolean) => !v && (draft = undefined)"
  >
    <NForm v-if="draft" label-placement="top" @submit.prevent="create">
      <NFormItem :label="t('indexes.vlvName')">
        <NInput v-model:value="draft.name" placeholder="people by name" />
      </NFormItem>
      <NFormItem :label="t('indexes.vlvBase')">
        <NInput v-model:value="draft.base" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('indexes.vlvScope')">
        <NRadioGroup v-model:value="draft.scope">
          <NRadioButton v-for="s in ['base', 'one', 'sub']" :key="s" :value="s">{{ t(`vlv.scopes.${s}`) }}</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem :label="t('indexes.vlvFilter')">
        <NInput v-model:value="draft.filter" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('indexes.vlvSort')" :feedback="t('vlv.sortHint')">
        <NDynamicInput v-model:value="draft.sorts" :min="1" placeholder="sn givenName" />
      </NFormItem>
    </NForm>
    <p class="hint">{{ t('vlv.buildHint') }}</p>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="saving" @click="draft = undefined">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :disabled="!complete" :loading="saving" @click="create">{{ t('create.create') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.toolbar {
  margin-bottom: 10px;
}

.hint {
  margin: 8px 0 0;
  font-size: 13px;
  opacity: 0.75;
}

:deep(.row-actions) {
  display: grid;
  grid-template-columns: repeat(2, 28px);
}
</style>
