<script setup lang="ts">
/** Changelog, RUV (with CleanAllRUV) and conflicts of a replicated suffix. */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NEmpty, NForm, NFormItem, NInput, NInputNumber, NSpace, NTable, NTag, useDialog, useMessage, useThemeVars } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { useTasksStore } from '@/stores/tasks'

const props = defineProps<{ suffix: string; section: 'changelog' | 'ruv' | 'conflicts'; replicaId?: number; role?: string }>()
const { t } = useI18n()
const vars = useThemeVars()
const message = useMessage()
const dialog = useDialog()
const tasks = useTasksStore()
const q = computed(() => ({ suffix: props.suffix }))
const fail = (e: unknown) => {
  const err = localizeError(e)
  message.error(`${t('replication.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
}
const fmt = (iso?: string) => (iso ? new Date(iso).toLocaleString(currentLocale.value) : '—')

// --- changelog --------------------------------------------------------------------
const changelog = ref<Schemas['Changelog']>()
const cl = ref({ maxAge: '', maxEntries: null as number | null, trimInterval: null as number | null })
async function loadChangelog() {
  try {
    changelog.value = await call(api.GET('/api/v1/replication/changelog', { params: { query: q.value } }))
    cl.value = { maxAge: changelog.value.maxAge ?? '', maxEntries: changelog.value.maxEntries ?? null, trimInterval: changelog.value.trimInterval ?? null }
  } catch (e) {
    fail(e)
  }
}
async function saveChangelog() {
  try {
    const body: Schemas['ChangelogPatch'] = { maxAge: cl.value.maxAge }
    if (cl.value.maxEntries !== null) body.maxEntries = cl.value.maxEntries
    if (cl.value.trimInterval !== null) body.trimInterval = cl.value.trimInterval
    changelog.value = await call(api.PATCH('/api/v1/replication/changelog', { params: { query: q.value }, body }))
    message.success(t('replication.saved'))
  } catch (e) {
    fail(e)
  }
}

// --- RUV --------------------------------------------------------------------------
const ruv = ref<Schemas['RUV']>()
async function loadRUV() {
  try {
    ruv.value = await call(api.GET('/api/v1/replication/ruv', { params: { query: q.value } }))
  } catch (e) {
    fail(e)
  }
}
function cleanRUV(rid: number) {
  dialog.warning({
    title: t('replication.cleanRUV'),
    content: t('replication.cleanRUVConfirm', { rid }),
    positiveText: t('replication.cleanRUV'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        const task = await call(api.POST('/api/v1/replication/cleanallruv', { params: { query: q.value }, body: { replicaId: rid } }))
        tasks.upsert(task)
        message.info(t('replication.cleanRUVStarted', { rid }))
      } catch (e) {
        fail(e)
      }
    },
  })
}

// --- conflicts --------------------------------------------------------------------
const conflicts = ref<Schemas['Conflict'][]>()
const converting = ref<{ dn: string; rdn: string }>()
async function loadConflicts() {
  try {
    conflicts.value = await call(api.GET('/api/v1/replication/conflicts', { params: { query: q.value } }))
  } catch (e) {
    fail(e)
  }
}
function deleteConflict(dn: string) {
  dialog.warning({
    title: t('replication.deleteConflict'),
    content: t('replication.deleteConflictConfirm', { dn }),
    positiveText: t('replication.deleteConflict'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/replication/conflicts', { params: { query: { dn } } }))
        await loadConflicts()
      } catch (e) {
        fail(e)
      }
    },
  })
}
async function convert() {
  const c = converting.value
  if (!c) return
  try {
    await call(api.POST('/api/v1/replication/conflicts/convert', { params: { query: { dn: c.dn } }, body: { rdn: c.rdn.trim() } }))
    converting.value = undefined
    message.success(t('replication.converted'))
    await loadConflicts()
  } catch (e) {
    fail(e)
  }
}

watch(
  () => [props.suffix, props.section],
  () => {
    if (props.section === 'changelog') void loadChangelog()
    if (props.section === 'ruv') void loadRUV()
    if (props.section === 'conflicts') void loadConflicts()
  },
  { immediate: true },
)
</script>

<template>
  <NCard v-if="section === 'changelog'" size="small">
    <NAlert v-if="changelog && !changelog.exists" type="info">{{ t('replication.noChangelog') }}</NAlert>
    <NForm v-else label-placement="top" class="form" @submit.prevent="saveChangelog">
      <p class="hint mono">{{ changelog?.dn }}</p>
      <NFormItem :label="t('replication.maxAge')" :feedback="t('replication.maxAgeHint')">
        <NInput v-model:value="cl.maxAge" :placeholder="t('pages._default')" class="w" />
      </NFormItem>
      <NFormItem :label="t('replication.maxEntries')" :feedback="t('replication.maxEntriesHint')">
        <NInputNumber v-model:value="cl.maxEntries" :min="0" class="w" :placeholder="t('pages._default')" />
      </NFormItem>
      <NFormItem :label="t('replication.trimInterval')">
        <NInputNumber v-model:value="cl.trimInterval" :min="1" class="w" :placeholder="t('pages._default')">
          <template #suffix>{{ t('pages._units.s') }}</template>
        </NInputNumber>
      </NFormItem>
      <NButton type="primary" attr-type="submit">{{ t('pages._save') }}</NButton>
    </NForm>
  </NCard>

  <NCard v-else-if="section === 'ruv'" size="small">
    <p class="hint">{{ t('replication.ruvHint') }}</p>
    <p v-if="ruv?.generation" class="hint">{{ t('replication.generation') }}: <span class="mono">{{ ruv.generation }}</span></p>
    <NEmpty v-if="ruv && !ruv.replicas.length" size="small" :description="t('replication.ruvEmpty')" />
    <NTable v-else-if="ruv" size="small" :single-line="false">
      <thead>
        <tr>
          <th>{{ t('replication.replicaId') }}</th>
          <th>URL</th>
          <th>{{ t('replication.lastChange') }}</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in ruv.replicas" :key="r.replicaId">
          <td>
            {{ r.replicaId }}
            <NTag v-if="r.replicaId === replicaId && role === 'supplier'" size="tiny" :bordered="false" type="info">{{ t('replication.thisServer') }}</NTag>
          </td>
          <td class="mono">{{ r.url || '—' }}</td>
          <td>{{ fmt(r.maxTime) }}</td>
          <td>
            <NButton v-if="!(r.replicaId === replicaId && role === 'supplier')" size="tiny" @click="cleanRUV(r.replicaId)">{{ t('replication.cleanRUV') }}</NButton>
          </td>
        </tr>
      </tbody>
    </NTable>
  </NCard>

  <NCard v-else size="small">
    <p class="hint">{{ t('replication.conflictsHint') }}</p>
    <NEmpty v-if="conflicts && !conflicts.length" size="small" :description="t('replication.noConflicts')" />
    <div v-for="c in conflicts ?? []" :key="c.dn" class="conflict">
      <div class="conflict-text">
        <RouterLink :to="{ path: '/browser', query: { dn: c.dn } }" class="mono">{{ c.dn }}</RouterLink>
        <NTag v-if="c.glue" size="tiny" :bordered="false" type="warning">glue</NTag>
        <div class="hint">{{ c.reason }}</div>
      </div>
      <NSpace :size="6">
        <NButton size="tiny" @click="converting = { dn: c.dn, rdn: '' }">{{ t('replication.convert') }}</NButton>
        <NButton size="tiny" type="error" ghost @click="deleteConflict(c.dn)">{{ t('replication.deleteConflict') }}</NButton>
      </NSpace>
    </div>
    <form v-if="converting" class="convert" @submit.prevent="convert">
      <span class="hint">{{ t('replication.convertHint') }}</span>
      <NInput v-model:value="converting.rdn" placeholder="uid=jdoe-2" :input-props="{ class: 'mono', spellcheck: false }" />
      <NButton attr-type="submit" type="primary" :disabled="!converting.rdn.trim()">{{ t('replication.convert') }}</NButton>
      <NButton @click="converting = undefined">{{ t('edit.cancel') }}</NButton>
    </form>
  </NCard>
</template>

<style scoped>
.form {
  max-width: 480px;
}

.w {
  width: 220px;
}

.hint {
  margin: 0 0 10px;
  font-size: 13px;
  color: v-bind('vars.textColor3');
  overflow-wrap: anywhere;
}

.conflict {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
  border-top: 1px solid v-bind('vars.dividerColor');
}

.conflict-text {
  min-width: 0;
  overflow-wrap: anywhere;
}

.convert {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}

.convert .n-input {
  flex: 1;
  min-width: 200px;
}
</style>
