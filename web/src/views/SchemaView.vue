<script setup lang="ts">
/**
 * Schema browser: attribute types and object classes with search, details
 * and dependencies; user-defined definitions can be created, changed and
 * deleted.
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NGrid,
  NGridItem,
  NH1,
  NIcon,
  NInput,
  NRadioButton,
  NRadioGroup,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
  useThemeVars,
  type DataTableColumns,
} from 'naive-ui'
import { Pencil, Plus, Search, Trash2 } from '@lucide/vue'
import { api, call, type AttributeType, type ObjectClass, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useSchemaStore } from '@/stores/schema'
import AttributeTypeDialog from '@/components/schema/AttributeTypeDialog.vue'
import ObjectClassDialog from '@/components/schema/ObjectClassDialog.vue'
import PluginTaskButton from '@/components/settings/PluginTaskButton.vue'
import { useAuthStore } from '@/stores/auth'

type Kind = 'attr' | 'class'
type Row = { name: string; oid: string; user: boolean; desc?: string }

const { t } = useI18n()
const vars = useThemeVars()
const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const store = useSchemaStore()

const loadError = ref<LocalizedError>()
onMounted(() => store.load().catch((e) => (loadError.value = localizeError(e))))

/** Reload of the schema files (Schema Reload plug-in task); administrators only. */
const auth = useAuthStore()
const reloadTask = ref<Schemas['PluginTask']>()
onMounted(async () => {
  if (auth.readOnly) return
  const tasks = await call(api.GET('/api/v1/plugin-tasks')).catch(() => [])
  reloadTask.value = tasks.find((x) => x.id === 'schema-reload')
})

const kind = computed<Kind>({
  get: () => (route.query.kind === 'class' ? 'class' : 'attr'),
  set: (k) => router.replace({ path: '/schema', query: { kind: k } }),
})
const selectedName = computed(() => (typeof route.query.name === 'string' ? route.query.name : undefined))
function select(k: Kind, name: string) {
  router.replace({ path: '/schema', query: { kind: k, name } })
}

const query = ref('')
const onlyUser = ref(false)

const isUser = (origin?: string[]) => (origin ?? []).some((o) => o.toLowerCase() === 'user defined')
const nameOf = (x: { names: string[]; oid: string }) => x.names[0] ?? x.oid

const rows = computed<Row[]>(() => {
  const s = store.schema
  if (!s) return []
  const list: (AttributeType | ObjectClass)[] = kind.value === 'attr' ? s.attributeTypes : s.objectClasses
  const q = query.value.trim().toLowerCase()
  return list
    .filter((x) => !onlyUser.value || isUser(x.origin))
    .filter((x) => !q || x.oid.includes(q) || x.names.some((n) => n.toLowerCase().includes(q)) || (x.desc ?? '').toLowerCase().includes(q))
    .map((x) => ({ name: nameOf(x), oid: x.oid, user: isUser(x.origin), desc: x.desc }))
    .sort((a, b) => a.name.localeCompare(b.name))
})

const columns = computed<DataTableColumns<Row>>(() => [
  {
    key: 'name',
    title: t('schemaView.name'),
    render: (r) => [
      h('span', null, r.name),
      r.user ? h(NTag, { size: 'tiny', bordered: false, type: 'success', style: 'margin-left:6px' }, () => t('schemaView.user')) : null,
    ],
  },
  { key: 'oid', title: 'OID', className: 'mono oid', ellipsis: { tooltip: true } },
])
const rowProps = (r: Row) => ({
  class: r.name === selectedName.value ? 'selected' : undefined,
  style: 'cursor: pointer',
  onClick: () => select(kind.value, r.name),
})

// --- details ----------------------------------------------------------------

const attr = computed(() => (kind.value === 'attr' && selectedName.value ? store.attr(selectedName.value) : undefined))
const cls = computed(() => (kind.value === 'class' && selectedName.value ? store.objectClass(selectedName.value) : undefined))

const syntaxName = (oid?: string) => {
  const s = store.schema?.syntaxes.find((x) => x.oid === oid)
  return s?.desc ? `${s.desc} (${oid})` : oid
}

