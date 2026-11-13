<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NIcon,
  NInput,
  NSelect,
  NSpace,
  NTable,
  NTag,
  NTooltip,
  useMessage,
  useThemeVars,
  type SelectOption,
} from 'naive-ui'
import { CircleAlert, Minus, Plus, Trash2, Upload } from '@lucide/vue'
import { api, ApiError, call, type Entry } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { ancestorsUnder, escapeRDNValue } from '@/ldap/dn'
import { diffEntry, ensureRequired, toEditAttrs, validateRows, type EditAttr, type RowProblem } from '@/ldap/edit'
import { applyDefaults, nextNumberAttrs } from '@/ldap/templates'
import { base64Size, formatBytes } from '@/ldap/values'
import { useSchemaStore } from '@/stores/schema'
import { useServerStore } from '@/stores/server'

/** Create mode: a new entry under parent, pre-filled from a template. */
export interface CreateSpec {
  parent: string
  objectClasses: string[]
  rdn: string
  custom: boolean
  password: boolean
  fields: Array<{ name: string; default?: string }>
}

const props = defineProps<{ entry?: Entry; create?: CreateSpec }>()
const emit = defineEmits<{ saved: [entry: Entry]; created: [entry: Entry]; cancel: []; reload: [] }>()

const { t } = useI18n()
const message = useMessage()
const vars = useThemeVars()
const schema = useSchemaStore()
const server = useServerStore()

const isBinaryKind = (name: string) => schema.attr(name)?.kind === 'binary'
// memberOf is maintained by the server's memberOf plugin.
const pluginManaged = new Set(['memberof'])
const editable = (name: string) => {
  const a = schema.attr(name)
  if (pluginManaged.has(name.split(';')[0]!.toLowerCase())) return false
  return !a || (!a.noUserModification && a.kind !== 'password')
}

const original = computed(() => (props.entry ? toEditAttrs(props.entry.attributes, (a) => editable(a.name)) : []))
const classes = ref<string[]>([])
const rows = ref<EditAttr[]>([])
const problems = ref<RowProblem[]>([])
const saving = ref(false)
const error = ref<LocalizedError & { conflict?: boolean }>()

// Create-mode state.
const parent = ref('')
const rdnAttr = ref('')
const password = ref('')
const password2 = ref('')
const nextNumbers = ref<Record<string, number>>({})
let autoFilled: Record<string, string> = {}
const defaults = computed(() =>
  Object.fromEntries((props.create?.fields ?? []).filter((f) => f.default).map((f) => [f.name, f.default!])),
)

function reset() {
  if (props.create) {
    classes.value = [...props.create.objectClasses]
    parent.value = props.create.parent
    rdnAttr.value = props.create.rdn
    password.value = password2.value = ''
    autoFilled = {}
    const fields = props.create.fields.map((f) => ({ name: f.name, values: [''], binary: isBinaryKind(f.name) }))
    if (!fields.some((f) => f.name.toLowerCase() === props.create!.rdn.toLowerCase())) {
      fields.unshift({ name: props.create.rdn, values: [''], binary: false })
    }
    rows.value = ensureRequired(fields, allowed.value.must, isBinaryKind)
  } else if (props.entry) {
    classes.value = [...props.entry.objectClasses]
    rows.value = ensureRequired(
      original.value.map((r) => ({ ...r, values: [...r.values] })),
      allowed.value.must,
      isBinaryKind,
    )
  }
  problems.value = []
  error.value = undefined
}

const allowed = computed(() => schema.allowed(classes.value))
const mustSet = computed(() => new Set(allowed.value.must.map((m) => m.toLowerCase())))
const isRequired = (name: string) => !name.includes(';') && mustSet.value.has(name.toLowerCase())
const isAllowed = (name: string) =>
  allowed.value.any ||
  mustSet.value.has(name.toLowerCase()) ||
  allowed.value.may.some((m) => m.toLowerCase() === name.toLowerCase())

watch(() => props.entry ?? props.create, reset, { immediate: true })

// Template defaults follow the fields they reference until the user edits them.
watch(
  [rows, nextNumbers],
  () => {
    if (props.create) autoFilled = applyDefaults(rows.value, defaults.value, autoFilled, (a) => nextNumbers.value[a])
  },
  { deep: true },
)

/** The naming context that contains the parent: scope for next-number suggestions. */
const suffixOfParent = computed(() => {
  const ncs = server.info?.server.namingContexts ?? []
  return ncs.find((nc) => ancestorsUnder(parent.value, nc) !== null) ?? parent.value
})

