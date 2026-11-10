<script setup lang="ts">
/** SASL identity mappings (cn=mapping,cn=sasl,cn=config): list and edit. */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NModal,
  NSpace,
  useDialog,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import { Pencil, Plus, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError } from '@/i18n/errors'

type Mapping = Schemas['SASLMapping']

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()

const list = ref<Mapping[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    list.value = await call(api.GET('/api/v1/sasl-mappings'))
  } catch (e) {
    message.error(`${t('sasl.error')}: ${localizeError(e).summary}`)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const fail = (e: unknown) => {
  const err = localizeError(e)
  message.error(`${t('sasl.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
}

// Add / edit dialog; original is the name being edited (undefined: new).
const editing = ref<{ m: Mapping; original?: string }>()
const saving = ref(false)
const complete = computed(() => {
  const m = editing.value?.m
  return !!m && !!m.name.trim() && !!m.regex.trim() && !!m.baseDN.trim() && !!m.filter.trim()
})

function add() {
  editing.value = { m: { name: '', regex: '', baseDN: '', filter: '', priority: 0 } }
}
function edit(m: Mapping) {
  editing.value = { m: { ...m, priority: m.priority ?? 0 }, original: m.name }
}

async function save() {
  const e = editing.value
  if (!e || !complete.value) return
  saving.value = true
  try {
    const body = { ...e.m, priority: e.m.priority || 0 }
    if (e.original === undefined) {
      await call(api.POST('/api/v1/sasl-mappings', { body }))
      message.success(t('sasl.added', { name: e.m.name }))
    } else {
      await call(api.PUT('/api/v1/sasl-mappings/{name}', { params: { path: { name: e.original } }, body }))
      message.success(t('sasl.saved'))
    }
    editing.value = undefined
    await load()
  } catch (err) {
    fail(err)
  } finally {
    saving.value = false
  }
}

function remove(m: Mapping) {
  dialog.warning({
    title: t('sasl.delete'),
    content: t('sasl.deleteConfirm', { name: m.name }),
    positiveText: t('sasl.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/sasl-mappings/{name}', { params: { path: { name: m.name } } }))
        message.success(t('sasl.deleted', { name: m.name }))
        await load()
      } catch (e) {
        fail(e)
      }
    },
  })
}

const icon = (c: unknown) => () => h(NIcon, null, () => h(c as never))
const columns = computed<DataTableColumns<Mapping>>(() => [
  { key: 'priority', title: t('sasl.priority'), width: 90, render: (r) => r.priority || '—' },
  { key: 'name', title: t('sasl.name') },
  { key: 'regex', title: t('sasl.regex'), className: 'mono' },
  { key: 'baseDN', title: t('sasl.baseDN'), className: 'mono' },
  { key: 'filter', title: t('sasl.filter'), className: 'mono' },
  {
    key: 'actions',
    title: '',
    width: 76,
    render: (r) =>
      h('div', { class: 'row-actions' }, [
        h(NButton, { size: 'tiny', quaternary: true, title: t('sasl.edit'), 'aria-label': t('sasl.edit'), onClick: () => edit(r) }, { icon: icon(Pencil) }),
        h(NButton, { size: 'tiny', quaternary: true, type: 'error', title: t('sasl.delete'), 'aria-label': t('sasl.delete'), onClick: () => remove(r) }, { icon: icon(Trash2) }),
      ]),
  },
])
</script>

<template>
  <NCard size="small" :title="t('pages.sasl-mappings.title')">
    <template #header-extra>
      <NButton size="small" type="primary" @click="add">
        <template #icon><NIcon><Plus /></NIcon></template>
        {{ t('sasl.add') }}
      </NButton>
    </template>
    <p class="description">{{ t('pages.sasl-mappings.description') }}</p>
    <NDataTable :columns="columns" :data="list" :loading="loading" size="small" :row-key="(r: Mapping) => r.name">
      <template #empty><NEmpty :description="t('sasl.none')" size="small" /></template>
    </NDataTable>

    <NModal
      :show="!!editing"
      preset="card"
      :title="editing?.original === undefined ? t('sasl.add') : t('sasl.edit')"
      :style="{ width: 'min(560px, 92vw)' }"
      :mask-closable="!saving"
      @update:show="(v: boolean) => !v && (editing = undefined)"
    >
      <NForm v-if="editing" label-placement="top" @submit.prevent="save">
        <NFormItem :label="t('sasl.name')">
          <NInput v-model:value="editing.m.name" :disabled="editing.original !== undefined" />
        </NFormItem>
        <NFormItem :label="t('sasl.regex')" :feedback="t('sasl.regexHint')">
          <NInput v-model:value="editing.m.regex" :input-props="{ class: 'mono', spellcheck: false }" placeholder="^(.*)@EXAMPLE\.ORG$" />
        </NFormItem>
        <NFormItem :label="t('sasl.baseDN')">
          <NInput v-model:value="editing.m.baseDN" :input-props="{ class: 'mono', spellcheck: false }" placeholder="ou=people,dc=example,dc=org" />
        </NFormItem>
        <NFormItem :label="t('sasl.filter')" :feedback="t('sasl.filterHint')">
          <NInput v-model:value="editing.m.filter" :input-props="{ class: 'mono', spellcheck: false }" placeholder="(uid=\1)" />
        </NFormItem>
        <NFormItem :label="t('sasl.priority')" :feedback="t('sasl.priorityHint')">
          <NInputNumber v-model:value="editing.m.priority" :min="0" :max="100" class="prio" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="saving" @click="editing = undefined">{{ t('edit.cancel') }}</NButton>
          <NButton type="primary" :disabled="!complete" :loading="saving" @click="save">{{ t('pages._save') }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </NCard>
</template>

<style scoped>
.description {
  margin: 0 0 12px;
  max-width: 820px;
}

.prio {
  width: 140px;
}

:deep(.row-actions) {
  display: grid;
  grid-template-columns: repeat(2, 28px);
}
</style>
