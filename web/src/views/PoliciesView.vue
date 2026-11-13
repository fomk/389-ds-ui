<script setup lang="ts">
/**
 * Password policies: the global one and the subtree/user (local) ones. Each
 * is edited with the declarative page; local policies are created and
 * deleted here.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NGrid,
  NGridItem,
  NH1,
  NIcon,
  NInput,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSkeleton,
  NSpace,
  NTag,
  useDialog,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { FolderTree, Globe, Plus, Trash2, User, UsersRound } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'
import SettingsPage from '@/components/settings/SettingsPage.vue'
import AccountReport from '@/components/AccountReport.vue'

type Policy = Schemas['LocalPolicy']

const GLOBAL = 'global'
const REPORT = 'report'

const { t } = useI18n()
const vars = useThemeVars()
const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()

const policies = ref<Policy[]>()
const error = ref<LocalizedError>()
const localEnabled = ref<boolean>()
const dirty = ref(false)

async function load() {
  try {
    const [list, global] = await Promise.all([
      call(api.GET('/api/v1/pwpolicies')),
      call(api.GET('/api/v1/pages/{id}', { params: { path: { id: 'password-policy' } } })),
    ])
    policies.value = list
    localEnabled.value = global.values['nsslapd-pwpolicy-local']?.[0]?.toLowerCase() === 'on'
  } catch (e) {
    error.value = localizeError(e)
  }
}
onMounted(load)

/** ?policy=<dn>, or "global". */
const selected = computed(() => (typeof route.query.policy === 'string' ? route.query.policy : GLOBAL))
const selectedPolicy = computed(() => policies.value?.find((p) => p.dn === selected.value))
// A policy deleted elsewhere: back to the global one.
watch([policies, selected], () => {
  if (policies.value && selected.value !== GLOBAL && selected.value !== REPORT && !selectedPolicy.value) select(GLOBAL)
})

function select(id: string) {
  router.replace({ path: '/policies', query: id === GLOBAL ? {} : { policy: id } })
}

async function enableLocal() {
  try {
    await call(api.PATCH('/api/v1/pages/{id}', { params: { path: { id: 'password-policy' } }, body: { values: { 'nsslapd-pwpolicy-local': ['on'] } } }))
    localEnabled.value = true
    message.success(t('policies.localEnabled'))
  } catch (e) {
    message.error(localizeError(e).summary)
  }
}

// --- create -----------------------------------------------------------------

const creating = ref(false)
const draft = ref<{ kind: 'subtree' | 'user'; target: string }>({ kind: 'subtree', target: '' })
const busy = ref(false)
const createError = ref<LocalizedError>()

function openCreate() {
  draft.value = { kind: 'subtree', target: server.info?.server.namingContexts[0] ?? '' }
  createError.value = undefined
  creating.value = true
}

async function create() {
  busy.value = true
  createError.value = undefined
  try {
    const res = await call(api.POST('/api/v1/pwpolicies', { body: { kind: draft.value.kind, target: draft.value.target.trim() } }))
    creating.value = false
    message.success(t('policies.created', { target: res.policy.target }))
    if (res.enabledLocalPolicies) {
      message.info(t('policies.localTurnedOn'), { duration: 8000 })
      localEnabled.value = true
    }
    await load()
    select(res.policy.dn)
  } catch (e) {
    createError.value = localizeError(e)
  } finally {
    busy.value = false
  }
}

function remove(p: Policy) {
  dialog.warning({
    title: t('policies.delete'),
    content: t(p.kind === 'subtree' ? 'policies.deleteSubtree' : 'policies.deleteUser', { target: p.target }),
    positiveText: t('policies.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/pwpolicies', { params: { query: { dn: p.dn } } }))
        message.success(t('policies.deleted', { target: p.target }))
        dirty.value = false
        select(GLOBAL)
        await load()
      } catch (e) {
        message.error(`${t('policies.deleteError')}: ${localizeError(e).summary}`)
      }
    },
  })
}

function confirmLeave(): Promise<boolean> {
  if (!dirty.value) return Promise.resolve(true)
  return new Promise((resolve) => {
    dialog.warning({
      title: t('pages._leaveTitle'),
      content: t('pages._leaveText'),
      positiveText: t('pages._leave'),
      negativeText: t('pages._stay'),
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false),
    })
  })
}
onBeforeRouteUpdate(confirmLeave)
onBeforeRouteLeave(confirmLeave)
</script>

