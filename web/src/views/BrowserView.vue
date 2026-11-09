<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NCard, NEmpty, NSplit } from 'naive-ui'
import EntryTree from '@/components/browser/EntryTree.vue'
import CreateEntry from '@/components/browser/CreateEntry.vue'
import EntryView from '@/components/browser/EntryView.vue'
import ImportLdifDialog from '@/components/browser/ImportLdifDialog.vue'
import SearchPanel from '@/components/browser/SearchPanel.vue'
import { useServerStore } from '@/stores/server'
import type { Entry } from '@/api/client'
import { splitRDN } from '@/ldap/dn'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

/** The selected entry lives in the URL (?dn=…) so it survives reloads and can be shared. */
const dn = computed(() => (typeof route.query.dn === 'string' ? route.query.dn : undefined))

const tree = ref<InstanceType<typeof EntryTree>>()
/** Parent DN while the create panel is open. */
const creatingUnder = ref<string>()
const searching = ref(false)
const importing = ref(false)
const server = useServerStore()
const searchBase = computed(() => dn.value ?? server.info?.server.namingContexts[0] ?? '')

function select(next: string) {
  creatingUnder.value = undefined
  searching.value = false
  router.push({ path: '/browser', query: { dn: next } })
}

function openSearch() {
  creatingUnder.value = undefined
  searching.value = true
}

async function onCreated(e: Entry) {
  await tree.value?.refresh(parentOf(e.dn))
  select(e.dn)
}

const parentOf = (dn: string) => splitRDN(dn)[1]

// Keep the tree in sync after changes made in the entry view.
function onChanged(dn: string) {
  tree.value?.refresh(parentOf(dn))
}

async function onDeleted(dn: string) {
  const parent = parentOf(dn)
  await tree.value?.refresh(parent)
  if (parent) select(parent)
  else router.push({ path: '/browser' })
}

async function onRenamed(oldDN: string, newDN: string) {
  await tree.value?.refresh(parentOf(oldDN))
  if (parentOf(newDN).toLowerCase() !== parentOf(oldDN).toLowerCase()) await tree.value?.refresh(parentOf(newDN))
  select(newDN)
}
</script>

<template>
  <NSplit class="browser" direction="horizontal" :default-size="0.32" :min="0.18" :max="0.6">
    <template #1>
      <NCard size="small" class="pane" content-class="pane-content">
        <EntryTree ref="tree" :selected="dn" @select="select" @search="openSearch" @import="importing = true" />
      </NCard>
    </template>
    <template #2>
      <div class="detail">
        <SearchPanel v-if="searching" :key="searchBase" :base="searchBase" />
        <CreateEntry
          v-else-if="creatingUnder"
          :key="creatingUnder"
          :parent="creatingUnder"
          @created="onCreated"
          @cancel="creatingUnder = undefined"
        />
        <EntryView
          v-else-if="dn"
          :dn="dn"
          @changed="onChanged"
          @deleted="onDeleted"
          @renamed="onRenamed"
          @create="(p) => (creatingUnder = p)"
        />
        <NCard v-else size="small">
          <NEmpty :description="t('browser.selectEntry')" class="empty" />
        </NCard>
        <ImportLdifDialog v-model:show="importing" @applied="tree?.reload()" />
      </div>
    </template>
  </NSplit>
</template>

<style scoped>
.browser {
  /* Header (56px) + content padding + the no-auth banner. */
  height: calc(100vh - 56px - 48px - 70px);
  min-height: 420px;
}

.pane {
  height: 100%;
}

.pane :deep(.pane-content) {
  height: 100%;
  padding-right: 4px;
}

.detail {
  height: 100%;
  overflow: auto;
  padding-left: 12px;
}

.empty {
  padding: 48px 0;
}
</style>
