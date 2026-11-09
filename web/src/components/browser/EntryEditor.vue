<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
import { diffEntry, ensureRequired, toEditAttrs, validateRows, type EditAttr, type RowProblem } from '@/ldap/edit'
import { base64Size, formatBytes } from '@/ldap/values'
import { useSchemaStore } from '@/stores/schema'

const props = defineProps<{ entry: Entry }>()
const emit = defineEmits<{ saved: [entry: Entry]; cancel: []; reload: [] }>()

const { t } = useI18n()
const message = useMessage()
const vars = useThemeVars()
const schema = useSchemaStore()

const isBinaryKind = (name: string) => schema.attr(name)?.kind === 'binary'
const editable = (name: string) => {
  const a = schema.attr(name)
  return !a || (!a.noUserModification && a.kind !== 'password')
}

const original = computed(() => toEditAttrs(props.entry.attributes, (a) => editable(a.name)))
const classes = ref<string[]>([])
const rows = ref<EditAttr[]>([])
const problems = ref<RowProblem[]>([])
const saving = ref(false)
const error = ref<LocalizedError & { conflict?: boolean }>()

function reset() {
  classes.value = [...props.entry.objectClasses]
  rows.value = ensureRequired(
    original.value.map((r) => ({ ...r, values: [...r.values] })),
    allowed.value.must,
    isBinaryKind,
  )
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

watch(() => props.entry, reset, { immediate: true })

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

async function save() {
  problems.value = validateRows(rows.value, allowed.value.must, isAllowed, (n) => schema.attr(n))
  if (problems.value.length) return
  const changes = diffEntry(props.entry.objectClasses, classes.value, original.value, rows.value)
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
        params: { query: { dn: props.entry.dn }, header: { 'If-Match': props.entry.etag } },
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
                :input-props="{ class: kindOf(row.name) === 'dn' ? 'mono' : undefined, spellcheck: false }"
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
        <NButton type="primary" :loading="saving" @click="save">{{ t('edit.save') }}</NButton>
      </NSpace>
    </div>
  </div>
</template>

<style scoped>
.block {
  margin-bottom: 12px;
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
