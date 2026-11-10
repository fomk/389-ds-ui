<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NButton,
  NCard,
  NEmpty,
  NGrid,
  NGridItem,
  NH1,
  NIcon,
  NRadioButton,
  NRadioGroup,
  NSkeleton,
  NTabPane,
  NTabs,
  NTag,
  useThemeVars,
} from 'naive-ui'
import { Database, Plus } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import BackendIndexes from '@/components/db/BackendIndexes.vue'
import BackendSettings from '@/components/db/BackendSettings.vue'
import BackendTransfer from '@/components/db/BackendTransfer.vue'
import BackupsPanel from '@/components/db/BackupsPanel.vue'
import LdifFilesPanel from '@/components/db/LdifFilesPanel.vue'
import CreateBackendDialog from '@/components/db/CreateBackendDialog.vue'
import VlvPanel from '@/components/db/VlvPanel.vue'
import TaskList from '@/components/TaskList.vue'
import { useServerStore } from '@/stores/server'

type Backend = Schemas['Backend']

const { t } = useI18n()
const vars = useThemeVars()
const route = useRoute()
const router = useRouter()
const server = useServerStore()

const backends = ref<Backend[]>()
/** Top-level section, kept in the URL (?section=…). */
const section = computed({
  get: () => (typeof route.query.section === 'string' ? route.query.section : 'suffixes'),
  set: (v: string) => router.replace({ path: '/databases', query: v === 'suffixes' ? {} : { section: v } }),
})
const creating = ref(false)

const selectedName = computed(() => (typeof route.query.be === 'string' ? route.query.be : backends.value?.[0]?.name))
const selected = computed(() => backends.value?.find((b) => b.name === selectedName.value))

async function load() {
  backends.value = await call(api.GET('/api/v1/backends'))
}
onMounted(load)

function select(name: string) {
  router.replace({ path: '/databases', query: { be: name } })
}

async function onCreated(b: Backend) {
  await load()
  await server.loadInfo() // naming contexts changed
  select(b.name)
}

function onChanged(b: Backend) {
  const i = backends.value?.findIndex((x) => x.name === b.name) ?? -1
  if (i >= 0) backends.value!.splice(i, 1, b)
}

async function onDeleted() {
  await load()
  await server.loadInfo()
  router.replace({ path: '/databases' })
}
</script>

<template>
  <NH1 class="title">{{ t('db.title') }}</NH1>

  <NRadioGroup v-model:value="section" class="sections">
    <NRadioButton value="suffixes">{{ t('db.suffixesTab') }}</NRadioButton>
    <NRadioButton value="backups">{{ t('backups.tab') }}</NRadioButton>
    <NRadioButton value="ldif">{{ t('ldifFiles.tab') }}</NRadioButton>
    <NRadioButton value="default-indexes">{{ t('indexes.defaultTab') }}</NRadioButton>
  </NRadioGroup>

  <NCard v-if="section === 'backups'" size="small"><BackupsPanel /></NCard>
  <NCard v-else-if="section === 'ldif'" size="small"><LdifFilesPanel /></NCard>
  <NCard v-else-if="section === 'default-indexes'" size="small" :title="t('indexes.defaultTitle')">
    <p class="section-hint">{{ t('indexes.defaultHint') }}</p>
    <BackendIndexes />
  </NCard>

  <NGrid v-else cols="1 l:4" responsive="screen" :x-gap="16" :y-gap="16">
    <NGridItem>
      <NCard size="small" :title="t('db.suffixes')">
        <template #header-extra>
          <NButton size="small" type="primary" @click="creating = true">
            <template #icon><NIcon><Plus /></NIcon></template>
            {{ t('db.create') }}
          </NButton>
        </template>
        <NSkeleton v-if="!backends" text :repeat="3" />
        <NEmpty v-else-if="!backends.length" :description="t('db.none')" />
        <div v-else class="list">
          <button
            v-for="b in backends"
            :key="b.name"
            type="button"
            class="item"
            :class="{ active: b.name === selectedName }"
            @click="select(b.name)"
          >
            <NIcon :size="18"><Database /></NIcon>
            <span class="item-text">
              <span class="mono suffix">{{ b.suffix }}</span>
              <span class="muted">{{ b.name }}</span>
            </span>
            <NTag v-if="b.readOnly" size="tiny" :bordered="false" type="warning">{{ t('db.readOnly') }}</NTag>
            <NTag v-else-if="b.state && b.state !== 'backend'" size="tiny" :bordered="false">{{ t(`db.states.${b.state}`) }}</NTag>
          </button>
        </div>
      </NCard>
    </NGridItem>

    <NGridItem span="1 l:3">
      <NCard v-if="selected" size="small">
        <template #header>
          <span class="mono">{{ selected.suffix }}</span>
          <span class="muted head-name">{{ selected.name }}</span>
        </template>
        <template #header-extra>
          <RouterLink v-if="selected.rootExists" :to="{ path: '/browser', query: { dn: selected.suffix } }">{{ t('db.openInBrowser') }}</RouterLink>
        </template>
        <NTabs type="line" animated>
          <NTabPane name="settings" :tab="t('db.tabs.settings')">
            <BackendSettings :backend="selected" @changed="onChanged" @deleted="onDeleted" />
          </NTabPane>
          <NTabPane name="indexes" :tab="t('db.tabs.indexes')">
            <BackendIndexes :backend="selected.name" />
            <NCard size="small" :title="t('tasks.title')" class="tasks">
              <TaskList :target="selected.name" :limit="5" />
            </NCard>
          </NTabPane>
          <NTabPane name="transfer" :tab="t('transfer.tab')">
            <BackendTransfer :backend="selected" />
          </NTabPane>
          <NTabPane name="vlv" :tab="t('db.tabs.vlv')">
            <VlvPanel :backend="selected.name" :suffix="selected.suffix" />
          </NTabPane>
        </NTabs>
      </NCard>
    </NGridItem>
  </NGrid>

  <CreateBackendDialog v-model:show="creating" @created="onCreated" />
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.sections {
  margin-bottom: 16px;
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
  flex: 1;
}

.suffix {
  overflow-wrap: anywhere;
}

.muted {
  font-size: 12px;
  color: v-bind('vars.textColor3');
}

.head-name {
  margin-left: 10px;
}

.tasks {
  margin-top: 16px;
}

.section-hint {
  margin: 0 0 12px;
  color: v-bind('vars.textColor2');
}
</style>
