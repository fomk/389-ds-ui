<script setup lang="ts">
/** Replication agreements of a suffix: status (refreshed), create, edit, init, delete. */
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NDataTable,
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
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  NTooltip,
  useDialog,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import { Pencil, Plus, RefreshCcw, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError, type LocalizedError } from '@/i18n/errors'

type Agreement = Schemas['Agreement']

const props = defineProps<{ suffix: string; role?: string }>()
const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()

const list = ref<Agreement[]>()
const error = ref<LocalizedError>()
async function load() {
  try {
    list.value = await call(api.GET('/api/v1/replication/agreements', { params: { query: { suffix: props.suffix } } }))
    error.value = undefined
  } catch (e) {
    error.value = localizeError(e)
  }
}
watch(() => props.suffix, load, { immediate: true })
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => (timer = setInterval(load, 5000)))
onUnmounted(() => clearInterval(timer))

const q = computed(() => ({ suffix: props.suffix }))
const fmt = (iso?: string) => (iso ? new Date(iso).toLocaleString(currentLocale.value) : '')
const stateType = { ok: 'success', error: 'error', busy: 'info', never: 'default', unknown: 'warning' } as const
const fail = (e: unknown) => {
  const err = localizeError(e)
  message.error(`${t('replication.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
}

async function toggle(a: Agreement, enabled: boolean) {
  try {
    await call(api.PATCH('/api/v1/replication/agreements/{name}', { params: { path: { name: a.name }, query: q.value }, body: { enabled } }))
    await load()
  } catch (e) {
    fail(e)
  }
}
function init(a: Agreement) {
  dialog.warning({
    title: t('replication.init'),
    content: t('replication.initConfirm', { host: `${a.host}:${a.port}` }),
    positiveText: t('replication.init'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.POST('/api/v1/replication/agreements/{name}/init', { params: { path: { name: a.name }, query: q.value } }))
        message.info(t('replication.initStarted'))
        await load()
      } catch (e) {
        fail(e)
      }
    },
  })
}
function remove(a: Agreement) {
  dialog.warning({
    title: t('replication.deleteAgreement'),
    content: t('replication.deleteAgreementConfirm', { name: a.name }),
    positiveText: t('replication.deleteAgreement'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/replication/agreements/{name}', { params: { path: { name: a.name }, query: q.value } }))
        await load()
      } catch (e) {
        fail(e)
      }
    },
  })
}

// --- dialog ---------------------------------------------------------------------
type Draft = {
  isNew: boolean
  name: string
  host: string
  port: number | null
  transport: 'LDAP' | 'StartTLS' | 'LDAPS'
  bindMethod: 'SIMPLE' | 'SSLCLIENTAUTH' | 'SASL/GSSAPI' | 'SASL/DIGEST-MD5'
  bindDN: string
  credentials: string
  schedule: string
  excluded: string[]
  description: string
}
const draft = ref<Draft>()
const saving = ref(false)
const saveError = ref<LocalizedError>()
function add() {
  saveError.value = undefined
  draft.value = {
    isNew: true,
    name: '',
    host: '',
    port: 389,
    transport: 'LDAP',
    bindMethod: 'SIMPLE',
    bindDN: 'cn=replication manager,cn=config',
    credentials: '',
    schedule: '',
    excluded: [],
    description: '',
  }
}
function edit(a: Agreement) {
  saveError.value = undefined
  draft.value = {
    isNew: false,
    name: a.name,
    host: a.host,
    port: a.port,
    transport: a.transport,
    bindMethod: a.bindMethod,
    bindDN: a.bindDN ?? '',
    credentials: '',
    schedule: a.schedule ?? '',
    excluded: [...a.excluded],
    description: a.description ?? '',
  }
}
async function save() {
  const d = draft.value
  if (!d) return
  saving.value = true
  saveError.value = undefined
  const body: Schemas['AgreementInput'] = {
    host: d.host.trim(),
    port: d.port ?? 0,
    transport: d.transport,
    bindMethod: d.bindMethod,
    bindDN: d.bindDN.trim(),
    schedule: d.schedule.trim(),
    excluded: d.excluded,
    description: d.description,
    ...(d.credentials ? { credentials: d.credentials } : {}),
  }
  try {
    if (d.isNew) {
      await call(api.POST('/api/v1/replication/agreements', { params: { query: q.value }, body: { ...body, name: d.name.trim() } }))
    } else {
      await call(api.PATCH('/api/v1/replication/agreements/{name}', { params: { path: { name: d.name }, query: q.value }, body }))
    }
    draft.value = undefined
    message.success(t('replication.agreementSaved'))
    await load()
  } catch (e) {
    saveError.value = localizeError(e)
  } finally {
    saving.value = false
  }
}
const transportOptions = ['LDAP', 'StartTLS', 'LDAPS'].map((v) => ({ label: v, value: v }))
const bindOptions = ['SIMPLE', 'SSLCLIENTAUTH', 'SASL/GSSAPI', 'SASL/DIGEST-MD5'].map((v) => ({ label: v, value: v }))

const icon = (c: unknown) => () => h(NIcon, null, () => h(c as never))
const columns = computed<DataTableColumns<Agreement>>(() => [
  {
    key: 'name',
    title: t('replication.agreement'),
    render: (a) => h('div', null, [h('div', { style: 'font-weight:500' }, a.name), h('div', { class: 'mono', style: 'font-size:12px;opacity:.75' }, `${a.transport.toLowerCase()}://${a.host}:${a.port}`)]),
  },
  {
    key: 'enabled',
    title: t('replication.enabled'),
    width: 90,
    render: (a) => h(NSwitch, { size: 'small', value: a.enabled, 'onUpdate:value': (v: boolean) => toggle(a, v) }),
  },
  {
    key: 'status',
    title: t('replication.lastUpdate'),
    render: (a) =>
      h(NTooltip, null, {
        trigger: () =>
          h('div', null, [
            h(NTag, { size: 'small', bordered: false, type: stateType[a.status.state] }, () =>
              a.status.inProgress ? t('replication.states.busy') : t(`replication.states.${a.status.state}`),
            ),
            h('div', { style: 'font-size:12px;opacity:.75;margin-top:2px' }, fmt(a.status.lastUpdateEnd)),
          ]),
        default: () => a.status.lastUpdate || t('replication.states.never'),
      }),
  },
  {
    key: 'init',
    title: t('replication.lastInit'),
    render: (a) =>
      h(NTooltip, null, {
        trigger: () =>
          h('div', null, [
            h(NTag, { size: 'small', bordered: false, type: stateType[a.status.initState] }, () => t(`replication.states.${a.status.initState}`)),
            h('div', { style: 'font-size:12px;opacity:.75;margin-top:2px' }, fmt(a.status.lastInitEnd)),
          ]),
        default: () => a.status.lastInit || t('replication.states.never'),
      }),
  },
  {
    key: 'actions',
    title: '',
    width: 104,
    render: (a) =>
      h('div', { class: 'row-actions' }, [
        h(NButton, { size: 'tiny', quaternary: true, title: t('replication.init'), 'aria-label': t('replication.init'), onClick: () => init(a) }, { icon: icon(RefreshCcw) }),
        h(NButton, { size: 'tiny', quaternary: true, title: t('replication.editAgreement'), 'aria-label': t('replication.editAgreement'), onClick: () => edit(a) }, { icon: icon(Pencil) }),
        h(NButton, { size: 'tiny', quaternary: true, type: 'error', title: t('replication.deleteAgreement'), 'aria-label': t('replication.deleteAgreement'), onClick: () => remove(a) }, { icon: icon(Trash2) }),
      ]),
  },
])
</script>

