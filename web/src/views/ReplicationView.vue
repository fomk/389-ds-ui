<script setup lang="ts">
/** Replication of one suffix: replica, agreements, changelog, RUV, conflicts. */
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NH1, NRadioButton, NRadioGroup, NSelect, NSkeleton, NTag } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import AgreementsPanel from '@/components/replication/AgreementsPanel.vue'
import EnableReplication from '@/components/replication/EnableReplication.vue'
import ReplicaSettings from '@/components/replication/ReplicaSettings.vue'
import ReplicationExtras from '@/components/replication/ReplicationExtras.vue'
import TopologyPanel from '@/components/replication/TopologyPanel.vue'

type Section = 'replica' | 'agreements' | 'changelog' | 'ruv' | 'conflicts' | 'topology'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const backends = ref<Schemas['Backend'][]>()
const replica = ref<Schemas['Replica']>()
const error = ref<LocalizedError>()

onMounted(async () => {
  try {
    backends.value = await call(api.GET('/api/v1/backends'))
  } catch (e) {
    error.value = localizeError(e)
  }
})

const suffix = computed(() => (typeof route.query.suffix === 'string' ? route.query.suffix : backends.value?.[0]?.suffix))
const section = computed<Section>(() => {
  const s = route.query.section
  return s === 'replica' || s === 'changelog' || s === 'ruv' || s === 'conflicts' || s === 'topology' ? s : 'agreements'
})
function go(query: { suffix?: string; section?: Section }) {
  router.replace({ path: '/replication', query: { suffix: query.suffix ?? suffix.value, section: query.section ?? section.value } })
}
const suffixOptions = computed(() => (backends.value ?? []).map((b) => ({ label: b.suffix, value: b.suffix })))

async function loadReplica() {
  replica.value = undefined
  if (!suffix.value) return
  try {
    replica.value = await call(api.GET('/api/v1/replication', { params: { query: { suffix: suffix.value } } }))
  } catch (e) {
    error.value = localizeError(e)
  }
}
watch(suffix, loadReplica, { immediate: true })
</script>

<template>
  <NH1 class="title">{{ t('nav.replication') }}</NH1>
  <NAlert v-if="error" type="error">{{ error.summary }}</NAlert>
  <template v-else>
    <div class="bar">
      <NSelect :value="suffix" :options="suffixOptions" class="suffix" :loading="!backends" @update:value="(v: string) => go({ suffix: v })" />
      <template v-if="replica?.enabled">
        <NTag :bordered="false" type="info">{{ t(`replication.roles.${replica.role}`) }}</NTag>
        <NTag v-if="replica.role === 'supplier'" :bordered="false">ID {{ replica.replicaId }}</NTag>
      </template>
      <NTag v-else-if="replica" :bordered="false">{{ t('replication.off') }}</NTag>
    </div>

    <NSkeleton v-if="!replica" text :repeat="5" />
    <EnableReplication v-else-if="!replica.enabled" :suffix="replica.suffix" @enabled="loadReplica" />
    <template v-else>
      <NRadioGroup :value="section" class="sections" @update:value="(v: Section) => go({ section: v })">
        <NRadioButton value="agreements">{{ t('replication.tabs.agreements') }}</NRadioButton>
        <NRadioButton value="replica">{{ t('replication.tabs.replica') }}</NRadioButton>
        <NRadioButton value="changelog">{{ t('replication.tabs.changelog') }}</NRadioButton>
        <NRadioButton value="ruv">{{ t('replication.tabs.ruv') }}</NRadioButton>
        <NRadioButton value="conflicts">{{ t('replication.tabs.conflicts') }}</NRadioButton>
        <NRadioButton value="topology">{{ t('replication.tabs.topology') }}</NRadioButton>
      </NRadioGroup>
      <AgreementsPanel v-if="section === 'agreements'" :suffix="replica.suffix" :role="replica.role" />
      <ReplicaSettings v-else-if="section === 'replica'" :replica="replica" @changed="loadReplica" @disabled="loadReplica" />
      <TopologyPanel v-else-if="section === 'topology'" :key="replica.suffix" :suffix="replica.suffix" />
      <ReplicationExtras v-else :suffix="replica.suffix" :section="section" :replica-id="replica.replicaId" :role="replica.role" />
    </template>
  </template>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}

.suffix {
  width: min(420px, 100%);
}

.sections {
  margin-bottom: 16px;
}
</style>
