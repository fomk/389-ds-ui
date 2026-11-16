<script setup lang="ts">
/** Account state of an entry (lock, lockout, password expiry) with lock/unlock. */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NCard, NIcon, NSpace, NTag, useMessage, useThemeVars } from 'naive-ui'
import { Lock, LockOpen } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { useLatest } from '@/composables/latest'
import { currentLocale } from '@/i18n'
import { localizeError } from '@/i18n/errors'

type Status = Schemas['AccountStatus']

const props = defineProps<{ dn: string; version?: string }>()
const { t } = useI18n()
const vars = useThemeVars()
const message = useMessage()

const status = ref<Status>()
const busy = ref(false)

const latest = useLatest()
async function load() {
  try {
    const r = await latest.run((signal) => call(api.GET('/api/v1/accounts/status', { params: { query: { dn: props.dn } }, signal })))
    if (!r.stale) status.value = r.value
  } catch {
    status.value = undefined
  }
}
watch(() => [props.dn, props.version], load, { immediate: true })

const tagType: Record<string, 'success' | 'error' | 'warning' | 'info'> = {
  active: 'success',
  locked: 'error',
  'locked-role': 'error',
  'locked-out': 'error',
  expired: 'warning',
  'must-change': 'info',
}

const fmt = (iso?: string) => (iso ? new Date(iso).toLocaleString(currentLocale.value) : '')

const locked = computed(() => status.value?.states.includes('locked') ?? false)
const unlockable = computed(() => locked.value || (status.value?.states.includes('locked-out') ?? false))

async function act(path: '/api/v1/accounts/lock' | '/api/v1/accounts/unlock') {
  busy.value = true
  try {
    status.value = await call(api.POST(path, { params: { query: { dn: props.dn } } }))
    message.success(t(path.endsWith('unlock') ? 'account.unlocked' : 'account.lockedMsg'))
  } catch (e) {
    message.error(`${t('account.error')}: ${localizeError(e).summary}`)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NCard v-if="status" size="small" :title="t('account.title')" class="card">
    <template #header-extra>
      <NSpace :size="6">
        <NButton v-if="unlockable" size="small" :loading="busy" @click="act('/api/v1/accounts/unlock')">
          <template #icon><NIcon><LockOpen /></NIcon></template>
          {{ t('account.unlock') }}
        </NButton>
        <NButton v-if="!locked" size="small" type="error" ghost :loading="busy" @click="act('/api/v1/accounts/lock')">
          <template #icon><NIcon><Lock /></NIcon></template>
          {{ t('account.lock') }}
        </NButton>
      </NSpace>
    </template>
    <NSpace :size="6" class="states">
      <NTag v-for="s in status.states" :key="s" size="small" :bordered="false" :type="tagType[s] ?? 'default'">
        {{ t(`account.states.${s}`) }}
      </NTag>
    </NSpace>
    <p v-if="status.lockedByRoleOrCoS" class="note">{{ t('account.roleNote') }}</p>
    <dl class="facts">
      <dt>{{ t('account.policy') }}</dt>
      <dd>
        <RouterLink v-if="status.policy" :to="{ path: '/policies', query: { policy: status.policy } }" class="mono">{{ status.policy }}</RouterLink>
        <RouterLink v-else :to="{ path: '/policies' }">{{ t('account.globalPolicy') }}</RouterLink>
      </dd>
      <template v-if="!status.hasPassword"><dt>{{ t('account.password') }}</dt><dd>{{ t('account.noPassword') }}</dd></template>
      <template v-if="status.passwordChanged"><dt>{{ t('account.changed') }}</dt><dd>{{ fmt(status.passwordChanged) }}</dd></template>
      <template v-if="status.passwordExpires && !status.states.includes('must-change')">
        <dt>{{ t('account.expires') }}</dt><dd>{{ fmt(status.passwordExpires) }}</dd>
      </template>
      <template v-if="status.graceLoginsUsed"><dt>{{ t('account.grace') }}</dt><dd>{{ status.graceLoginsUsed }}</dd></template>
      <template v-if="status.failedBinds"><dt>{{ t('account.failed') }}</dt><dd>{{ status.failedBinds }}</dd></template>
      <template v-if="status.states.includes('locked-out')">
        <dt>{{ t('account.lockedUntil') }}</dt><dd>{{ status.lockedOutUntil ? fmt(status.lockedOutUntil) : t('account.untilAdmin') }}</dd>
      </template>
      <template v-if="status.lastLogin"><dt>{{ t('account.lastLogin') }}</dt><dd>{{ fmt(status.lastLogin) }}</dd></template>
    </dl>
  </NCard>
</template>

<style scoped>
.card {
  margin-top: 16px;
}

.states {
  margin-bottom: 10px;
}

.note {
  margin: 0 0 10px;
  font-size: 13px;
  color: v-bind('vars.warningColor');
}

.facts {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 4px 16px;
  margin: 0;
  font-size: 13px;
}

.facts dt {
  color: v-bind('vars.textColor3');
}

.facts dd {
  margin: 0;
  overflow-wrap: anywhere;
}
</style>
