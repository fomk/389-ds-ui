<script setup lang="ts">
/**
 * Replication topology: follows the agreements to the other servers with
 * credentials entered here (sent for this report only, not stored) and
 * shows how far each server is behind each supplier.
 */
import { computed, h, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NCheckbox,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NSpace,
  NTag,
  useThemeVars,
  type DataTableColumns,
} from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

type TopoNode = Schemas['TopologyNode']

const props = defineProps<{ suffix: string }>()
const { t } = useI18n()
const vars = useThemeVars()

const bindDN = ref('cn=Directory Manager')
const password = ref('')
const insecure = ref(false)
/** Credentials for single servers, added when their bind fails. */
const hostCreds = reactive<Record<string, { bindDN: string; password: string }>>({})
const report = ref<Schemas['TopologyReport']>()
const busy = ref(false)
const error = ref<LocalizedError>()

async function build() {
  busy.value = true
  error.value = undefined
  try {
    report.value = await call(
      api.POST('/api/v1/replication/topology', {
        body: {
          suffix: props.suffix,
          default: { bindDN: bindDN.value.trim(), password: password.value },
          hosts: Object.entries(hostCreds)
            .filter(([, c]) => c.password)
            .map(([address, c]) => ({ address, bindDN: c.bindDN.trim(), password: c.password })),
          insecureTLS: insecure.value,
        },
      }),
    )
    for (const n of report.value.nodes) {
      if (n.errorKind === 'auth' && !hostCreds[n.address]) hostCreds[n.address] = { bindDN: bindDN.value, password: '' }
    }
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}

/** Seconds as "12 s", "3 min", "2 h". */
function ago(s: number) {
  if (s < 60) return t('topology.seconds', { n: s })
  if (s < 3600) return t('topology.minutes', { n: Math.round(s / 60) })
  return t('topology.hours', { n: Math.round(s / 3600) })
}
function lagCell(s: number | undefined) {
  if (s === undefined) return h('span', { style: { color: vars.value.textColor3 } }, '—')
  if (s < 0) return h(NTag, { size: 'small', type: 'warning', bordered: false }, () => t('topology.missing'))
  if (s === 0) return h(NTag, { size: 'small', type: 'success', bordered: false }, () => t('topology.inSync'))
  return h(NTag, { size: 'small', type: s > 300 ? 'error' : 'warning', bordered: false }, () => t('topology.behind', { time: ago(s) }))
}

const columns = computed<DataTableColumns<TopoNode>>(() => [
  {
    key: 'address',
    title: t('topology.server'),
    render: (n) =>
      h('div', null, [
        h('span', { class: 'mono' }, n.address),
        n.self ? h(NTag, { size: 'tiny', type: 'info', bordered: false, style: 'margin-left:6px' }, () => t('topology.self')) : null,
        n.aliases.length ? h('div', { class: 'mono', style: 'font-size:12px;opacity:.7' }, t('topology.alsoAs', { names: n.aliases.join(', ') })) : null,
        n.error ? h('div', { style: { fontSize: '12px', color: vars.value.errorColor } }, `${t(`topology.errors.${n.errorKind ?? 'other'}`)}: ${n.error}`) : null,
      ]),
  },
  {
    key: 'role',
    title: t('replication.role'),
    width: 150,
    render: (n) => (n.role ? `${t(`replication.roles.${n.role}`)} · ${n.replicaId}` : ''),
  },
  ...(report.value?.suppliers ?? []).map((rid) => ({
    key: `rid-${rid}`,
    title: t('topology.fromSupplier', { rid }),
    width: 150,
    render: (n: TopoNode) => (n.reachable ? lagCell(n.behind.find((b) => b.replicaId === rid)?.seconds) : ''),
  })),
])

const links = computed(() =>
  (report.value?.nodes ?? []).flatMap((n) => n.agreements.map((a) => ({ from: n.address, ...a }))),
)
const authFailed = computed(() => (report.value?.nodes ?? []).filter((n) => n.errorKind === 'auth'))
const stateType = (s: string) => (s === 'ok' ? 'success' : s === 'error' ? 'error' : 'default')
</script>

<template>
  <NCard size="small" :title="t('topology.title')">
    <p class="hint">{{ t('topology.description') }}</p>
    <NForm label-placement="top" class="form" @submit.prevent="build">
      <NFormItem :label="t('topology.bindDN')" class="grow">
        <NInput v-model:value="bindDN" :input-props="{ class: 'mono', spellcheck: false, 'aria-label': t('topology.bindDN') }" />
      </NFormItem>
      <NFormItem :label="t('topology.password')" class="grow">
        <NInput
          v-model:value="password"
          type="password"
          show-password-on="click"
          :input-props="{ autocomplete: 'off', 'aria-label': t('topology.password') }"
        />
      </NFormItem>
      <NFormItem label=" ">
        <NButton type="primary" attr-type="submit" :loading="busy" :disabled="!bindDN.trim() || !password">{{ t('topology.build') }}</NButton>
      </NFormItem>
    </NForm>
    <NCheckbox v-model:checked="insecure">{{ t('topology.insecure') }}</NCheckbox>
    <p class="hint small">{{ t('topology.notStored') }}</p>

    <NAlert v-if="error" type="error" class="block">{{ error.summary }}<div v-if="error.detail" class="mono">{{ error.detail }}</div></NAlert>

    <template v-if="report">
      <NAlert v-if="report.plain.length" type="warning" class="block">{{ t('topology.plain', { servers: report.plain.join(', ') }) }}</NAlert>
      <NAlert v-if="report.truncated" type="warning" class="block">{{ t('topology.truncated') }}</NAlert>
      <NDataTable :columns="columns" :data="report.nodes" size="small" :row-key="(n: TopoNode) => n.address" class="block" />

      <div v-if="authFailed.length" class="block">
        <p class="hint">{{ t('topology.ownCredentials') }}</p>
        <div v-for="n in authFailed" :key="n.address" class="host-creds">
          <span class="mono host">{{ n.address }}</span>
          <NInput v-model:value="hostCreds[n.address]!.bindDN" size="small" :input-props="{ class: 'mono', 'aria-label': `${n.address} ${t('topology.bindDN')}` }" />
          <NInput
            v-model:value="hostCreds[n.address]!.password"
            size="small"
            type="password"
            :input-props="{ autocomplete: 'off', 'aria-label': `${n.address} ${t('topology.password')}` }"
          />
        </div>
        <NButton size="small" :loading="busy" @click="build">{{ t('topology.retry') }}</NButton>
      </div>

      <h4 class="subhead">{{ t('topology.links') }}</h4>
      <p v-if="!links.length" class="hint">{{ t('topology.noLinks') }}</p>
      <NSpace v-else vertical :size="4">
        <div v-for="l in links" :key="l.from + l.name" class="link">
          <span class="mono">{{ l.from }}</span> → <span class="mono">{{ l.target }}</span>
          <NTag size="tiny" :bordered="false">{{ l.transport }}</NTag>
          <NTag v-if="!l.enabled" size="tiny" :bordered="false">{{ t('topology.disabled') }}</NTag>
          <NTag v-else size="tiny" :bordered="false" :type="stateType(l.state)">{{ t(`replication.states.${l.state}`) }}</NTag>
          <span v-if="l.lastUpdate" class="muted"> {{ l.lastUpdate }}</span>
        </div>
      </NSpace>
    </template>
  </NCard>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 12.5px;
  opacity: 0.75;
}

.small {
  margin-top: 6px;
}

.form {
  display: flex;
  flex-wrap: wrap;
  gap: 0 12px;
}

.grow {
  flex: 1 1 220px;
}

.block {
  margin-top: 12px;
}

.host-creds {
  display: grid;
  grid-template-columns: minmax(140px, max-content) 1fr 1fr;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}

.subhead {
  margin: 16px 0 8px;
}

.link {
  font-size: 13px;
}

.muted {
  opacity: 0.7;
  font-size: 12px;
}
</style>
