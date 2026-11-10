<script setup lang="ts">
/** TLS: what LDAPS presents, TLS settings, and the NSS certificate database. */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDropdown,
  NEmpty,
  NH1,
  NIcon,
  NInput,
  NModal,
  NSkeleton,
  NSpace,
  NTag,
  useDialog,
  useMessage,
  useThemeVars,
  type DataTableColumns,
} from 'naive-ui'
import { EllipsisVertical, Plus, RefreshCw } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'
import SettingsPage from '@/components/settings/SettingsPage.vue'
import CertImportDialog, { type ImportKind } from '@/components/tls/CertImportDialog.vue'

type Cert = Schemas['Certificate']

const { t } = useI18n()
const vars = useThemeVars()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()

const canManage = computed(() => server.capabilities?.certs ?? false)
const list = ref<Schemas['CertList']>()
const listError = ref<LocalizedError>()
const presented = ref<Schemas['Presented']>()
const presentedError = ref<LocalizedError>()

async function loadCerts() {
  if (!canManage.value) return
  try {
    list.value = await call(api.GET('/api/v1/security/certs'))
    listError.value = undefined
  } catch (e) {
    listError.value = localizeError(e)
  }
}
async function loadPresented() {
  presented.value = undefined
  presentedError.value = undefined
  try {
    presented.value = await call(api.GET('/api/v1/security/presented'))
  } catch (e) {
    presentedError.value = localizeError(e)
  }
}
onMounted(async () => {
  if (!server.capabilities) await server.start()
  await Promise.all([loadCerts(), loadPresented()])
})

const fmtDate = (iso: string) => new Date(iso).toLocaleDateString(currentLocale.value)
const daysLeft = (iso: string) => Math.floor((new Date(iso).getTime() - Date.now()) / 86400000)
const expiryType = (iso: string) => (daysLeft(iso) < 0 ? 'error' : daysLeft(iso) < 30 ? 'warning' : 'default')
/** The database certificate the server presents now, matched by fingerprint. */
const presentedName = computed(() => list.value?.certificates.find((c) => c.sha256 === presented.value?.chain[0]?.sha256)?.nickname)

