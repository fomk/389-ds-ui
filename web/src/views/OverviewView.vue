<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NCard,
  NDescriptions,
  NDescriptionsItem,
  NGrid,
  NGridItem,
  NH1,
  NIcon,
  NSkeleton,
  NSpace,
  NTag,
  useThemeVars,
} from 'naive-ui'
import { CircleCheck, CircleMinus, Network } from '@lucide/vue'
import type { Capabilities } from '@/api/client'
import { useServerStore } from '@/stores/server'

const { t } = useI18n()
const server = useServerStore()
const vars = useThemeVars()
const info = computed(() => server.info?.server)
const caps = computed(() => server.capabilities)

// API enum values → message keys (keys must be literal for typed t()).
const authLabel = (a: Capabilities['ldapAuth']) =>
  ({
    'sasl-external': t('overview.auth.saslExternal'),
    simple: t('overview.auth.simple'),
    anonymous: t('overview.auth.anonymous'),
  })[a]
const modeLabel = (m: Capabilities['mode']) =>
  ({
    sidecar: t('overview.deployment.modes.sidecar'),
    remote: t('overview.deployment.modes.remote'),
    aio: t('overview.deployment.modes.aio'),
  })[m]
const dbLabel = computed(() => {
  switch (info.value?.dbImplementation) {
    case 'mdb':
      return t('overview.db.mdb')
    case 'bdb':
      return t('overview.db.bdb')
    default:
      return '—'
  }
})

const features = computed(() => {
  const c = caps.value
  if (!c) return []
  return [
    { label: t('overview.deployment.features.lifecycle'), ok: c.lifecycle !== 'none', note: c.lifecycle !== 'none' ? c.lifecycle : '' },
    { label: t('overview.deployment.features.files'), ok: c.files, note: '' },
    { label: t('overview.deployment.features.certs'), ok: c.certs, note: '' },
    { label: t('overview.deployment.features.offline'), ok: c.offline, note: '' },
  ]
})
</script>

<template>
  <NH1 class="title">{{ t('overview.title') }}</NH1>

  <NAlert v-if="server.infoError && !info" type="error" :title="t('overview.infoError')" class="block">
    {{ server.infoError.summary }}
    <div v-if="server.infoError.detail" class="mono muted">{{ server.infoError.detail }}</div>
  </NAlert>

  <NGrid cols="1 m:2 l:3" responsive="screen" :x-gap="16" :y-gap="16">
    <NGridItem>
      <NCard :title="t('overview.server.title')" class="card">
        <NDescriptions v-if="info" :column="1" label-placement="left" size="small">
          <NDescriptionsItem :label="t('overview.server.version')">{{ info.vendorVersion }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('overview.server.instance')">
            <span class="mono">slapd-{{ server.info?.instance }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem :label="t('overview.server.storage')">{{ dbLabel }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('overview.server.connection')">
            <span class="mono">{{ info.boundAs || '—' }}</span>
            <div v-if="caps" class="muted">{{ authLabel(caps.ldapAuth) }}</div>
          </NDescriptionsItem>
          <NDescriptionsItem :label="t('overview.server.uiVersion')">{{ server.info?.uiVersion }}</NDescriptionsItem>
        </NDescriptions>
        <NSkeleton v-else text :repeat="4" />
      </NCard>
    </NGridItem>

    <NGridItem>
      <NCard :title="t('overview.suffixes.title')" class="card">
        <template v-if="info" #header-extra>
          <span class="muted">{{ t('overview.suffixes.count', { n: info.namingContexts.length }, info.namingContexts.length) }}</span>
        </template>
        <NSpace v-if="info && info.namingContexts.length" vertical>
          <div v-for="nc in info.namingContexts" :key="nc" class="row">
            <NIcon :size="16"><Network /></NIcon>
            <span class="mono">{{ nc }}</span>
            <NTag v-if="nc === info.defaultNamingContext" size="small" :bordered="false">
              {{ t('overview.suffixes.default') }}
            </NTag>
          </div>
        </NSpace>
        <span v-else-if="info" class="muted">{{ t('overview.suffixes.none') }}</span>
        <NSkeleton v-else text :repeat="2" />
      </NCard>
    </NGridItem>

    <NGridItem>
      <NCard :title="t('overview.deployment.title')" class="card">
        <template v-if="caps">
          <p class="mode">
            {{ t('overview.deployment.mode') }}: <strong>{{ modeLabel(caps.mode) }}</strong>
          </p>
          <NSpace vertical :size="8">
            <div v-for="f in features" :key="f.label" class="row">
              <NIcon :size="16" :color="f.ok ? vars.successColor : vars.textColor3">
                <CircleCheck v-if="f.ok" />
                <CircleMinus v-else />
              </NIcon>
              <span>{{ f.label }}</span>
              <span v-if="f.note" class="muted">({{ f.note }})</span>
            </div>
          </NSpace>
        </template>
        <NSkeleton v-else text :repeat="4" />
      </NCard>
    </NGridItem>

    <NGridItem>
      <NCard :title="t('overview.ldap.title')" class="card">
        <NDescriptions v-if="info" :column="1" label-placement="left" size="small">
          <NDescriptionsItem :label="t('overview.ldap.controls')">{{ info.supportedControls.length }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('overview.ldap.extensions')">{{ info.supportedExtensions.length }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('overview.ldap.sasl')">
            {{ info.supportedSaslMechanisms.join(', ') || '—' }}
          </NDescriptionsItem>
        </NDescriptions>
        <NSkeleton v-else text :repeat="3" />
      </NCard>
    </NGridItem>
  </NGrid>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.block {
  margin-bottom: 16px;
}

.card {
  height: 100%;
}

.row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mode {
  margin: 0 0 12px;
}

.muted {
  color: v-bind('vars.textColor3');
}
</style>