onMounted(async () => {
  if (!props.create) return
  for (const attr of nextNumberAttrs(Object.values(defaults.value))) {
    try {
      const res = await call(api.GET('/api/v1/entries/next-number', { params: { query: { base: suffixOfParent.value, attr } } }))
      nextNumbers.value = { ...nextNumbers.value, [attr]: res.value }
    } catch {
      // A suggestion only: the field simply stays empty.
    }
  }
})

const rdnValue = computed(() => rows.value.find((r) => r.name.toLowerCase() === rdnAttr.value.toLowerCase())?.values[0] ?? '')
const newDN = computed(() => `${rdnAttr.value}=${escapeRDNValue(rdnValue.value)}${parent.value ? ',' + parent.value : ''}`)
const rdnOptions = computed<SelectOption[]>(() =>
  [...allowed.value.must, ...allowed.value.may]
    .filter((n) => n.toLowerCase() !== 'objectclass' && editable(n) && !isBinaryKind(n))
    .map((n) => ({ label: n, value: n })),
)

// Choosing another naming attribute adds its row if needed.
watch(rdnAttr, (a) => {
  if (a && !rows.value.some((r) => r.name.toLowerCase() === a.toLowerCase())) {
    rows.value.unshift({ name: a, values: [''], binary: false })
  }
})

// New object classes may bring new required attributes.
watch(
  () => allowed.value.must.join(),
  () => (rows.value = ensureRequired(rows.value, allowed.value.must, isBinaryKind)),
)

const present = computed(() => new Set(rows.value.map((r) => r.name.toLowerCase())))

const addAttrOptions = computed<SelectOption[]>(() => {
  const names = allowed.value.any
    ? (schema.schema?.attributeTypes ?? [])
        .filter((a) => !a.noUserModification && (a.usage ?? 'userApplications') === 'userApplications')
        .map((a) => a.names[0] ?? a.oid)
    : [...allowed.value.must, ...allowed.value.may]
  return names
    .filter((n) => !present.value.has(n.toLowerCase()) && editable(n) && n.toLowerCase() !== 'objectclass')
    .sort((a, b) => a.localeCompare(b))
    .map((n) => ({ label: n, value: n }))
})

const classOptions = computed<SelectOption[]>(() => {
  const have = new Set(classes.value.map((c) => c.toLowerCase()))
  return (schema.schema?.objectClasses ?? [])
    .filter((c) => !c.obsolete && !c.names.some((n) => have.has(n.toLowerCase())))
    .map((c) => ({ label: `${c.names[0] ?? c.oid} · ${t(`edit.classKind.${c.kind}`)}`, value: c.names[0] ?? c.oid }))
    .sort((a, b) => String(a.value).localeCompare(String(b.value)))
})

function addAttribute(name: string) {
  rows.value.push({ name, values: [''], binary: isBinaryKind(name) })
}

function removeClass(c: string) {
  classes.value = classes.value.filter((x) => x !== c)
}

function addClass(c: string) {
  classes.value = [...classes.value, c]
}

const singleValue = (name: string) => !!schema.attr(name)?.singleValue
const kindOf = (name: string) => schema.attr(name)?.kind ?? 'string'

function problemsFor(row: string) {
  return problems.value.filter((p) => p.row === row)
}

function problemText(p: RowProblem, row: string) {
  switch (p.kind) {
    case 'required':
      return t('edit.emptyRequired', { attr: row })
    case 'singleValue':
      return t('edit.singleValue')
    case 'integer':
      return t('edit.invalidInteger')
    case 'notAllowed':
      return t('edit.notAllowed')
  }
}

function upload(row: EditAttr, index: number) {
  const input = document.createElement('input')
  input.type = 'file'
  input.onchange = () => {
    const file = input.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const url = String(reader.result)
      row.values[index] = url.slice(url.indexOf(',') + 1) // strip "data:...;base64,"
    }
    reader.readAsDataURL(file)
  }
  input.click()
}

const passwordMismatch = computed(() => password2.value !== '' && password.value !== password2.value)

