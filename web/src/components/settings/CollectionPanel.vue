<script setup lang="ts">
/**
 * A collection of similar entries (plug-in definitions): list, create, edit,
 * delete. Collections nested under this one are shown inside each item.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NFormItem,
  NIcon,
  NInput,
  NModal,
  NSpace,
  useDialog,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { ChevronDown, ChevronRight, Pencil, Plus, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { useLatest } from '@/composables/latest'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { changedAttrs, clean, fieldKey } from '@/settings/fields'
import SettingsField from './SettingsField.vue'

type State = Schemas['CollectionState']
type Item = Schemas['Item']

const props = defineProps<{ id: string; parent?: string; nested?: string[]; showAttrs?: boolean }>()

const { t, te } = useI18n()
const vars = useThemeVars()
const message = useMessage()
const dialog = useDialog()

const state = ref<State>()
const error = ref<LocalizedError>()
const open = ref<Set<string>>(new Set())

const latest = useLatest()
async function load() {
  error.value = undefined
  try {
    const r = await latest.run((signal) =>
      call(api.GET('/api/v1/collections/{id}', { params: { path: { id: props.id }, query: props.parent ? { parent: props.parent } : {} }, signal })),
    )
    if (!r.stale) state.value = r.value
  } catch (e) {
    error.value = localizeError(e)
  }
}
watch(() => [props.id, props.parent], load, { immediate: true })

const fields = computed(() => state.value?.collection.fields ?? [])
const title = computed(() => t(`pages.${props.id}.title`))
const description = computed(() => (te(`pages.${props.id}.description`) ? t(`pages.${props.id}.description`) : ''))
/** The first required fields summarise an item in the list. */
const summaryFields = computed(() => fields.value.filter((f) => f.required).slice(0, 2))
const query = computed(() => (props.parent ? { parent: props.parent } : {}))

function toggle(name: string) {
  const s = new Set(open.value)
  if (s.has(name)) s.delete(name)
  else s.add(name)
  open.value = s
}

// --- dialog -----------------------------------------------------------------

const editing = ref<{ name: string; isNew: boolean; values: Record<string, string[]>; original: Record<string, string[]> }>()
const saving = ref(false)
const saveError = ref<LocalizedError>()
const changed = computed(() => (editing.value ? changedAttrs(editing.value.values, editing.value.original) : []))
const missing = computed(() => fields.value.filter((f) => f.required && !clean(editing.value?.values[f.attr]).length))

function add() {
  saveError.value = undefined
  const values = Object.fromEntries(fields.value.map((f) => [f.attr, [] as string[]]))
  editing.value = { name: '', isNew: true, values, original: { ...values } }
}
function edit(it: Item) {
  saveError.value = undefined
  const values = Object.fromEntries(fields.value.map((f) => [f.attr, [...(it.values[f.attr] ?? [])]]))
  editing.value = { name: it.name, isNew: false, values, original: Object.fromEntries(Object.entries(values).map(([k, v]) => [k, [...v]])) }
}

async function save() {
  const e = editing.value
  if (!e || missing.value.length || (e.isNew && !e.name.trim())) return
  saving.value = true
  saveError.value = undefined
  try {
    if (e.isNew) {
      const values = Object.fromEntries(Object.entries(e.values).filter(([, v]) => clean(v).length))
      await call(api.POST('/api/v1/collections/{id}', { params: { path: { id: props.id }, query: query.value }, body: { name: e.name.trim(), values } }))
      message.success(t('collections.created', { name: e.name.trim() }))
    } else {
      const values = Object.fromEntries(changed.value.map((a) => [a, e.values[a] ?? []]))
      await call(
        api.PATCH('/api/v1/collections/{id}/items/{name}', { params: { path: { id: props.id, name: e.name }, query: query.value }, body: { values } }),
      )
      message.success(t('collections.saved', { name: e.name }))
    }
    editing.value = undefined
    await load()
  } catch (err) {
    saveError.value = localizeError(err)
  } finally {
    saving.value = false
  }
}

function remove(it: Item) {
  dialog.warning({
    title: t('collections.delete'),
    content: t(props.nested?.length ? 'collections.deleteNested' : 'collections.deleteConfirm', { name: it.name }),
    positiveText: t('collections.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/collections/{id}/items/{name}', { params: { path: { id: props.id, name: it.name }, query: query.value } }))
        message.success(t('collections.deleted', { name: it.name }))
        await load()
      } catch (e) {
        message.error(`${t('collections.error')}: ${localizeError(e).summary}`)
      }
    },
  })
}
</script>

