<script setup lang="ts">
/** Roles: list, create, edit, members. */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NForm,
  NFormItem,
  NGrid,
  NGridItem,
  NIcon,
  NInput,
  NModal,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSkeleton,
  NSpace,
  NTag,
  useDialog,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { Plus, Trash2, UserMinus } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { useLatest } from '@/composables/latest'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'

type Role = Schemas['Role']

const props = defineProps<{ selected?: string }>()
const emit = defineEmits<{ select: [dn: string | undefined] }>()

const { t } = useI18n()
const vars = useThemeVars()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()

const roles = ref<Role[]>()
const error = ref<LocalizedError>()

async function load() {
  try {
    roles.value = await call(api.GET('/api/v1/roles'))
  } catch (e) {
    error.value = localizeError(e)
  }
}
onMounted(load)

const role = computed(() => roles.value?.find((r) => r.dn === props.selected))
const parentOf = (dn: string) => dn.slice(dn.indexOf(',') + 1)
const roleOptions = computed(() => (roles.value ?? []).filter((r) => r.dn !== role.value?.dn).map((r) => ({ label: r.name, value: r.dn })))
const fail = (e: unknown) => {
  const err = localizeError(e)
  message.error(`${t('roles.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
}

// --- details: edit -------------------------------------------------------------

const form = ref<{ description: string; filter: string; roles: string[] }>({ description: '', filter: '', roles: [] })
watch(role, (r) => (form.value = { description: r?.description ?? '', filter: r?.filter ?? '', roles: [...(r?.roles ?? [])] }), { immediate: true })
const dirty = computed(() => {
  const r = role.value
  if (!r) return false
  return (
    form.value.description !== (r.description ?? '') ||
    (r.kind === 'filtered' && form.value.filter !== (r.filter ?? '')) ||
    (r.kind === 'nested' && form.value.roles.join('\n') !== r.roles.join('\n'))
  )
})
const saving = ref(false)
async function save() {
  const r = role.value
  if (!r) return
  saving.value = true
  try {
    const body: Schemas['RolePatch'] = { description: form.value.description }
    if (r.kind === 'filtered') body.filter = form.value.filter
    if (r.kind === 'nested') body.roles = form.value.roles
    await call(api.PATCH('/api/v1/roles', { params: { query: { dn: r.dn } }, body }))
    message.success(t('roles.saved'))
    await load()
  } catch (e) {
    fail(e)
  } finally {
    saving.value = false
  }
}

function remove() {
  const r = role.value
  if (!r) return
  dialog.warning({
    title: t('roles.delete'),
    content: t('roles.deleteConfirm', { name: r.name }),
    positiveText: t('roles.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/roles', { params: { query: { dn: r.dn } } }))
        message.success(t('roles.deleted', { name: r.name }))
        emit('select', undefined)
        await load()
      } catch (e) {
        fail(e)
      }
    },
  })
}

// --- members -------------------------------------------------------------------

const members = ref<Schemas['RoleMembers']>()
const latestMembers = useLatest()
async function loadMembers() {
  members.value = undefined
  if (!role.value) return
  const dn = role.value.dn
  try {
    const r = await latestMembers.run((signal) => call(api.GET('/api/v1/roles/members', { params: { query: { dn } }, signal })))
    if (!r.stale) members.value = r.value
  } catch {
    members.value = { members: [], truncated: false }
  }
}
watch(() => role.value?.dn, loadMembers, { immediate: true })

const newMember = ref('')
async function setMember(member: string, add: boolean) {
  if (!role.value) return
  try {
    await call(api.POST('/api/v1/roles/members', { params: { query: { dn: role.value.dn } }, body: { member, add } }))
    newMember.value = ''
    // nsRole is computed from a cache the server refreshes in the background.
    setTimeout(loadMembers, 600)
    message.success(t(add ? 'roles.memberAdded' : 'roles.memberRemoved'))
  } catch (e) {
    fail(e)
  }
}

// --- create ----------------------------------------------------------------------

const creating = ref(false)
const draft = ref<Schemas['NewRole']>({ parent: '', name: '', kind: 'managed', roles: [] })
const createError = ref<LocalizedError>()
const busy = ref(false)
function openCreate() {
  draft.value = { parent: server.info?.server.namingContexts[0] ?? '', name: '', kind: 'managed', description: '', filter: '', roles: [] }
  createError.value = undefined
  creating.value = true
}
async function create() {
  busy.value = true
  createError.value = undefined
  try {
    const r = await call(api.POST('/api/v1/roles', { body: draft.value }))
    creating.value = false
    message.success(t('roles.created', { name: r.name }))
    await load()
    emit('select', r.dn)
  } catch (e) {
    createError.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
const allRoleOptions = computed(() => (roles.value ?? []).map((r) => ({ label: `${r.name} — ${parentOf(r.dn)}`, value: r.dn })))
</script>

<template>
  <NAlert v-if="error" type="error">{{ error.summary }}</NAlert>
  <NGrid v-else cols="1 l:5" responsive="screen" :x-gap="16" :y-gap="16">
    <NGridItem span="1 l:2">
      <NCard size="small" :title="t('roles.list')">
        <template #header-extra>
          <NButton size="small" type="primary" @click="openCreate">
            <template #icon><NIcon><Plus /></NIcon></template>
            {{ t('roles.new') }}
          </NButton>
        </template>
        <NSkeleton v-if="!roles" text :repeat="4" />
        <NEmpty v-else-if="!roles.length" size="small" :description="t('roles.none')" />
        <div v-else class="list">
          <button v-for="r in roles" :key="r.dn" type="button" class="item" :class="{ active: r.dn === selected }" @click="emit('select', r.dn)">
            <span class="item-text">
              <span>{{ r.name }}</span>
              <span class="muted mono">{{ parentOf(r.dn) }}</span>
            </span>
            <NTag size="tiny" :bordered="false">{{ t(`roles.kinds.${r.kind}`) }}</NTag>
          </button>
        </div>
      </NCard>
    </NGridItem>
    <NGridItem span="1 l:3">
      <NCard v-if="role" size="small" :title="role.name">
        <template #header-extra>
          <NSpace :size="6" align="center">
            <NTag size="small" :bordered="false" type="info">{{ t(`roles.kinds.${role.kind}`) }}</NTag>
            <NButton size="small" type="error" quaternary @click="remove">
              <template #icon><NIcon><Trash2 /></NIcon></template>
              {{ t('roles.delete') }}
            </NButton>
          </NSpace>
        </template>
        <p class="muted mono dn">{{ role.dn }}</p>
        <p class="hint">{{ t(`roles.kindHints.${role.kind}`) }}</p>
        <NForm label-placement="top" @submit.prevent="save">
          <NFormItem :label="t('roles.description')">
            <NInput v-model:value="form.description" />
          </NFormItem>
          <NFormItem v-if="role.kind === 'filtered'" :label="t('roles.filter')">
            <NInput v-model:value="form.filter" :input-props="{ class: 'mono', spellcheck: false }" />
          </NFormItem>
          <NFormItem v-if="role.kind === 'nested'" :label="t('roles.contains')">
            <NSelect v-model:value="form.roles" multiple filterable :options="roleOptions" />
          </NFormItem>
          <NButton type="primary" attr-type="submit" :disabled="!dirty" :loading="saving">{{ t('pages._save') }}</NButton>
        </NForm>

        <h3 class="sub">{{ t('roles.members') }}</h3>
        <form v-if="role.kind === 'managed'" class="add" @submit.prevent="newMember.trim() && setMember(newMember.trim(), true)">
          <NInput v-model:value="newMember" :placeholder="t('roles.memberPlaceholder')" :input-props="{ class: 'mono', spellcheck: false }" />
          <NButton attr-type="submit" :disabled="!newMember.trim()">{{ t('roles.addMember') }}</NButton>
        </form>
        <NSkeleton v-if="!members" text :repeat="3" />
        <p v-else-if="!members.members.length" class="muted">{{ t('roles.noMembers') }}</p>
        <div v-for="m in members?.members ?? []" :key="m" class="member">
          <RouterLink :to="{ path: '/browser', query: { dn: m } }" class="mono">{{ m }}</RouterLink>
          <NButton v-if="role.kind === 'managed'" size="tiny" quaternary :title="t('roles.removeMember')" :aria-label="t('roles.removeMember')" @click="setMember(m, false)">
            <template #icon><NIcon><UserMinus /></NIcon></template>
          </NButton>
        </div>
        <p v-if="members?.truncated" class="muted">{{ t('roles.truncated') }}</p>
      </NCard>
      <NCard v-else size="small"><NEmpty :description="t('roles.pick')" /></NCard>
    </NGridItem>
  </NGrid>

  <NModal v-model:show="creating" preset="card" :title="t('roles.new')" :style="{ width: 'min(600px, 94vw)' }" :mask-closable="!busy">
    <NForm label-placement="top" @submit.prevent="create">
      <NFormItem :label="t('roles.kind')" :feedback="t(`roles.kindHints.${draft.kind}`)">
        <NRadioGroup v-model:value="draft.kind">
          <NRadioButton v-for="k in ['managed', 'filtered', 'nested']" :key="k" :value="k">{{ t(`roles.kinds.${k}`) }}</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem :label="t('roles.name')">
        <NInput v-model:value="draft.name" />
      </NFormItem>
      <NFormItem :label="t('roles.parent')" :feedback="t('roles.parentHint')">
        <NInput v-model:value="draft.parent" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('roles.description')">
        <NInput v-model:value="draft.description" />
      </NFormItem>
      <NFormItem v-if="draft.kind === 'filtered'" :label="t('roles.filter')">
        <NInput v-model:value="draft.filter" :input-props="{ class: 'mono', spellcheck: false }" placeholder="(employeeType=contractor)" />
      </NFormItem>
      <NFormItem v-if="draft.kind === 'nested'" :label="t('roles.contains')">
        <NSelect v-model:value="draft.roles" multiple filterable :options="allRoleOptions" />
      </NFormItem>
    </NForm>
    <NAlert v-if="createError" type="error">
      {{ createError.summary }}
      <div v-if="createError.detail" class="mono">{{ createError.detail }}</div>
    </NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="creating = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="busy" :disabled="!draft.name.trim() || !draft.parent.trim()" @click="create">{{ t('create.create') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
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
  flex: 1;
  min-width: 0;
}

.muted {
  font-size: 12px;
  color: v-bind('vars.textColor3');
  overflow-wrap: anywhere;
}

.dn {
  margin: 0 0 6px;
}

.hint {
  margin: 0 0 14px;
  font-size: 13px;
  color: v-bind('vars.textColor2');
}

.sub {
  margin: 20px 0 8px;
  font-size: 13px;
  font-weight: 600;
}

.add {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}

.add .n-input {
  flex: 1;
}

.member {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
  overflow-wrap: anywhere;
}
</style>
