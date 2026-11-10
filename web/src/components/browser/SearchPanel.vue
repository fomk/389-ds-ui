<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import {
  NAlert,
  NButton,
  NCard,
  NCollapseTransition,
  NDataTable,
  NEmpty,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NSelect,
  NSpace,
  useThemeVars,
  type DataTableColumns,
} from 'naive-ui'
import { ChevronDown, ChevronRight, Download, Search } from '@lucide/vue'
import { api, call, exportUrl, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { quickFilter } from '@/ldap/filter'

type Hit = Schemas['SearchHit']

const props = defineProps<{ base: string }>()

const { t } = useI18n()
const vars = useThemeVars()

const base = ref(props.base)
const scope = ref<'base' | 'one' | 'sub'>('sub')
/** What the user types: plain text (name, uid, email) or an LDAP filter. */
const query = ref('')
const advanced = ref(false)
const queryInput = ref<InstanceType<typeof NInput>>()
onMounted(() => queryInput.value?.focus())

// Text starting with "(" is used as an LDAP filter as is; anything else
// becomes a name/uid/email match; empty finds everything below the base.
const filter = computed(() => {
  const q = query.value.trim()
  if (!q) return '(objectClass=*)'
  return q.startsWith('(') ? q : quickFilter(q)
})
const attrsText = ref('cn, uid, mail')
const limit = ref(500)
// The API caps interactive searches at 5000 entries; use export for more.
const limitOptions = [100, 500, 1000, 2000, 5000].map((n) => ({ value: n, label: String(n) }))
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
        body: { base: base.value.trim(), scope: scope.value, filter: filter.value, attrs: attrs.value, limit: limit.value },
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

const exportQuery = computed(() => ({ base: base.value.trim(), scope: scope.value, filter: filter.value, attrs: attrs.value }))
</script>

<template>
  <NCard size="small" :title="t('search.title')">
    <form class="quick" @submit.prevent="run">
      <NInput
        ref="queryInput"
        v-model:value="query"
        size="large"
        clearable
        :placeholder="t('search.quickPlaceholder')"
        :input-props="{ spellcheck: false }"
      >
        <template #prefix><NIcon><Search /></NIcon></template>
      </NInput>
      <NButton type="primary" size="large" attr-type="submit" :loading="busy">{{ t('search.run') }}</NButton>
    </form>
    <div class="under">
      <span class="muted">{{ t('search.inBase') }} <span class="mono">{{ base }}</span></span>
      <NButton text size="small" @click="advanced = !advanced">
        <template #icon><NIcon><ChevronDown v-if="advanced" /><ChevronRight v-else /></NIcon></template>
        {{ t('search.advanced') }}
      </NButton>
    </div>
    <div v-if="query.trim() && !query.trim().startsWith('(')" class="muted filter-preview">
      {{ t('search.filter') }}: <span class="mono">{{ filter }}</span>
    </div>

    <NCollapseTransition :show="advanced">
      <NForm label-placement="left" label-width="auto" size="small" class="advanced" @submit.prevent="run">
        <NFormItem :label="t('search.base')">
          <NInput v-model:value="base" :input-props="{ class: 'mono', spellcheck: false }" />
        </NFormItem>
        <NFormItem :label="t('search.attrs')" :feedback="t('search.attrsHint')">
          <NInput v-model:value="attrsText" :input-props="{ class: 'mono', spellcheck: false }" />
        </NFormItem>
        <div class="row">
          <NFormItem :label="t('search.scope')" :show-feedback="false">
            <NSelect v-model:value="scope" :options="scopeOptions" class="scope" />
          </NFormItem>
          <NFormItem :label="t('search.limit')" :show-feedback="false">
            <NSelect v-model:value="limit" :options="limitOptions" class="limit" />
          </NFormItem>
        </div>
      </NForm>
    </NCollapseTransition>

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
      >
        <template #empty><NEmpty :description="t('search.none')" size="small" /></template>
      </NDataTable>
    </template>
  </NCard>
</template>

<style scoped>
.quick {
  display: flex;
  gap: 8px;
}

.under {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 6px 2px 0;
  font-size: 12px;
}

.filter-preview {
  margin: 4px 2px 0;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.muted {
  color: v-bind('vars.textColor3');
}

.advanced {
  margin-top: 12px;
}

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
