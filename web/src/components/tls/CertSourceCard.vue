<script setup lang="ts">
/** Server certificate kept in step with PEM files of an external tool (DSUI_CERT_FILE). */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NIcon, NTag, useThemeVars } from 'naive-ui'
import { RefreshCw } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { useAuthStore } from '@/stores/auth'

const emit = defineEmits<{ imported: []; loaded: [enabled: boolean] }>()
const { t } = useI18n()
const vars = useThemeVars()
const auth = useAuthStore()

const status = ref<Schemas['CertSource']>()
const checking = ref(false)
const failure = ref<string>()

async function load() {
  try {
    status.value = await call(api.GET('/api/v1/security/cert-source'))
    emit('loaded', status.value.enabled)
  } catch (e) {
    failure.value = localizeError(e).summary
  }
}
onMounted(load)

async function checkNow() {
  checking.value = true
  failure.value = undefined
  const before = status.value?.lastImport
  try {
    status.value = await call(api.POST('/api/v1/security/cert-source/check'))
    if (status.value.lastImport && status.value.lastImport !== before) emit('imported')
  } catch (e) {
    failure.value = localizeError(e).summary
  } finally {
    checking.value = false
  }
}

const fmtTime = (iso?: string) => (iso ? new Date(iso).toLocaleString(currentLocale.value) : '—')
const fmtDate = (iso: string) => new Date(iso).toLocaleDateString(currentLocale.value)
/** A Go duration ("1h30m0s", "20s") in words of the current language. */
const interval = computed(() =>
  [...(status.value?.interval ?? '').matchAll(/(\d+(?:\.\d+)?)(h|m|s)/g)]
    .filter(([, n]) => Number(n) > 0)
    .map(([, n, u]) => t(`tls.source.units.${u}`, { n }))
    .join(' '),
)
const daysLeft = (iso: string) => Math.floor((new Date(iso).getTime() - Date.now()) / 86400000)
const expiryType = (iso: string) => (daysLeft(iso) < 0 ? 'error' : daysLeft(iso) < 30 ? 'warning' : 'default')
</script>

<template>
  <NCard v-if="status?.enabled" size="small" :title="t('tls.source.title')">
    <template v-if="!auth.readOnly" #header-extra>
      <NButton size="small" :loading="checking" @click="checkNow">
        <template #icon><NIcon><RefreshCw /></NIcon></template>
        {{ t('tls.source.checkNow') }}
      </NButton>
    </template>
    <p class="hint">{{ t('tls.source.description') }}</p>
    <NAlert v-if="status.error || failure" type="error" :show-icon="false" class="alert">
      {{ t('tls.source.failed') }}: <span class="mono">{{ status.error || failure }}</span>
    </NAlert>
    <dl class="facts">
      <dt>{{ t('tls.source.files') }}</dt>
      <dd class="mono">{{ status.certFile }}<template v-if="status.keyFile && status.keyFile !== status.certFile"><br />{{ status.keyFile }}</template></dd>
      <dt>{{ t('tls.source.schedule') }}</dt>
      <dd>
        {{ t('tls.source.every', { interval }) }} ·
        {{ status.autoRestart ? t('tls.source.autoRestart') : t('tls.source.manualRestart') }}
      </dd>
      <template v-if="status.source">
        <dt>{{ t('tls.subject') }}</dt><dd class="mono">{{ status.source.subject }}</dd>
        <dt>{{ t('tls.expires') }}</dt>
        <dd><NTag size="small" :bordered="false" :type="expiryType(status.source.notAfter)">{{ fmtDate(status.source.notAfter) }}</NTag></dd>
        <dt>{{ t('tls.inDatabase') }}</dt>
        <dd>
          {{ status.nickname || t('tls.notInDatabase') }}
          <NTag v-if="status.inUse" size="small" type="success" :bordered="false">{{ t('tls.serverTag') }}</NTag>
        </dd>
      </template>
      <dt>{{ t('tls.source.lastCheck') }}</dt><dd>{{ fmtTime(status.lastCheck) }}</dd>
      <dt>{{ t('tls.source.lastImport') }}</dt><dd>{{ status.lastImport ? fmtTime(status.lastImport) : t('tls.source.noImport') }}</dd>
    </dl>
  </NCard>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 12.5px;
  color: v-bind('vars.textColor3');
}

.alert {
  margin-bottom: 12px;
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
</style>