async function save() {
  problems.value = validateRows(rows.value, allowed.value.must, isAllowed, (n) => schema.attr(n))
  if (problems.value.length) return
  if (props.create) return createEntry()
  const changes = diffEntry(props.entry!.objectClasses, classes.value, original.value, rows.value)
  if (!changes.length) {
    message.info(t('edit.noChanges'))
    emit('cancel')
    return
  }
  saving.value = true
  error.value = undefined
  try {
    const updated = await call(
      api.PATCH('/api/v1/entries', {
        params: { query: { dn: props.entry!.dn }, header: { 'If-Match': props.entry!.etag } },
        body: { changes },
      }),
    )
    message.success(t('edit.saved'))
    emit('saved', updated)
  } catch (e) {
    error.value = { ...localizeError(e), conflict: e instanceof ApiError && e.status === 412 }
  } finally {
    saving.value = false
  }
}

async function createEntry() {
  if (!rdnValue.value) {
    error.value = { summary: t('create.rdnRequired', { attr: rdnAttr.value }) }
    return
  }
  if (!classes.value.some((c) => schema.objectClass(c)?.kind === 'structural')) {
    error.value = { summary: t('create.classesRequired') }
    return
  }
  if (passwordMismatch.value) return
  const attributes = [
    { name: 'objectClass', values: classes.value },
    ...rows.value
      .map((r) => ({ name: r.name, values: r.values.filter((v) => v !== ''), binary: r.binary || undefined }))
      .filter((r) => r.values.length),
  ]
  if (props.create?.password && password.value) attributes.push({ name: 'userPassword', values: [password.value] })
  saving.value = true
  error.value = undefined
  try {
    const created = await call(api.POST('/api/v1/entries', { body: { dn: newDN.value, attributes } }))
    message.success(t('create.created', { dn: created.dn }))
    emit('created', created)
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="editor">
    <NAlert v-if="error" type="error" :title="error.conflict ? undefined : t('edit.saveError')" class="block">
      <template v-if="error.conflict">
        {{ t('edit.conflict') }}
        <NButton size="small" class="reload" @click="emit('reload')">{{ t('edit.reload') }}</NButton>
      </template>
      <template v-else>
        {{ error.summary }}
        <div v-if="error.detail" class="mono muted">{{ error.detail }}</div>
      </template>
    </NAlert>

    <div v-if="create" class="create-head">
      <label>{{ t('create.parent') }}</label>
      <NInput v-model:value="parent" size="small" :input-props="{ class: 'mono', spellcheck: false, 'aria-label': t('create.parent') }" />
      <label>{{ t('create.rdnAttribute') }}</label>
      <NSelect v-model:value="rdnAttr" size="small" filterable :options="rdnOptions" class="rdn-select" />
      <label>{{ t('create.dnPreview') }}</label>
      <code class="dn-preview">{{ newDN }}</code>
    </div>

    <div class="classes">
      <NTag
        v-for="c in classes"
        :key="c"
        size="small"
        type="info"
        :bordered="false"
        :closable="c.toLowerCase() !== 'top'"
        @close="removeClass(c)"
      >
        {{ c }}
      </NTag>
      <NSelect
        class="add-class"
        size="tiny"
        filterable
        :value="null"
        :options="classOptions"
        :placeholder="t('edit.addObjectClass')"
        @update:value="addClass"
      />
    </div>

    <NTable size="small" :bordered="false" :single-line="false">
      <tbody>
        <tr v-for="row in rows" :key="row.name">
          <td class="name-col">
            <span class="attr-name">{{ row.name }}</span>
            <NTooltip v-if="isRequired(row.name)">
              <template #trigger><span class="required">*</span></template>
              {{ t('edit.required') }}
            </NTooltip>
            <NTooltip v-if="!isAllowed(row.name.split(';')[0]!)">
              <template #trigger>
                <NIcon :color="vars.warningColor" class="warn"><CircleAlert /></NIcon>
              </template>
              {{ t('edit.notAllowed') }}
            </NTooltip>
          </td>
          <td>
            <div v-for="(_, i) in row.values" :key="i" class="value-row">
              <template v-if="row.binary">
                <span class="muted">
                  {{
                    row.values[i]
                      ? (row.original?.includes(row.values[i]!) ? t('browser.binary', { size: formatBytes(base64Size(row.values[i]!), currentLocale) }) : t('edit.newBinary', { size: formatBytes(base64Size(row.values[i]!), currentLocale) }))
                      : '—'
                  }}
                </span>
                <NButton size="tiny" secondary @click="upload(row, i)">
                  <template #icon><NIcon><Upload /></NIcon></template>
                  {{ row.values[i] ? t('edit.replaceFile') : t('edit.upload') }}
                </NButton>
              </template>
              <NSelect
                v-else-if="kindOf(row.name) === 'boolean'"
                v-model:value="row.values[i]"
                size="small"
                class="bool"
                :options="[{ label: 'TRUE', value: 'TRUE' }, { label: 'FALSE', value: 'FALSE' }]"
              />
              <NInput
                v-else
                v-model:value="row.values[i]"
                size="small"
                :type="row.values[i]!.includes('\n') ? 'textarea' : 'text'"
                :autosize="row.values[i]!.includes('\n') ? { minRows: 2, maxRows: 10 } : undefined"
                :input-props="{ class: kindOf(row.name) === 'dn' ? 'mono' : undefined, spellcheck: false, 'aria-label': row.name }"
              />
              <NButton
                v-if="row.values.length > 1"
                size="tiny"
                quaternary
                :title="t('edit.removeValue')"
                @click="row.values.splice(i, 1)"
              >
                <template #icon><NIcon><Minus /></NIcon></template>
              </NButton>
            </div>
            <div v-for="p in problemsFor(row.name)" :key="p.kind" class="problem">{{ problemText(p, row.name) }}</div>
          </td>
          <td class="actions-col">
            <NButton
              v-if="!singleValue(row.name)"
              size="tiny"
              quaternary
              :title="t('edit.addValue')"
              @click="row.values.push('')"
            >
              <template #icon><NIcon><Plus /></NIcon></template>
            </NButton>
            <NButton
              v-if="!isRequired(row.name)"
              size="tiny"
              quaternary
              :title="t('edit.removeAttribute')"
              @click="rows.splice(rows.indexOf(row), 1)"
            >
              <template #icon><NIcon><Trash2 /></NIcon></template>
            </NButton>
          </td>
        </tr>
      </tbody>
    </NTable>

    <div v-if="create?.password" class="create-head password">
      <label>{{ t('create.password') }}</label>
      <NInput
        v-model:value="password"
        size="small"
        type="password"
        show-password-on="click"
        :input-props="{ autocomplete: 'new-password', 'aria-label': t('create.password') }"
      />
      <label>{{ t('create.passwordConfirm') }}</label>
      <NInput
        v-model:value="password2"
        size="small"
        type="password"
        show-password-on="click"
        :status="passwordMismatch ? 'error' : undefined"
        :input-props="{ autocomplete: 'new-password' }"
      />
      <span v-if="passwordMismatch" class="problem span2">{{ t('password.mismatch') }}</span>
    </div>

    <div class="footer">
      <NSelect
        class="add-attr"
        size="small"
        filterable
        :value="null"
        :options="addAttrOptions"
        :placeholder="t('edit.addAttribute')"
        @update:value="addAttribute"
      />
      <NSpace>
        <NButton @click="emit('cancel')">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" @click="save">{{ create ? t('create.create') : t('edit.save') }}</NButton>
      </NSpace>
    </div>
  </div>
</template>

<style scoped>
.block {
  margin-bottom: 12px;
}

.create-head {
  display: grid;
  grid-template-columns: max-content 1fr;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 14px;
}

.create-head label {
  color: v-bind('vars.textColor2');
  font-size: 13px;
}

.create-head.password {
  margin-top: 14px;
}

.rdn-select {
  max-width: 280px;
}

.dn-preview {
  overflow-wrap: anywhere;
}

.span2 {
  grid-column: 2;
}

.reload {
  margin-left: 12px;
}

.classes {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
}

.add-class {
  width: 240px;
}

.name-col {
  width: 26%;
  min-width: 140px;
  vertical-align: top;
  padding-top: 12px;
}

.attr-name {
  font-weight: 500;
  overflow-wrap: anywhere;
}

.required {
  color: v-bind('vars.errorColor');
  margin-left: 2px;
  cursor: help;
}

.warn {
  margin-left: 6px;
  vertical-align: -2px;
  cursor: help;
}

.value-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.value-row + .value-row {
  margin-top: 6px;
}

.bool {
  width: 140px;
}

.actions-col {
  width: 72px;
  white-space: nowrap;
  vertical-align: top;
  padding-top: 10px;
}

.problem {
  margin-top: 4px;
  font-size: 12px;
  color: v-bind('vars.errorColor');
}

.footer {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 12px;
}

.add-attr {
  max-width: 320px;
}

.muted {
  color: v-bind('vars.textColor3');
}
</style>