/** Classes that list the attribute, and attribute types derived from it. */
const attrUsers = computed(() => {
  const a = attr.value
  const s = store.schema
  if (!a || !s) return { must: [] as string[], may: [] as string[], subtypes: [] as string[] }
  const is = (n: string) => store.attr(n) === a
  return {
    must: s.objectClasses.filter((c) => c.must.some(is)).map(nameOf).sort(),
    may: s.objectClasses.filter((c) => c.may.some(is)).map(nameOf).sort(),
    subtypes: s.attributeTypes.filter((x) => x !== a && x.sup && is(x.sup)).map(nameOf).sort(),
  }
})

const classInfo = computed(() => {
  const c = cls.value
  const s = store.schema
  if (!c || !s) return undefined
  const chain = store.superclasses(nameOf(c)).slice(1).map(nameOf)
  const subclasses = s.objectClasses.filter((x) => (x.sup ?? []).some((n) => store.objectClass(n) === c)).map(nameOf).sort()
  const all = store.allowed([nameOf(c)])
  const own = new Set([...c.must, ...c.may].map((n) => (store.attr(n)?.names[0] ?? n).toLowerCase()))
  return {
    chain,
    subclasses,
    inheritedMust: all.must.filter((n) => !own.has(n.toLowerCase())),
    inheritedMay: all.may.filter((n) => !own.has(n.toLowerCase())),
  }
})

// --- create / edit / delete -------------------------------------------------

const attrDialog = ref(false)
const classDialog = ref(false)
const editing = ref(false)

function create() {
  editing.value = false
  if (kind.value === 'attr') attrDialog.value = true
  else classDialog.value = true
}
function edit() {
  editing.value = true
  if (kind.value === 'attr') attrDialog.value = true
  else classDialog.value = true
}
function onSaved(name: string) {
  message.success(t('schemaView.saved', { name }))
  select(kind.value, name)
}

function remove() {
  const k = kind.value
  const x = k === 'attr' ? attr.value : cls.value
  if (!x) return
  const name = nameOf(x)
  dialog.warning({
    title: t('schemaView.delete'),
    content: t('schemaView.deleteConfirm', { name }),
    positiveText: t('schemaView.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        if (k === 'attr') await call(api.DELETE('/api/v1/schema/attributetypes/{name}', { params: { path: { name: x.oid } } }))
        else await call(api.DELETE('/api/v1/schema/objectclasses/{name}', { params: { path: { name: x.oid } } }))
        await store.load(true)
        message.success(t('schemaView.deleted', { name }))
        router.replace({ path: '/schema', query: { kind: k } })
      } catch (e) {
        const err = localizeError(e)
        message.error(`${t('schemaView.deleteError')}: ${err.detail ?? err.summary}`, { duration: 10000 })
      }
    },
  })
}

const selectedUser = computed(() => isUser((attr.value ?? cls.value)?.origin))
</script>