<template>
  <NH1 class="title">{{ t('nav.policies') }}</NH1>
  <NAlert v-if="error" type="error" :title="t('policies.loadError')">{{ error.summary }}</NAlert>

  <template v-else>
    <NAlert v-if="localEnabled === false && policies?.length" type="warning" class="block">
      {{ t('policies.localOff') }}
      <NButton size="small" class="inline-btn" @click="enableLocal">{{ t('policies.enableLocal') }}</NButton>
    </NAlert>

    <NGrid cols="1 l:3" responsive="screen" :x-gap="16" :y-gap="16">
      <NGridItem>
        <NCard size="small">
          <NSkeleton v-if="!policies" text :repeat="4" />
          <template v-else>
          <button type="button" class="item" :class="{ active: selected === GLOBAL }" @click="select(GLOBAL)">
            <NIcon :size="17"><Globe /></NIcon>
            <span class="item-text">
              <span>{{ t('policies.global') }}</span>
              <span class="muted">{{ t('policies.globalHint') }}</span>
            </span>
          </button>
          <button type="button" class="item" :class="{ active: selected === REPORT }" @click="select(REPORT)">
            <NIcon :size="17"><UsersRound /></NIcon>
            <span class="item-text">
              <span>{{ t('accountReport.title') }}</span>
              <span class="muted">{{ t('accountReport.hint') }}</span>
            </span>
          </button>
          <div class="local-head">
            <span class="local-title">{{ t('policies.local') }}</span>
            <NButton size="small" type="primary" :title="t('policies.new')" @click="openCreate">
              <template #icon><NIcon><Plus /></NIcon></template>
              {{ t('policies.add') }}
            </NButton>
          </div>
          <div class="list">
            <button
              v-for="p in policies"
              :key="p.dn"
              type="button"
              class="item"
              :class="{ active: selected === p.dn }"
              @click="select(p.dn)"
            >
              <NIcon :size="17"><FolderTree v-if="p.kind === 'subtree'" /><User v-else /></NIcon>
              <span class="item-text">
                <span class="mono target">{{ p.target }}</span>
                <span class="muted">{{ t(`policies.kinds.${p.kind}`) }}</span>
              </span>
            </button>
            <p v-if="!policies.length" class="muted empty">{{ t('policies.none') }}</p>
          </div>
          </template>
        </NCard>
      </NGridItem>

      <NGridItem span="1 l:2">
        <template v-if="selectedPolicy">
          <NCard size="small" class="head">
            <div class="head-row">
              <div>
                <NTag size="small" :bordered="false" type="info">{{ t(`policies.kinds.${selectedPolicy.kind}`) }}</NTag>
                <span class="mono target-head">{{ selectedPolicy.target }}</span>
              </div>
              <NButton size="small" type="error" ghost @click="remove(selectedPolicy)">
                <template #icon><NIcon><Trash2 /></NIcon></template>
                {{ t('policies.delete') }}
              </NButton>
            </div>
          </NCard>
          <SettingsPage id="password-policy-local" :key="selectedPolicy.dn" v-model:dirty="dirty" :dn="selectedPolicy.dn" />
        </template>
        <AccountReport v-else-if="selected === REPORT" />
        <SettingsPage v-else-if="policies" id="password-policy" key="global" v-model:dirty="dirty" />
      </NGridItem>
    </NGrid>
  </template>

  <NModal v-model:show="creating" preset="card" :title="t('policies.new')" :style="{ width: 'min(560px, 92vw)' }" :mask-closable="!busy">
    <NForm label-placement="top" @submit.prevent="create">
      <NFormItem :label="t('policies.kind')">
        <NRadioGroup v-model:value="draft.kind">
          <NRadioButton value="subtree">{{ t('policies.kinds.subtree') }}</NRadioButton>
          <NRadioButton value="user">{{ t('policies.kinds.user') }}</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem :label="t(draft.kind === 'subtree' ? 'policies.targetSubtree' : 'policies.targetUser')" :feedback="t(draft.kind === 'subtree' ? 'policies.subtreeHint' : 'policies.userHint')">
        <NInput v-model:value="draft.target" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
    </NForm>
    <NAlert v-if="createError" type="error" class="top">
      {{ createError.summary }}
      <div v-if="createError.detail" class="mono">{{ createError.detail }}</div>
    </NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="creating = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :disabled="!draft.target.trim()" :loading="busy" @click="create">{{ t('create.create') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.block {
  margin-bottom: 16px;
}

.inline-btn {
  margin-left: 12px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.item:hover {
  background: v-bind('vars.hoverColor');
}

.item.active {
  background: color-mix(in srgb, v-bind('vars.primaryColor') 12%, transparent);
}

.item-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.target {
  overflow-wrap: anywhere;
}

.muted {
  font-size: 12px;
  color: v-bind('vars.textColor3');
}

.empty {
  margin: 4px 10px;
}

.local-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin: 12px 0 6px;
  padding: 12px 0 0 10px;
  border-top: 1px solid v-bind('vars.dividerColor');
}

.local-title {
  font-size: 13px;
  font-weight: 600;
}

.head {
  margin-bottom: 16px;
}

.head-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.target-head {
  margin-left: 10px;
  overflow-wrap: anywhere;
}

.top {
  margin-top: 12px;
}
</style>