<template>
  <NAlert v-if="role === 'consumer'" type="info" class="block">{{ t('replication.consumerNoAgreements') }}</NAlert>
  <NSpace v-else class="block">
    <NButton type="primary" size="small" @click="add">
      <template #icon><NIcon><Plus /></NIcon></template>
      {{ t('replication.newAgreement') }}
    </NButton>
  </NSpace>
  <NAlert v-if="error" type="error">{{ error.summary }}</NAlert>
  <NDataTable v-else :columns="columns" :data="list ?? []" :loading="!list" size="small" :row-key="(a: Agreement) => a.name">
    <template #empty><NEmpty size="small" :description="t('replication.noAgreements')" /></template>
  </NDataTable>

  <NModal
    :show="!!draft"
    preset="card"
    :title="draft?.isNew ? t('replication.newAgreement') : `${t('replication.editAgreement')}: ${draft?.name}`"
    :style="{ width: 'min(720px, 94vw)' }"
    :mask-closable="!saving"
    @update:show="(v: boolean) => !v && (draft = undefined)"
  >
    <NForm v-if="draft" label-placement="top" @submit.prevent="save">
      <NFormItem v-if="draft.isNew" :label="t('replication.agreementName')"><NInput v-model:value="draft.name" placeholder="to-ds2" /></NFormItem>
      <NGrid cols="1 s:3" responsive="screen" :x-gap="12">
        <NGridItem span="1 s:2">
          <NFormItem :label="t('replication.host')"><NInput v-model:value="draft.host" :input-props="{ spellcheck: false }" /></NFormItem>
        </NGridItem>
        <NGridItem>
          <NFormItem :label="t('replication.port')"><NInputNumber v-model:value="draft.port" :min="1" :max="65535" /></NFormItem>
        </NGridItem>
        <NGridItem>
          <NFormItem :label="t('replication.transport')"><NSelect v-model:value="draft.transport" :options="transportOptions" /></NFormItem>
        </NGridItem>
        <NGridItem span="1 s:2">
          <NFormItem :label="t('replication.bindMethod')"><NSelect v-model:value="draft.bindMethod" :options="bindOptions" /></NFormItem>
        </NGridItem>
      </NGrid>
      <NFormItem :label="t('replication.consumerBindDN')"><NInput v-model:value="draft.bindDN" :input-props="{ class: 'mono', spellcheck: false }" /></NFormItem>
      <NFormItem :label="t('replication.credentials')" :feedback="draft.isNew ? undefined : t('replication.credentialsKeep')">
        <NInput v-model:value="draft.credentials" type="password" show-password-on="click" :input-props="{ autocomplete: 'new-password' }" />
      </NFormItem>
      <NFormItem :label="t('replication.schedule')" :feedback="t('replication.scheduleHint')">
        <NInput v-model:value="draft.schedule" :placeholder="t('replication.always')" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('replication.excluded')" :feedback="t('replication.excludedHint')">
        <NDynamicTags v-model:value="draft.excluded" />
      </NFormItem>
      <NFormItem :label="t('replication.description')"><NInput v-model:value="draft.description" /></NFormItem>
    </NForm>
    <NAlert v-if="saveError" type="error">{{ saveError.summary }}<div v-if="saveError.detail" class="mono">{{ saveError.detail }}</div></NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="saving" @click="draft = undefined">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!draft?.host.trim() || (draft?.isNew && !draft.name.trim())" @click="save">{{ t('pages._save') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.block {
  margin-bottom: 12px;
}

:deep(.row-actions) {
  display: grid;
  grid-template-columns: repeat(3, 28px);
}
</style>
