<script setup lang="ts">
/** State of the accounts under a subtree: locks, lockouts, expired passwords (GET /accounts/report). */
import { computed, h, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NDataTable, NEmpty, NForm, NFormItem, NInput, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'

type Row = Schemas['AccountRow']

const { t } = useI18n()
const server = useServerStore()

const base = ref('')
// Server info may still be loading when the page is opened directly.
watch(
  () => server.info?.server.namingContexts[0],
  (s) => {
    if (s && !base.value) base.value = s
  },
  { immediate: true },
)
const filter = ref('')
const report = ref<Schemas['AccountReport']>()
const busy = ref(false)
const error = ref<LocalizedError>()
/** Shown states; empty: all. */
const only = ref<string>()

async function run() {
  busy.value = true
  error.value = undefined
  try {
    report.value = await call(api.GET('/api/v1/accounts/report', { params: { query: { base: base.value.trim(), filter: filter.value.trim() || undefined } } }))
    only.value = undefined
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}

const STATES = ['active', 'locked', 'locked-role', 'locked-out', 'expired', 'must-change']
const stateType = (s: string) => (s === 'active' ? 'success' : s === 'expired' || s === 'must-change' ? 'warning' : 'error')
const rows = computed(() => (report.value?.accounts ?? []).filter((a) => !only.value || a.states.includes(only.value)))
const fmt = (iso?: string) => (iso ? new Date(iso).toLocaleString(currentLocale.value) : '')

const columns = computed<DataTableColumns<Row>>(() => [
  {
    key: 'name',
    title: t('accountReport.account'),
    minWidth: 260,
    sorter: (a, b) => a.name.localeCompare(b.name),
    render: (a) =>
      h('div', null, [
        h(RouterLink, { to: { path: '/browser', query: { dn: a.dn } } }, () => a.name || a.dn),
        h('div', { class: 'mono', style: 'font-size:12px;opacity:.7;overflow-wrap:anywhere' }, a.dn),
      ]),
  },
  {
    key: 'states',
    title: t('accountReport.state'),
    width: 200,
    render: (a) => h(NSpace, { size: 4 }, () => a.states.map((s) => h(NTag, { size: 'small', bordered: false, type: stateType(s) }, () => t(`account.states.${s}`)))),
  },
  {
    key: 'passwordExpires',
    title: t('account.expires'),
    width: 160,
    sorter: (a, b) => (a.passwordExpires ?? '').localeCompare(b.passwordExpires ?? ''),
    render: (a) => fmt(a.passwordExpires),
  },
  {
    key: 'lastLogin',
    title: t('account.lastLogin'),
    width: 160,
    sorter: (a, b) => (a.lastLogin ?? '').localeCompare(b.lastLogin ?? ''),
    render: (a) => fmt(a.lastLogin),
  },
  { key: 'failedBinds', title: t('account.failed'), width: 100, sorter: (a, b) => (a.failedBinds ?? 0) - (b.failedBinds ?? 0) },
])
</script>

<template>
  <NCard size="small" :title="t('accountReport.title')">
    <p class="hint">{{ t('accountReport.description') }}</p>
    <NForm label-placement="top" class="form" @submit.prevent="run">
      <NFormItem :label="t('accountReport.base')" class="base">
        <NInput v-model:value="base" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('accountReport.filter')" class="filter">
        <NInput v-model:value="filter" :placeholder="t('accountReport.filterDefault')" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem label=" ">
        <NButton type="primary" attr-type="submit" :loading="busy" :disabled="!base.trim()">{{ t('accountReport.run') }}</NButton>
      </NFormItem>
    </NForm>
    <NAlert v-if="error" type="error">{{ error.summary }}<div v-if="error.detail" class="mono">{{ error.detail }}</div></NAlert>
    <template v-if="report">
      <NSpace class="counts" :size="6">
        <NTag :checked="!only" checkable @update:checked="only = undefined">{{ t('accountReport.all', { n: report.accounts.length }) }}</NTag>
        <template v-for="s in STATES" :key="s">
          <NTag v-if="report.counts[s]" checkable :checked="only === s" @update:checked="(v: boolean) => (only = v ? s : undefined)">
            {{ t(`account.states.${s}`) }}: {{ report.counts[s] }}
          </NTag>
        </template>
      </NSpace>
      <NAlert v-if="report.truncated" type="warning" class="counts">{{ t('accountReport.truncated', { n: report.accounts.length }) }}</NAlert>
      <NDataTable :columns="columns" :data="rows" size="small" :pagination="{ pageSize: 50 }" :scroll-x="880" :row-key="(a: Row) => a.dn">
        <template #empty><NEmpty size="small" :description="t('accountReport.none')" /></template>
      </NDataTable>
    </template>
  </NCard>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 12.5px;
  opacity: 0.75;
}

.form {
  display: flex;
  flex-wrap: wrap;
  gap: 0 12px;
}

.base {
  flex: 2 1 260px;
}

.filter {
  flex: 2 1 220px;
}

.counts {
  margin-bottom: 12px;
}
</style>
