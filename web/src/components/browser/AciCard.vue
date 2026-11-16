<script setup lang="ts">
/**
 * Access control of an entry: its own ACIs (edit with syntax check), the ACIs
 * inherited from ancestors, and an effective-rights check for a bind DN.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NCollapse,
  NCollapseItem,
  NEmpty,
  NIcon,
  NInput,
  NModal,
  NSpace,
  NTable,
  NTag,
  useDialog,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { Pencil, Plus, ShieldCheck, Trash2 } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { useLatest } from '@/composables/latest'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { ACI_TEMPLATE, check } from '@/ldap/aci'
import AciText from './AciText.vue'

type EntryACIs = Schemas['EntryACIs']
type Rights = Schemas['EffectiveRights']

const props = defineProps<{ dn: string; version?: string }>()
const { t } = useI18n()
const vars = useThemeVars()
const message = useMessage()
const dialog = useDialog()

const own = ref<string[]>([])
const inherited = ref<EntryACIs[]>([])

const latest = useLatest()
async function load() {
  try {
    const r = await latest.run((signal) =>
      Promise.all([
        call(api.GET('/api/v1/aci', { params: { query: { dn: props.dn } }, signal })),
        call(api.GET('/api/v1/aci/inherited', { params: { query: { dn: props.dn } }, signal })),
      ]),
    )
    if (r.stale) return
    const [mine, up] = r.value
    own.value = mine[0]?.acis ?? []
    inherited.value = up
  } catch {
    own.value = []
    inherited.value = []
  }
}
watch(() => [props.dn, props.version], load, { immediate: true })

const inheritedCount = computed(() => inherited.value.reduce((n, e) => n + e.acis.length, 0))

async function save(list: string[]) {
  await call(api.PUT('/api/v1/aci', { params: { query: { dn: props.dn } }, body: { acis: list } }))
  await load()
}

// --- edit one ACI -------------------------------------------------------------

const editing = ref<{ index: number; text: string }>()
const saving = ref(false)
const saveError = ref<LocalizedError>()
const problems = computed(() => (editing.value ? check(editing.value.text) : []))

function add() {
  saveError.value = undefined
  editing.value = { index: -1, text: ACI_TEMPLATE }
}
function edit(i: number) {
  saveError.value = undefined
  editing.value = { index: i, text: own.value[i]! }
}

async function submit() {
  const e = editing.value
  if (!e) return
  const list = [...own.value]
  if (e.index < 0) list.push(e.text.trim())
  else list[e.index] = e.text.trim()
  saving.value = true
  saveError.value = undefined
  try {
    await save(list)
    editing.value = undefined
    message.success(t('aci.saved'))
  } catch (err) {
    saveError.value = localizeError(err)
  } finally {
    saving.value = false
  }
}

function remove(i: number) {
  dialog.warning({
    title: t('aci.delete'),
    content: t('aci.deleteConfirm'),
    positiveText: t('aci.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await save(own.value.filter((_, j) => j !== i))
        message.success(t('aci.deleted'))
      } catch (e) {
        message.error(`${t('aci.error')}: ${localizeError(e).summary}`)
      }
    },
  })
}

// --- effective rights -----------------------------------------------------------

const gerOpen = ref(false)
const subject = ref('')
const rights = ref<Rights>()
const gerBusy = ref(false)
const gerError = ref<LocalizedError>()

async function runGER() {
  gerBusy.value = true
  gerError.value = undefined
  try {
    rights.value = await call(api.POST('/api/v1/aci/effective-rights', { body: { target: props.dn, subject: subject.value.trim() } }))
  } catch (e) {
    rights.value = undefined
    gerError.value = localizeError(e)
  } finally {
    gerBusy.value = false
  }
}
function openGER() {
  rights.value = undefined
  gerError.value = undefined
  gerOpen.value = true
}

const ENTRY_RIGHTS = ['v', 'a', 'd', 'n'] as const
const ATTR_RIGHTS = ['r', 's', 'c', 'w', 'o'] as const
const attrRows = computed(() =>
  Object.entries(rights.value?.attributes ?? {})
    .filter(([name]) => name !== '')
    .sort(([a], [b]) => a.localeCompare(b)),
)
</script>

<template>
  <NCard size="small" :title="t('aci.title')" class="card">
    <template #header-extra>
      <NSpace :size="6">
        <NButton size="small" @click="openGER">
          <template #icon><NIcon><ShieldCheck /></NIcon></template>
          {{ t('aci.effective') }}
        </NButton>
        <NButton size="small" type="primary" @click="add">
          <template #icon><NIcon><Plus /></NIcon></template>
          {{ t('aci.add') }}
        </NButton>
      </NSpace>
    </template>

    <NEmpty v-if="!own.length" size="small" :description="t('aci.none')" />
    <div v-for="(a, i) in own" :key="i" class="aci-row">
      <AciText :aci="a" />
      <div class="row-actions">
        <NButton size="tiny" quaternary :title="t('aci.edit')" :aria-label="t('aci.edit')" @click="edit(i)">
          <template #icon><NIcon><Pencil /></NIcon></template>
        </NButton>
        <NButton size="tiny" quaternary type="error" :title="t('aci.delete')" :aria-label="t('aci.delete')" @click="remove(i)">
          <template #icon><NIcon><Trash2 /></NIcon></template>
        </NButton>
      </div>
    </div>

    <NCollapse v-if="inheritedCount" class="inherited">
      <NCollapseItem :title="t('aci.inherited', { n: inheritedCount }, inheritedCount)" name="inh">
        <div v-for="e in inherited" :key="e.dn" class="inh-entry">
          <div class="inh-dn mono">{{ e.dn }}</div>
          <div v-for="(a, i) in e.acis" :key="i" class="aci-row inh"><AciText :aci="a" /></div>
        </div>
        <p class="muted">{{ t('aci.inheritedNote') }}</p>
      </NCollapseItem>
    </NCollapse>
  </NCard>

  <NModal
    :show="!!editing"
    preset="card"
    :title="editing && editing.index >= 0 ? t('aci.edit') : t('aci.add')"
    :style="{ width: 'min(820px, 94vw)' }"
    :mask-closable="!saving"
    @update:show="(v: boolean) => !v && (editing = undefined)"
  >
    <template v-if="editing">
      <NInput
        v-model:value="editing.text"
        type="textarea"
        :autosize="{ minRows: 4, maxRows: 12 }"
        :input-props="{ class: 'mono', spellcheck: false }"
      />
      <div class="preview"><AciText :aci="editing.text" /></div>
      <ul v-if="problems.length" class="problems">
        <li v-for="(p, i) in problems" :key="i">{{ t(`aci.problems.${p.key}`, p.params ?? {}) }}</li>
      </ul>
      <p v-else class="ok">{{ t('aci.looksOk') }}</p>
      <NAlert v-if="saveError" type="error" :title="t('aci.rejected')">
        {{ saveError.summary }}
        <div v-if="saveError.detail" class="mono">{{ saveError.detail }}</div>
      </NAlert>
    </template>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="saving" @click="editing = undefined">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!editing?.text.trim()" @click="submit">{{ t('pages._save') }}</NButton>
      </NSpace>
    </template>
  </NModal>

  <NModal v-model:show="gerOpen" preset="card" :title="t('aci.effective')" :style="{ width: 'min(720px, 94vw)' }">
    <p class="muted">{{ t('aci.effectiveHint', { dn }) }}</p>
    <form class="ger-form" @submit.prevent="runGER">
      <NInput v-model:value="subject" :placeholder="t('aci.subjectPlaceholder')" :input-props="{ class: 'mono', spellcheck: false }" />
      <NButton type="primary" attr-type="submit" :loading="gerBusy">{{ t('aci.check') }}</NButton>
    </form>
    <NAlert v-if="gerError" type="error" class="top">{{ gerError.summary }}</NAlert>
    <template v-if="rights">
      <div class="entry-rights">
        <span class="muted">{{ t('aci.entryRights') }}:</span>
        <NTag v-for="r in ENTRY_RIGHTS" :key="r" size="small" :bordered="false" :type="rights.entry.includes(r) ? 'success' : 'default'" :class="{ off: !rights.entry.includes(r) }">
          {{ t(`aci.entryRight.${r}`) }}
        </NTag>
      </div>
      <NTable size="small" :single-line="false" class="rights-table">
        <thead>
          <tr>
            <th>{{ t('browser.attribute') }}</th>
            <th v-for="r in ATTR_RIGHTS" :key="r" class="center">{{ t(`aci.attrRight.${r}`) }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="[name, val] in attrRows" :key="name">
            <td class="mono">{{ name }}</td>
            <td v-for="r in ATTR_RIGHTS" :key="r" class="center" :class="val.includes(r) ? 'yes' : 'no'">{{ val.includes(r) ? '✓' : '—' }}</td>
          </tr>
        </tbody>
      </NTable>
    </template>
  </NModal>
</template>

<style scoped>
.card {
  margin-top: 16px;
}

.aci-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  padding: 8px 0;
  border-top: 1px solid v-bind('vars.dividerColor');
}

.aci-row:first-of-type {
  border-top: 0;
}

.aci-row.inh {
  grid-template-columns: 1fr;
}

.row-actions {
  display: grid;
  grid-template-columns: repeat(2, 28px);
  align-content: start;
}

.inherited {
  margin-top: 12px;
}

.inh-dn {
  margin-top: 8px;
  font-size: 12px;
  font-weight: 600;
}

.muted {
  font-size: 12.5px;
  color: v-bind('vars.textColor3');
}

.preview {
  margin-top: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  background: v-bind('vars.actionColor');
}

.problems {
  margin: 10px 0;
  padding-left: 18px;
  color: v-bind('vars.warningColor');
  font-size: 13px;
}

.ok {
  margin: 10px 0;
  color: v-bind('vars.successColor');
  font-size: 13px;
}

.ger-form {
  display: flex;
  gap: 8px;
}

.ger-form .n-input {
  flex: 1;
}

.top {
  margin-top: 12px;
}

.entry-rights {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin: 14px 0 10px;
}

.off {
  opacity: 0.55;
  text-decoration: line-through;
}

.rights-table {
  max-height: 50vh;
}

.center {
  text-align: center;
}

.yes {
  color: v-bind('vars.successColor');
}

.no {
  color: v-bind('vars.textColor3');
}
</style>