<template>
  <div class="head">
    <NH1 class="title">{{ t('nav.schema') }}</NH1>
    <PluginTaskButton v-if="reloadTask" :task="reloadTask" />
  </div>
  <NAlert v-if="loadError" type="error">{{ loadError.summary }}</NAlert>

  <NGrid v-else cols="1 l:5" responsive="screen" :x-gap="16" :y-gap="16">
    <NGridItem span="1 l:2">
      <NCard size="small">
        <div class="filters">
          <NSpace justify="space-between" align="center">
            <NRadioGroup v-model:value="kind" size="small">
              <NRadioButton value="attr">{{ t('schemaView.attributes') }}</NRadioButton>
              <NRadioButton value="class">{{ t('schemaView.classes') }}</NRadioButton>
            </NRadioGroup>
            <NButton size="small" type="primary" @click="create">
              <template #icon><NIcon><Plus /></NIcon></template>
              {{ kind === 'attr' ? t('schemaView.newAttr') : t('schemaView.newClass') }}
            </NButton>
          </NSpace>
          <NInput v-model:value="query" size="small" clearable :placeholder="t('schemaView.search')">
            <template #prefix><NIcon><Search /></NIcon></template>
          </NInput>
          <label class="toggle">
            <NSwitch v-model:value="onlyUser" size="small" />
            {{ t('schemaView.onlyUser') }}
            <span class="count">{{ rows.length }}</span>
          </label>
        </div>
        <NDataTable
          :columns="columns"
          :data="rows"
          :row-props="rowProps"
          :row-key="(r: Row) => r.oid + r.name"
          :loading="!store.schema"
          size="small"
          virtual-scroll
          :max-height="'calc(100vh - 420px)'"
          :min-height="240"
        >
          <template #empty><NEmpty :description="t('schemaView.none')" size="small" /></template>
        </NDataTable>
      </NCard>
    </NGridItem>

    <NGridItem span="1 l:3">
      <NCard v-if="attr" size="small">
        <template #header>
          <span class="mono">{{ attr.names.join(', ') || attr.oid }}</span>
        </template>
        <template #header-extra>
          <NSpace v-if="selectedUser" :size="4">
            <NButton size="small" quaternary @click="edit"><template #icon><NIcon><Pencil /></NIcon></template>{{ t('schemaView.edit') }}</NButton>
            <NButton size="small" quaternary type="error" @click="remove"><template #icon><NIcon><Trash2 /></NIcon></template>{{ t('schemaView.delete') }}</NButton>
          </NSpace>
          <NTag v-else size="small" :bordered="false">{{ t('schemaView.builtIn') }}</NTag>
        </template>
        <p v-if="attr.desc" class="desc">{{ attr.desc }}</p>
        <dl class="facts">
          <dt>OID</dt><dd class="mono">{{ attr.oid }}</dd>
          <dt>{{ t('schemaView.syntax') }}</dt><dd>{{ syntaxName(attr.syntax) }}<span v-if="attr.maxLen" class="muted"> {{ '{' }}{{ attr.maxLen }}{{ '}' }}</span></dd>
          <template v-if="attr.sup">
            <dt>{{ t('schemaView.sup') }}</dt>
            <dd><a class="link mono" @click="select('attr', attr.sup)">{{ attr.sup }}</a></dd>
          </template>
          <template v-if="attr.equality"><dt>{{ t('schemaView.equality') }}</dt><dd class="mono">{{ attr.equality }}</dd></template>
          <template v-if="attr.ordering"><dt>{{ t('schemaView.ordering') }}</dt><dd class="mono">{{ attr.ordering }}</dd></template>
          <template v-if="attr.substr"><dt>{{ t('schemaView.substr') }}</dt><dd class="mono">{{ attr.substr }}</dd></template>
          <dt>{{ t('schemaView.values') }}</dt><dd>{{ attr.singleValue ? t('schemaView.single') : t('schemaView.multi') }}</dd>
          <template v-if="attr.usage && attr.usage !== 'userApplications'"><dt>{{ t('schemaView.usage') }}</dt><dd class="mono">{{ attr.usage }}</dd></template>
          <template v-if="attr.noUserModification"><dt>{{ t('schemaView.flags') }}</dt><dd>{{ t('schemaView.noUserMod') }}</dd></template>
          <template v-if="attr.obsolete"><dt>{{ t('schemaView.flags') }}</dt><dd>{{ t('schemaView.obsolete') }}</dd></template>
          <template v-if="attr.origin?.length"><dt>{{ t('schemaView.origin') }}</dt><dd>{{ attr.origin.join(', ') }}</dd></template>
        </dl>
        <h3 class="sub">{{ t('schemaView.usedBy') }}</h3>
        <dl class="facts">
          <dt>MUST</dt>
          <dd><a v-for="n in attrUsers.must" :key="n" class="link chip" @click="select('class', n)">{{ n }}</a><span v-if="!attrUsers.must.length" class="muted">—</span></dd>
          <dt>MAY</dt>
          <dd><a v-for="n in attrUsers.may" :key="n" class="link chip" @click="select('class', n)">{{ n }}</a><span v-if="!attrUsers.may.length" class="muted">—</span></dd>
          <dt>{{ t('schemaView.subtypes') }}</dt>
          <dd><a v-for="n in attrUsers.subtypes" :key="n" class="link chip" @click="select('attr', n)">{{ n }}</a><span v-if="!attrUsers.subtypes.length" class="muted">—</span></dd>
        </dl>
      </NCard>

      <NCard v-else-if="cls && classInfo" size="small">
        <template #header>
          <span class="mono">{{ cls.names.join(', ') || cls.oid }}</span>
          <NTag size="small" :bordered="false" class="kind">{{ t(`schemaView.kinds.${cls.kind}`) }}</NTag>
        </template>
        <template #header-extra>
          <NSpace v-if="selectedUser" :size="4">
            <NButton size="small" quaternary @click="edit"><template #icon><NIcon><Pencil /></NIcon></template>{{ t('schemaView.edit') }}</NButton>
            <NButton size="small" quaternary type="error" @click="remove"><template #icon><NIcon><Trash2 /></NIcon></template>{{ t('schemaView.delete') }}</NButton>
          </NSpace>
          <NTag v-else size="small" :bordered="false">{{ t('schemaView.builtIn') }}</NTag>
        </template>
        <p v-if="cls.desc" class="desc">{{ cls.desc }}</p>
        <dl class="facts">
          <dt>OID</dt><dd class="mono">{{ cls.oid }}</dd>
          <dt>{{ t('schemaView.chain') }}</dt>
          <dd><a v-for="n in classInfo.chain" :key="n" class="link chip" @click="select('class', n)">{{ n }}</a><span v-if="!classInfo.chain.length" class="muted">—</span></dd>
          <dt>{{ t('schemaView.subclasses') }}</dt>
          <dd><a v-for="n in classInfo.subclasses" :key="n" class="link chip" @click="select('class', n)">{{ n }}</a><span v-if="!classInfo.subclasses.length" class="muted">—</span></dd>
          <template v-if="cls.origin?.length"><dt>{{ t('schemaView.origin') }}</dt><dd>{{ cls.origin.join(', ') }}</dd></template>
        </dl>
        <h3 class="sub">{{ t('schemaView.attributes') }}</h3>
        <dl class="facts">
          <dt>MUST</dt>
          <dd>
            <a v-for="n in cls.must" :key="n" class="link chip strong" @click="select('attr', n)">{{ n }}</a>
            <a v-for="n in classInfo.inheritedMust" :key="'i' + n" class="link chip inherited" :title="t('schemaView.inherited')" @click="select('attr', n)">{{ n }}</a>
            <span v-if="!cls.must.length && !classInfo.inheritedMust.length" class="muted">—</span>
          </dd>
          <dt>MAY</dt>
          <dd>
            <a v-for="n in cls.may" :key="n" class="link chip" @click="select('attr', n)">{{ n }}</a>
            <a v-for="n in classInfo.inheritedMay" :key="'i' + n" class="link chip inherited" :title="t('schemaView.inherited')" @click="select('attr', n)">{{ n }}</a>
            <span v-if="!cls.may.length && !classInfo.inheritedMay.length" class="muted">—</span>
          </dd>
        </dl>
        <p v-if="classInfo.inheritedMust.length || classInfo.inheritedMay.length" class="muted legend">{{ t('schemaView.inheritedLegend') }}</p>
      </NCard>

      <NCard v-else size="small"><NEmpty :description="t('schemaView.pick')" /></NCard>
    </NGridItem>
  </NGrid>

  <AttributeTypeDialog v-model:show="attrDialog" :edit="editing ? attr : undefined" @saved="onSaved" />
  <ObjectClassDialog v-model:show="classDialog" :edit="editing ? cls : undefined" @saved="onSaved" />
