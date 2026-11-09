<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { NCard, NEmpty, NH1, NIcon } from 'naive-ui'
import { Wrench } from '@lucide/vue'
import type { NavKey } from '@/router'

defineProps<{ navKey: NavKey; stage: number }>()
const { t } = useI18n()
</script>

<template>
  <NH1 class="title">{{ t(`nav.${navKey}`) }}</NH1>
  <NCard>
    <NEmpty :description="t('placeholder.comingInStage', { stage })" size="large" class="empty">
      <template #icon>
        <NIcon><Wrench /></NIcon>
      </template>
    </NEmpty>
  </NCard>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.empty {
  padding: 32px 0;
}
</style>