// --- actions ------------------------------------------------------------------
const fail = (e: unknown) => {
  const err = localizeError(e)
  message.error(`${t('tls.error')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 10000 })
}
function useAsServer(c: Cert) {
  dialog.warning({
    title: t('tls.useAsServer'),
    content: t('tls.useAsServerConfirm', { name: c.nickname }),
    positiveText: t('tls.useAsServer'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.PUT('/api/v1/security/server-cert', { body: { nickname: c.nickname } }))
        message.warning(t('tls.afterRestart'), { duration: 8000 })
        void server.refreshState()
        await loadCerts()
      } catch (e) {
        fail(e)
      }
    },
  })
}
function remove(c: Cert) {
  dialog.warning({
    title: t('tls.delete'),
    content: t(c.hasKey ? 'tls.deleteWithKey' : 'tls.deleteConfirm', { name: c.nickname }),
    positiveText: t('tls.delete'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.DELETE('/api/v1/security/certs/{nickname}', { params: { path: { nickname: c.nickname } } }))
        await loadCerts()
      } catch (e) {
        fail(e)
      }
    },
  })
}
const trustEdit = ref<{ name: string; trust: string }>()
async function saveTrust() {
  const e = trustEdit.value
  if (!e) return
  try {
    await call(api.PATCH('/api/v1/security/certs/{nickname}', { params: { path: { nickname: e.name } }, body: { trust: e.trust.trim() } }))
    trustEdit.value = undefined
    await loadCerts()
  } catch (err) {
    fail(err)
  }
}
const details = ref<Cert>()
async function showDetails(c: Cert) {
  try {
    details.value = await call(api.GET('/api/v1/security/certs/{nickname}', { params: { path: { nickname: c.nickname } } }))
  } catch (e) {
    fail(e)
  }
}

const importKind = ref<ImportKind>('ca')
const importOpen = ref(false)
function openImport(k: string | number) {
  importKind.value = k as ImportKind
  importOpen.value = true
}
const importOptions = computed(() => (['pair', 'ca', 'csr', 'issued'] as const).map((k) => ({ key: k, label: t(`tls.import.${k}`) })))

const rowMenu = (c: Cert) =>
  [
    { key: 'details', label: t('tls.details') },
    ...(c.hasKey && c.nickname !== list.value?.serverCert ? [{ key: 'server', label: t('tls.useAsServer') }] : []),
    { key: 'trust', label: t('tls.trust') },
    ...(c.nickname !== list.value?.serverCert ? [{ key: 'delete', label: t('tls.delete') }] : []),
  ]
function onRow(c: Cert, key: string | number) {
  if (key === 'details') void showDetails(c)
  if (key === 'server') useAsServer(c)
  if (key === 'trust') trustEdit.value = { name: c.nickname, trust: c.trust }
  if (key === 'delete') remove(c)
}

const columns = computed<DataTableColumns<Cert>>(() => [
  {
    key: 'nickname',
    title: t('tls.nickname'),
    render: (c) =>
      h('div', null, [
        h('span', { style: 'font-weight:500' }, c.nickname),
        c.nickname === list.value?.serverCert ? h(NTag, { size: 'tiny', type: 'success', bordered: false, style: 'margin-left:6px' }, () => t('tls.serverTag')) : null,
        c.ca ? h(NTag, { size: 'tiny', bordered: false, style: 'margin-left:6px' }, () => 'CA') : null,
        c.hasKey ? h(NTag, { size: 'tiny', type: 'info', bordered: false, style: 'margin-left:6px' }, () => t('tls.keyTag')) : null,
        h('div', { class: 'mono', style: 'font-size:12px;opacity:.75;overflow-wrap:anywhere' }, c.subject),
      ]),
  },
  {
    key: 'notAfter',
    title: t('tls.expires'),
    width: 150,
    render: (c) => h(NTag, { size: 'small', bordered: false, type: expiryType(c.notAfter) }, () => fmtDate(c.notAfter)),
  },
  { key: 'trust', title: t('tls.trust'), width: 90, className: 'mono' },
  {
    key: 'actions',
    title: '',
    width: 48,
    render: (c) =>
      h(NDropdown, { trigger: 'click', options: rowMenu(c), onSelect: (k: string | number) => onRow(c, k) }, () =>
        h(NButton, { size: 'tiny', quaternary: true, 'aria-label': t('tls.actions') }, { icon: () => h(NIcon, null, () => h(EllipsisVertical)) }),
      ),
  },
])
</script>

<template>
  <NH1 class="title">{{ t('nav.security') }}</NH1>

  <NCard size="small" :title="t('tls.presented')" class="block">
    <template #header-extra>
      <NButton size="small" quaternary :aria-label="t('tls.refresh')" @click="loadPresented">
        <template #icon><NIcon><RefreshCw /></NIcon></template>
      </NButton>
    </template>
    <NAlert v-if="presentedError" type="warning" :show-icon="false">{{ presentedError.detail || presentedError.summary }}</NAlert>
    <NSkeleton v-else-if="!presented" text :repeat="2" />
    <dl v-else class="facts">
      <dt>{{ t('tls.address') }}</dt><dd class="mono">{{ presented.address }} · {{ presented.version }} · {{ presented.cipher }}</dd>
      <template v-if="presented.chain[0]">
        <dt>{{ t('tls.subject') }}</dt><dd class="mono">{{ presented.chain[0].subject }}</dd>
        <dt>{{ t('tls.issuer') }}</dt><dd class="mono">{{ presented.chain[0].issuer }}</dd>
        <dt>{{ t('tls.expires') }}</dt><dd><NTag size="small" :bordered="false" :type="expiryType(presented.chain[0].notAfter)">{{ fmtDate(presented.chain[0].notAfter) }}</NTag></dd>
        <template v-if="canManage">
          <dt>{{ t('tls.inDatabase') }}</dt>
          <dd>
            {{ presentedName ?? t('tls.notInDatabase') }}
            <NTag v-if="presentedName && list && presentedName !== list.serverCert" size="small" type="warning" :bordered="false">{{ t('tls.pendingRestart') }}</NTag>
          </dd>
        </template>
      </template>
    </dl>
  </NCard>

  <div class="settings">
    <SettingsPage id="tls-general" />
    <SettingsPage id="tls-protocol" />
  </div>

  <NCard size="small" :title="t('tls.certificates')" class="block">
    <template v-if="canManage" #header-extra>
      <NDropdown trigger="click" :options="importOptions" @select="openImport">
        <NButton size="small" type="primary">
          <template #icon><NIcon><Plus /></NIcon></template>
          {{ t('tls.add') }}
        </NButton>
      </NDropdown>
    </template>
    <NAlert v-if="!canManage" type="info">{{ t('tls.noFiles') }}</NAlert>
    <NAlert v-else-if="listError" type="error">{{ listError.summary }}<div v-if="listError.detail" class="mono">{{ listError.detail }}</div></NAlert>
    <NDataTable v-else :columns="columns" :data="list?.certificates ?? []" :loading="!list" size="small" :row-key="(c: Cert) => c.nickname">
      <template #empty><NEmpty size="small" :description="t('tls.none')" /></template>
    </NDataTable>
    <p v-if="canManage" class="hint">{{ t('tls.keysWarning') }}</p>
  </NCard>

  <CertImportDialog v-model:show="importOpen" :kind="importKind" @done="loadCerts" />

  <NModal :show="!!trustEdit" preset="card" :title="t('tls.trust')" :style="{ width: 'min(480px, 92vw)' }" @update:show="(v: boolean) => !v && (trustEdit = undefined)">
    <template v-if="trustEdit">
      <p class="hint">{{ t('tls.trustHint') }}</p>
      <NInput v-model:value="trustEdit.trust" :input-props="{ class: 'mono', spellcheck: false }" />
    </template>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="trustEdit = undefined">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" @click="saveTrust">{{ t('pages._save') }}</NButton>
      </NSpace>
    </template>
  </NModal>

  <NModal :show="!!details" preset="card" :title="details?.nickname" :style="{ width: 'min(760px, 94vw)' }" @update:show="(v: boolean) => !v && (details = undefined)">
    <dl v-if="details" class="facts">
      <dt>{{ t('tls.subject') }}</dt><dd class="mono">{{ details.subject }}</dd>
      <dt>{{ t('tls.issuer') }}</dt><dd class="mono">{{ details.issuer }}</dd>
      <dt>{{ t('tls.valid') }}</dt><dd>{{ fmtDate(details.notBefore) }} — {{ fmtDate(details.notAfter) }}</dd>
      <template v-if="details.dnsNames.length || details.ipAddresses.length">
        <dt>{{ t('tls.sans') }}</dt><dd class="mono">{{ [...details.dnsNames, ...details.ipAddresses].join(', ') }}</dd>
      </template>
      <dt>{{ t('tls.keyType') }}</dt><dd>{{ details.keyType }}</dd>
      <dt>{{ t('tls.serial') }}</dt><dd class="mono">{{ details.serial }}</dd>
      <dt>SHA-256</dt><dd class="mono fp">{{ details.sha256 }}</dd>
      <dt>{{ t('tls.trust') }}</dt><dd class="mono">{{ details.trust }}</dd>
    </dl>
    <NInput v-if="details?.pem" :value="details.pem" type="textarea" readonly :autosize="{ minRows: 6, maxRows: 12 }" :input-props="{ class: 'mono' }" class="pem" />
  </NModal>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.block {
  margin-bottom: 16px;
}

.settings {
  display: grid;
  gap: 16px;
  margin-bottom: 16px;
}

.facts {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 6px 16px;
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

.fp {
  font-size: 11.5px;
}

.hint {
  margin: 10px 0 0;
  font-size: 12.5px;
  color: v-bind('vars.textColor3');
}

.pem {
  margin-top: 12px;
}
</style>