<template>
  <NCard size="small" :title="title" :class="{ nested: !!parent }" :bordered="!parent">
    <template #header-extra>
      <NButton size="small" type="primary" @click="add">
        <template #icon><NIcon><Plus /></NIcon></template>
        {{ t('collections.add') }}
      </NButton>
    </template>
    <p v-if="description && !parent" class="description">{{ description }}</p>
    <NAlert v-if="error" type="error">{{ error.summary }}</NAlert>
    <NEmpty v-else-if="state && !state.items.length" size="small" :description="t('collections.none')" />
    <div v-for="it in state?.items ?? []" :key="it.dn" class="item">
      <div class="item-row">
        <button v-if="nested?.length" type="button" class="expand" :aria-label="t('collections.expand')" @click="toggle(it.name)">
          <NIcon :size="16"><ChevronDown v-if="open.has(it.name)" /><ChevronRight v-else /></NIcon>
        </button>
        <div class="item-text">
          <div class="name">{{ it.name }}</div>
          <div v-for="f in summaryFields" :key="f.attr" class="summary">
            <span class="muted">{{ t(`${fieldKey(id, f)}.label`) }}:</span>
            <span class="mono">{{ (it.values[f.attr] ?? []).join(', ') || '—' }}</span>
          </div>
        </div>
        <div class="row-actions">
          <NButton size="tiny" quaternary :title="t('collections.edit')" :aria-label="t('collections.edit')" @click="edit(it)">
            <template #icon><NIcon><Pencil /></NIcon></template>
          </NButton>
          <NButton size="tiny" quaternary type="error" :title="t('collections.delete')" :aria-label="t('collections.delete')" @click="remove(it)">
            <template #icon><NIcon><Trash2 /></NIcon></template>
          </NButton>
        </div>
      </div>
      <div v-if="open.has(it.name)" class="children">
        <CollectionPanel v-for="n in nested" :id="n" :key="n" :parent="it.name" :show-attrs="showAttrs" />
      </div>
    </div>
  </NCard>

  <NModal
    :show="!!editing"
    preset="card"
    :title="editing?.isNew ? `${title}: ${t('collections.add').toLowerCase()}` : `${title}: ${editing?.name}`"
    :style="{ width: 'min(820px, 94vw)' }"
    :mask-closable="!saving"
    @update:show="(v: boolean) => !v && (editing = undefined)"
  >
    <template v-if="editing">
      <NFormItem v-if="editing.isNew" :label="t('collections.name')" :feedback="t('collections.nameHint')">
        <NInput v-model:value="editing.name" />
      </NFormItem>
      <SettingsField
        v-for="f in fields"
        :key="f.attr"
        v-model="editing.values[f.attr]!"
        :page-id="id"
        :field="f"
        :options="state?.options?.[f.attr]"
        :changed="!editing.isNew && changed.includes(f.attr)"
        :show-attr="showAttrs"
        sparse
      />
      <p v-if="missing.length" class="missing">{{ t('collections.required', { fields: missing.map((f) => t(`${fieldKey(id, f)}.label`)).join(', ') }) }}</p>
      <NAlert v-if="saveError" type="error">
        {{ saveError.summary }}
        <div v-if="saveError.detail" class="mono">{{ saveError.detail }}</div>
      </NAlert>
    </template>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="saving" @click="editing = undefined">{{ t('edit.cancel') }}</NButton>
        <NButton
          type="primary"
          :loading="saving"
          :disabled="!!missing.length || (editing?.isNew ? !editing.name.trim() : !changed.length)"
          @click="save"
        >
          {{ editing?.isNew ? t('create.create') : t('pages._save') }}
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.nested {
  margin-top: 8px;
  background: v-bind('vars.actionColor');
}

.description {
  margin: 0 0 8px;
  color: v-bind('vars.textColor2');
}

.item + .item {
  border-top: 1px solid v-bind('vars.dividerColor');
}

.item-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 0;
}

.expand {
  padding: 2px;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.item-text {
  flex: 1;
  min-width: 0;
}

.name {
  font-weight: 500;
}

.summary {
  font-size: 12.5px;
  overflow-wrap: anywhere;
}

.muted {
  margin-right: 6px;
  color: v-bind('vars.textColor3');
}

.row-actions {
  display: grid;
  grid-template-columns: repeat(2, 28px);
}

.children {
  padding: 0 0 10px 26px;
}

.missing {
  margin: 10px 0;
  font-size: 13px;
  color: v-bind('vars.warningColor');
}
</style>
