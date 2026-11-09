<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NCard, NEmpty, NSplit } from 'naive-ui'
import EntryTree from '@/components/browser/EntryTree.vue'
import EntryView from '@/components/browser/EntryView.vue'
import { splitRDN } from '@/ldap/dn'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

/** The selected entry lives in the URL (?dn=…) so it survives reloads and can be shared. */
const dn = computed(() => (typeof route.query.dn === 'string' ? route.query.dn : undefined))

const tree = ref<InstanceType<typeof EntryTree>>()

function select(next: string) {
  router.push({ path: '/browser', query: { dn: next } })
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
        <EntryTree ref="tree" :selected="dn" @select="select" />
      </NCard>
    </template>
    <template #2>
      <div class="detail">
        <EntryView v-if="dn" :dn="dn" @changed="onChanged" @deleted="onDeleted" @renamed="onRenamed" />
        <NCard v-else size="small">
          <NEmpty :description="t('browser.selectEntry')" class="empty" />
        </NCard>
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
