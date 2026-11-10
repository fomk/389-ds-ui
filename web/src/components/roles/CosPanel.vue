<script setup lang="ts">
/** Class of Service: definitions with their templates. */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NDynamicTags,
  NEmpty,
  NForm,
  NFormItem,
  NGrid,
  NGridItem,
  NIcon,
  NInput,
  NInputNumber,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSkeleton,
  NSpace,
  NTag,
  useDialog,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { Pencil, Plus, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'

type Def = Schemas['CoSDefinition']
type Template = Schemas['CoSTemplate']

const props = defineProps<{ selected?: string }>()
const emit = defineEmits<{ select: [dn: string | undefined] }>()

const { t } = useI18n()
const vars = useThemeVars()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()

const defs = ref<Def[]>()
const error = ref<LocalizedError>()
async function load() {
  try {
    defs.value = await call(api.GET('/api/v1/cos'))
  } catch (e) {
    error.value = localizeError(e)
  }
}
onMounted(load)

const def = computed(() => defs.value?.find((d) => d.dn === props.selected))
const parentOf = (dn: string) => dn.slice(dn.indexOf(',') + 1)
const attrNames = (d: Def) => d.attributes.map((a) => a.split(/\s+/)[0]!).filter(Boolean)
const fail = (e: unknown) => {
  const err = localizeError(e)
  message.error(`${t('cos.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
}

function confirm(content: string, run: () => Promise<void>) {
  dialog.warning({
    title: t('cos.delete'),
    content,
    positiveText: t('cos.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await run()
        await load()
      } catch (e) {
        fail(e)
      }
    },
  })
}
function removeDef() {
  const d = def.value
  if (!d) return
  confirm(t('cos.deleteConfirm', { name: d.name }), async () => {
    await call(api.DELETE('/api/v1/cos', { params: { query: { dn: d.dn } } }))
    message.success(t('cos.deleted', { name: d.name }))
    emit('select', undefined)
  })
}
function removeTemplate(tpl: Template) {
  confirm(t('cos.deleteTemplateConfirm', { name: tpl.name }), async () => {
    await call(api.DELETE('/api/v1/cos/templates', { params: { query: { dn: tpl.dn } } }))
    message.success(t('cos.templateDeleted'))
  })
}

// --- template editor -------------------------------------------------------------

const editing = ref<{ dn: string; name: string; isNew: boolean; priority: number | null; values: Record<string, string> }>()
const saving = ref(false)
const saveError = ref<LocalizedError>()

function editTemplate(tpl: Template) {
  const d = def.value!
  saveError.value = undefined
  editing.value = {
    dn: tpl.dn,
    name: tpl.name,
    isNew: false,
    priority: tpl.priority ?? null,
    values: Object.fromEntries(attrNames(d).map((a) => [a, (tpl.values[a] ?? []).join('\n')])),
  }
}
/** Pointer: the template at cosTemplateDN; classic: cn=<specifier value> below the container. */
function newTemplate() {
  const d = def.value!
  saveError.value = undefined
  editing.value = {
    dn: d.kind === 'pointer' ? (d.templateDN ?? '') : '',
    name: '',
    isNew: true,
    priority: null,
    values: Object.fromEntries(attrNames(d).map((a) => [a, ''])),
  }
}
const templateDN = computed(() => {
  const e = editing.value
  const d = def.value
  if (!e || !d) return ''
  if (!e.isNew || d.kind === 'pointer') return e.dn
  return e.name.trim() ? `cn=${e.name.trim().replace(/([,=+"\\<>;])/g, '\\$1')},${d.templateDN}` : ''
})
async function saveTemplate() {
  const e = editing.value
  if (!e || !templateDN.value) return
  saving.value = true
  saveError.value = undefined
  try {
    const values = Object.fromEntries(Object.entries(e.values).map(([a, v]) => [a, v.split('\n').map((x) => x.trim()).filter(Boolean)]))
    await call(
      api.PUT('/api/v1/cos/templates', {
        params: { query: { dn: templateDN.value } },
        body: { values, ...(e.priority !== null ? { priority: e.priority } : {}) },
      }),
    )
    message.success(t('cos.templateSaved'))
    editing.value = undefined
    await load()
  } catch (err) {
    saveError.value = localizeError(err)
  } finally {
    saving.value = false
  }
}
const canAddTemplate = computed(() => def.value && (def.value.kind === 'classic' || (def.value.kind === 'pointer' && !def.value.templates.length)))

// --- create definition --------------------------------------------------------------

const creating = ref(false)
const draft = ref<Schemas['NewCoS']>({ parent: '', name: '', kind: 'pointer', attributes: [] })
const busy = ref(false)
const createError = ref<LocalizedError>()
function openCreate() {
  const base = server.info?.server.namingContexts[0] ?? ''
  draft.value = { parent: base, name: '', kind: 'pointer', attributes: [], templateDN: '', specifier: '', indirectSpecifier: '', description: '' }
  createError.value = undefined
  creating.value = true
}
async function create() {
  busy.value = true
  createError.value = undefined
  try {
    const d = await call(api.POST('/api/v1/cos', { body: draft.value }))
    creating.value = false
    message.success(t('cos.created', { name: d.name }))
    await load()
    emit('select', d.dn)
  } catch (e) {
    createError.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NAlert v-if="error" type="error">{{ error.summary }}</NAlert>
  <NGrid v-else cols="1 l:5" responsive="screen" :x-gap="16" :y-gap="16">
    <NGridItem span="1 l:2">
      <NCard size="small" :title="t('cos.list')">
        <template #header-extra>
          <NButton size="small" type="primary" @click="openCreate">
            <template #icon><NIcon><Plus /></NIcon></template>
            {{ t('cos.new') }}
          </NButton>
        </template>
        <NSkeleton v-if="!defs" text :repeat="4" />
        <NEmpty v-else-if="!defs.length" size="small" :description="t('cos.none')" />
        <div v-else class="list">
          <button v-for="d in defs" :key="d.dn" type="button" class="item" :class="{ active: d.dn === selected }" @click="emit('select', d.dn)">
            <span class="item-text">
              <span>{{ d.name }}</span>
              <span class="muted mono">{{ parentOf(d.dn) }}</span>
            </span>
            <NTag size="tiny" :bordered="false">{{ t(`cos.kinds.${d.kind}`) }}</NTag>
          </button>
        </div>
      </NCard>
    </NGridItem>
    <NGridItem span="1 l:3">
      <NCard v-if="def" size="small" :title="def.name">
        <template #header-extra>
          <NSpace :size="6" align="center">
            <NTag size="small" :bordered="false" type="info">{{ t(`cos.kinds.${def.kind}`) }}</NTag>
            <NButton size="small" type="error" quaternary @click="removeDef">
              <template #icon><NIcon><Trash2 /></NIcon></template>
              {{ t('cos.delete') }}
            </NButton>
          </NSpace>
        </template>
        <p class="muted mono dn">{{ def.dn }}</p>
        <p class="hint">{{ t(`cos.kindHints.${def.kind}`) }}</p>
        <dl class="facts">
          <template v-if="def.description"><dt>{{ t('cos.description') }}</dt><dd>{{ def.description }}</dd></template>
          <dt>{{ t('cos.attributes') }}</dt><dd class="mono">{{ def.attributes.join('; ') }}</dd>
          <template v-if="def.templateDN"><dt>{{ def.kind === 'classic' ? t('cos.container') : t('cos.template') }}</dt><dd class="mono">{{ def.templateDN }}</dd></template>
          <template v-if="def.specifier"><dt>{{ t('cos.specifier') }}</dt><dd class="mono">{{ def.specifier }}</dd></template>
          <template v-if="def.indirectSpecifier"><dt>{{ t('cos.indirectSpecifier') }}</dt><dd class="mono">{{ def.indirectSpecifier }}</dd></template>
        </dl>

        <template v-if="def.kind !== 'indirect'">
          <div class="sub-head">
            <h3 class="sub">{{ t('cos.templates') }}</h3>
            <NButton v-if="canAddTemplate" size="small" @click="newTemplate">
              <template #icon><NIcon><Plus /></NIcon></template>
              {{ t('cos.addTemplate') }}
            </NButton>
          </div>
          <p v-if="!def.templates.length" class="muted">{{ t('cos.noTemplates') }}</p>
          <div v-for="tpl in def.templates" :key="tpl.dn" class="tpl">
            <div class="tpl-head">
              <span class="tpl-name">{{ tpl.name }}</span>
              <NTag v-if="tpl.priority !== undefined" size="tiny" :bordered="false">{{ t('cos.priority') }} {{ tpl.priority }}</NTag>
              <span class="spacer" />
              <NButton size="tiny" quaternary :title="t('cos.editTemplate')" :aria-label="t('cos.editTemplate')" @click="editTemplate(tpl)">
                <template #icon><NIcon><Pencil /></NIcon></template>
              </NButton>
              <NButton size="tiny" quaternary type="error" :title="t('cos.deleteTemplate')" :aria-label="t('cos.deleteTemplate')" @click="removeTemplate(tpl)">
                <template #icon><NIcon><Trash2 /></NIcon></template>
              </NButton>
            </div>
            <dl class="facts">
              <template v-for="a in attrNames(def)" :key="a">
                <dt class="mono">{{ a }}</dt>
                <dd>{{ (tpl.values[a] ?? []).join(', ') || '—' }}</dd>
              </template>
            </dl>
          </div>
        </template>
      </NCard>
      <NCard v-else size="small"><NEmpty :description="t('cos.pick')" /></NCard>
    </NGridItem>
  </NGrid>

  <NModal
    :show="!!editing"
    preset="card"
    :title="editing?.isNew ? t('cos.addTemplate') : t('cos.editTemplate')"
    :style="{ width: 'min(600px, 94vw)' }"
    :mask-closable="!saving"
    @update:show="(v: boolean) => !v && (editing = undefined)"
  >
    <NForm v-if="editing && def" label-placement="top" @submit.prevent="saveTemplate">
      <NFormItem v-if="editing.isNew && def.kind === 'classic'" :label="t('cos.templateValue', { attr: def.specifier ?? '' })" :feedback="t('cos.templateValueHint')">
        <NInput v-model:value="editing.name" />
      </NFormItem>
      <p class="muted mono">{{ templateDN || '…' }}</p>
      <NFormItem :label="t('cos.priority')" :feedback="t('cos.priorityHint')">
        <NInputNumber v-model:value="editing.priority" :min="0" clearable class="prio" />
      </NFormItem>
      <NFormItem v-for="a in attrNames(def)" :key="a" :label="a" :feedback="t('cos.valuesHint')">
        <NInput v-model:value="editing.values[a]" type="textarea" :autosize="{ minRows: 1, maxRows: 6 }" />
      </NFormItem>
    </NForm>
    <NAlert v-if="saveError" type="error">{{ saveError.summary }}<div v-if="saveError.detail" class="mono">{{ saveError.detail }}</div></NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="saving" @click="editing = undefined">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!templateDN" @click="saveTemplate">{{ t('pages._save') }}</NButton>
      </NSpace>
    </template>
  </NModal>

  <NModal v-model:show="creating" preset="card" :title="t('cos.new')" :style="{ width: 'min(640px, 94vw)' }" :mask-closable="!busy">
    <NForm label-placement="top" @submit.prevent="create">
      <NFormItem :label="t('cos.kind')" :feedback="t(`cos.kindHints.${draft.kind}`)">
        <NRadioGroup v-model:value="draft.kind">
          <NRadioButton v-for="k in ['pointer', 'classic', 'indirect']" :key="k" :value="k">{{ t(`cos.kinds.${k}`) }}</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem :label="t('cos.name')"><NInput v-model:value="draft.name" /></NFormItem>
      <NFormItem :label="t('cos.parent')" :feedback="t('cos.parentHint')">
        <NInput v-model:value="draft.parent" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('cos.attributes')" :feedback="t('cos.attributesHint')">
        <NDynamicTags v-model:value="draft.attributes" />
      </NFormItem>
      <NFormItem v-if="draft.kind !== 'indirect'" :label="draft.kind === 'classic' ? t('cos.container') : t('cos.template')">
        <NInput v-model:value="draft.templateDN" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem v-if="draft.kind === 'classic'" :label="t('cos.specifier')" :feedback="t('cos.specifierHint')">
        <NInput v-model:value="draft.specifier" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem v-if="draft.kind === 'indirect'" :label="t('cos.indirectSpecifier')" :feedback="t('cos.indirectHint')">
        <NInput v-model:value="draft.indirectSpecifier" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('cos.description')"><NInput v-model:value="draft.description" /></NFormItem>
    </NForm>
    <NAlert v-if="createError" type="error">{{ createError.summary }}<div v-if="createError.detail" class="mono">{{ createError.detail }}</div></NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="creating = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="busy" :disabled="!draft.name.trim() || !draft.attributes.length" @click="create">{{ t('create.create') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.item:hover {
  background: v-bind('vars.hoverColor');
}

.item.active {
  background: color-mix(in srgb, v-bind('vars.primaryColor') 12%, transparent);
}

.item-text {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.muted {
  font-size: 12px;
  color: v-bind('vars.textColor3');
  overflow-wrap: anywhere;
}

.dn {
  margin: 0 0 6px;
}

.hint {
  margin: 0 0 12px;
  font-size: 13px;
  color: v-bind('vars.textColor2');
}

.facts {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 4px 16px;
  margin: 0;
  font-size: 13px;
}

.facts dt {
  color: v-bind('vars.textColor3');
}

.facts dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.sub-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 18px 0 8px;
}

.sub {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}

.tpl {
  padding: 8px 0;
  border-top: 1px solid v-bind('vars.dividerColor');
}

.tpl-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.tpl-name {
  font-weight: 500;
}

.spacer {
  flex: 1;
}

.prio {
  width: 160px;
}
</style>
