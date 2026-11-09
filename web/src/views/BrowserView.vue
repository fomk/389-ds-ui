<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NCard, NEmpty, NSplit } from 'naive-ui'
import EntryTree from '@/components/browser/EntryTree.vue'
import EntryView from '@/components/browser/EntryView.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

/** The selected entry lives in the URL (?dn=…) so it survives reloads and can be shared. */
const dn = computed(() => (typeof route.query.dn === 'string' ? route.query.dn : undefined))

function select(next: string) {
  router.push({ path: '/browser', query: { dn: next } })
}
</script>

<template>
  <NSplit class="browser" direction="horizontal" :default-size="0.32" :min="0.18" :max="0.6">
    <template #1>
      <NCard size="small" class="pane" content-class="pane-content">
        <EntryTree :selected="dn" @select="select" />
      </NCard>
    </template>
    <template #2>
      <div class="detail">
        <EntryView v-if="dn" :dn="dn" />
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
