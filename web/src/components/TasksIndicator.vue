<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NBadge, NButton, NIcon, NPopover, NSpin } from 'naive-ui'
import { ListChecks } from '@lucide/vue'
import { useTasksStore } from '@/stores/tasks'
import TaskList from './TaskList.vue'

const { t } = useI18n()
const store = useTasksStore()
onMounted(() => store.start())
onUnmounted(() => store.stop())
</script>

<template>
  <NPopover trigger="click" placement="bottom-end" :width="420" scrollable style="max-height: 480px">
    <template #trigger>
      <NButton size="small" quaternary :aria-label="t('tasks.title')" :title="t('tasks.title')">
        <template #icon>
          <NSpin v-if="store.running.length" :size="14" />
          <NIcon v-else><ListChecks /></NIcon>
        </template>
        <NBadge v-if="store.running.length" :value="store.running.length" type="info" />
      </NButton>
    </template>
    <div class="head">
      {{ store.running.length ? t('tasks.running', { n: store.running.length }, store.running.length) : t('tasks.title') }}
    </div>
    <TaskList :limit="15" />
  </NPopover>
</template>

<style scoped>
.head {
  font-weight: 600;
  margin-bottom: 10px;
}
</style>
