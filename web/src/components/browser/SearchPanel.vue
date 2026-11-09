<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NSelect,
  NSpace,
  useThemeVars,
  type DataTableColumns,
} from 'naive-ui'
import { Download, Search } from '@lucide/vue'
import { api, call, exportUrl, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

type Hit = Schemas['SearchHit']

const props = defineProps<{ base: string }>()

const { t } = useI18n()
const vars = useThemeVars()

const base = ref(props.base)
const scope = ref<'base' | 'one' | 'sub'>('sub')
const filter = ref('(objectClass=*)')
const attrsText = ref('cn, uid, mail')
const limit = ref(500)
const busy = ref(false)
const error = ref<LocalizedError>()
const hits = ref<Hit[]>()
const truncated = ref(false)

const attrs = computed(() =>
  attrsText.value
    .split(',')
    .map((a) => a.trim())
    .filter(Boolean),
)

const scopeOptions = computed(() =>
  (['sub', 'one', 'base'] as const).map((s) => ({ value: s, label: t(`search.scopes.${s}`) })),
)

async function run() {
  busy.value = true
  error.value = undefined
  try {
    const res = await call(
      api.POST('/api/v1/search', {
        body: { base: base.value.trim(), scope: scope.value, filter: filter.value.trim(), attrs: attrs.value, limit: limit.value },
      }),
    )
    hits.value = res.entries
    truncated.value = res.truncated
  } catch (e) {
    hits.value = undefined
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}

// Columns: the DN plus each requested attribute (or all returned ones).
const columnNames = computed(() => {
  if (attrs.value.length) return attrs.value
  const names = new Map<string, string>()
  for (const h of hits.value ?? []) for (const a of h.attributes) names.set(a.name.toLowerCase(), a.name)
  return [...names.values()].slice(0, 12)
})

const columns = computed<DataTableColumns<Hit>>(() => [
  {
    key: 'dn',
    title: 'DN',
    minWidth: 260,
    render: (row) => h(RouterLink, { to: { path: '/browser', query: { dn: row.dn } }, class: 'mono dn-link' }, () => row.dn),
  },
  ...columnNames.value.map((name) => ({
    key: name,
    title: name,
    minWidth: 120,
    ellipsis: { tooltip: true },
    render: (row: Hit) => {
      const a = row.attributes.find((x) => x.name.toLowerCase() === name.toLowerCase())
      if (!a) return ''
      if (a.binary) return '(binary)'
      return a.values.join('; ')
    },
  })),
])

const exportQuery = computed(() => ({ base: base.value.trim(), scope: scope.value, filter: filter.value.trim(), attrs: attrs.value }))
</script>

<template>
  <NCard size="small" :title="t('search.title')">
    <NForm label-placement="left" label-width="auto" size="small" @submit.prevent="run">
      <NFormItem :label="t('search.base')">
        <NInput v-model:value="base" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('search.filter')">
        <NInput v-model:value="filter" :input-props="{ class: 'mono', spellcheck: false }" @keydown.enter.prevent="run" />
      </NFormItem>
      <NFormItem :label="t('search.attrs')" :feedback="t('search.attrsHint')">
        <NInput v-model:value="attrsText" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <div class="row">
        <NFormItem :label="t('search.scope')" :show-feedback="false">
          <NSelect v-model:value="scope" :options="scopeOptions" class="scope" />
        </NFormItem>
        <NFormItem :label="t('search.limit')" :show-feedback="false">
          <NInputNumber v-model:value="limit" :min="1" :max="5000" class="limit" />
        </NFormItem>
        <NButton type="primary" attr-type="submit" :loading="busy">
          <template #icon><NIcon><Search /></NIcon></template>
          {{ t('search.run') }}
        </NButton>
      </div>
    </NForm>

    <NAlert v-if="error" type="error" :title="t('search.error')" class="block">
      {{ error.summary }}
      <div v-if="error.detail" class="mono">{{ error.detail }}</div>
    </NAlert>

    <template v-if="hits">
      <div class="results-head">
        <span>{{ t('search.results', { n: hits.length }, hits.length) }}</span>
        <NSpace>
          <NButton tag="a" size="small" :href="exportUrl({ ...exportQuery, format: 'ldif' })" download>
            <template #icon><NIcon><Download /></NIcon></template>
            {{ t('search.exportLDIF') }}
          </NButton>
          <NButton
            tag="a"
            size="small"
            :disabled="!attrs.length"
            :title="attrs.length ? undefined : t('search.csvNeedsAttrs')"
            :href="attrs.length ? exportUrl({ ...exportQuery, format: 'csv' }) : undefined"
            download
          >
            <template #icon><NIcon><Download /></NIcon></template>
            {{ t('search.exportCSV') }}
          </NButton>
        </NSpace>
      </div>
      <NAlert v-if="truncated" type="warning" class="block">{{ t('search.truncated', { n: hits.length }) }}</NAlert>
      <NDataTable
        :columns="columns"
        :data="hits"
        :row-key="(r: Hit) => r.dn"
        size="small"
        virtual-scroll
        :max-height="480"
        :scroll-x="300 + columnNames.length * 160"
        :empty-text="t('search.none')"
      />
    </template>
  </NCard>
</template>

<style scoped>
.row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
  margin-bottom: 8px;
}

.scope {
  width: 200px;
}

.limit {
  width: 120px;
}

.block {
  margin: 12px 0;
}

.results-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 16px 0 8px;
}

:deep(.dn-link) {
  color: v-bind('vars.primaryColor');
  text-decoration: none;
}
</style>
