<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import {
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NIcon,
  NInput,
  NSelect,
  NSpace,
  useMessage,
  useThemeVars,
  type DataTableColumns,
  type SelectOption,
} from 'naive-ui'
import { UserMinus, UserPlus } from '@lucide/vue'
import { api, call, type Entry } from '@/api/client'
import { localizeError } from '@/i18n/errors'
import { ancestorsUnder } from '@/ldap/dn'
import { quickFilter } from '@/ldap/filter'
import { useServerStore } from '@/stores/server'

/** member (groupOfNames) or uniqueMember (groupOfUniqueNames). */
const props = defineProps<{ entry: Entry; attr: string }>()
const emit = defineEmits<{ changed: [entry: Entry] }>()

const { t } = useI18n()
const message = useMessage()
const vars = useThemeVars()
const server = useServerStore()

const members = computed(
  () => props.entry.attributes.find((a) => a.name.toLowerCase() === props.attr.toLowerCase())?.values ?? [],
)
const filter = ref('')
const shown = computed(() => {
  const f = filter.value.trim().toLowerCase()
  return (f ? members.value.filter((m) => m.toLowerCase().includes(f)) : members.value).map((dn) => ({ dn }))
})

async function modify(op: 'add' | 'delete', values: string[]) {
  return call(
    api.PATCH('/api/v1/entries', {
      params: { query: { dn: props.entry.dn } },
      body: { changes: [{ op, attr: props.attr, values }] },
    }),
  )
}

async function remove(dn: string) {
  try {
    emit('changed', await modify('delete', [dn]))
    message.success(t('groups.removed'))
  } catch (e) {
    message.error(`${t('groups.error')}: ${localizeError(e).summary}`)
  }
}

const columns = computed<DataTableColumns<{ dn: string }>>(() => [
  {
    key: 'dn',
    title: 'DN',
    render: (row) => h(RouterLink, { to: { path: '/browser', query: { dn: row.dn } }, class: 'mono dn-link' }, () => row.dn),
  },
  {
    key: 'actions',
    title: '',
    width: 48,
    render: (row) =>
      h(
        NButton,
        { size: 'tiny', quaternary: true, title: t('groups.remove'), 'aria-label': t('groups.remove'), onClick: () => remove(row.dn) },
        { icon: () => h(NIcon, null, () => h(UserMinus)) },
      ),
  },
])

// Member picker: searches the suffix that contains the group.
const suffix = computed(
  () => server.info?.server.namingContexts.find((nc) => ancestorsUnder(props.entry.dn, nc) !== null) ?? props.entry.parent,
)
const options = ref<SelectOption[]>([])
const picked = ref<string[]>([])
const searching = ref(false)
let seq = 0
let timer: ReturnType<typeof setTimeout> | undefined

// Debounced: one request after the user pauses typing.
function onSearch(q: string) {
  clearTimeout(timer)
  timer = setTimeout(() => find(q), 300)
}

async function find(q: string) {
  if (q.trim().length < 2) {
    options.value = []
    return
  }
  const my = ++seq
  searching.value = true
  try {
    const res = await call(
      api.POST('/api/v1/search', {
        body: { base: suffix.value, scope: 'sub', filter: quickFilter(q), attrs: ['cn', 'uid'], limit: 25 },
      }),
    )
    if (my !== seq) return // a newer query is running
    const present = new Set(members.value.map((m) => m.toLowerCase()))
    options.value = res.entries
      .filter((e) => !present.has(e.dn.toLowerCase()) && e.dn.toLowerCase() !== props.entry.dn.toLowerCase())
      .map((e) => {
        const cn = e.attributes.find((a) => a.name.toLowerCase() === 'cn')?.values[0]
        return { value: e.dn, label: cn ? `${cn} — ${e.dn}` : e.dn }
      })
  } catch {
    options.value = []
  } finally {
    if (my === seq) searching.value = false
  }
}

async function add() {
  if (!picked.value.length) return
  try {
    const n = picked.value.length
    emit('changed', await modify('add', picked.value))
    message.success(t('groups.added', { n }, n))
    picked.value = []
    options.value = []
  } catch (e) {
    message.error(`${t('groups.error')}: ${localizeError(e).summary}`)
  }
}
</script>

<template>
  <NCard size="small" class="card">
    <template #header>
      {{ t('groups.members') }}
      <span class="count">{{ t('groups.memberCount', { n: members.length }, members.length) }}</span>
    </template>
    <NSpace vertical :size="10">
      <div class="add">
        <NSelect
          v-model:value="picked"
          multiple
          filterable
          remote
          clearable
          :options="options"
          :loading="searching"
          :placeholder="t('groups.addPlaceholder')"
          @search="onSearch"
        />
        <NButton type="primary" :disabled="!picked.length" @click="add">
          <template #icon><NIcon><UserPlus /></NIcon></template>
          {{ t('groups.add') }}
        </NButton>
      </div>
      <NInput v-if="members.length > 10" v-model:value="filter" size="small" clearable :placeholder="t('groups.filter')" />
      <NDataTable
        :columns="columns"
        :data="shown"
        :row-key="(r: { dn: string }) => r.dn"
        size="small"
        :pagination="shown.length > 50 ? { pageSize: 50 } : false"
      >
        <template #empty><NEmpty :description="t('groups.none')" size="small" /></template>
      </NDataTable>
    </NSpace>
  </NCard>
</template>

<style scoped>
.card {
  margin-top: 12px;
}

.count {
  margin-left: 8px;
  font-size: 13px;
  font-weight: normal;
  color: v-bind('vars.textColor3');
}

.add {
  display: flex;
  gap: 8px;
}

:deep(.dn-link) {
  color: v-bind('vars.primaryColor');
  text-decoration: none;
  overflow-wrap: anywhere;
}
</style>