</template>

<style scoped>
.title {
  margin: 0;
}

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 20px;
}

.filters {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 10px;
}

.toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}

.count {
  margin-left: auto;
  font-size: 12px;
  color: v-bind('vars.textColor3');
  font-variant-numeric: tabular-nums;
}

:deep(.selected td) {
  background: color-mix(in srgb, v-bind('vars.primaryColor') 12%, transparent) !important;
}

:deep(.oid) {
  font-size: 12px;
  color: v-bind('vars.textColor3');
}

.kind {
  margin-left: 8px;
}

.desc {
  margin: 0 0 12px;
  color: v-bind('vars.textColor2');
}

.facts {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 6px 16px;
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

.sub {
  margin: 18px 0 8px;
  font-size: 13px;
  font-weight: 600;
}

.link {
  color: v-bind('vars.primaryColor');
  cursor: pointer;
}

.link:hover {
  text-decoration: underline;
}

.chip {
  display: inline-block;
  margin: 0 10px 2px 0;
  font-family: var(--mono, monospace);
}

.strong {
  font-weight: 600;
}

.inherited {
  opacity: 0.6;
  font-style: italic;
}

.muted {
  color: v-bind('vars.textColor3');
}

.legend {
  margin: 10px 0 0;
  font-size: 12px;
}
</style>
