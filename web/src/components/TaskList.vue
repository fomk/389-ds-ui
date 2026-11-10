<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NProgress, NTag, useThemeVars } from 'naive-ui'
import { currentLocale } from '@/i18n'
import { useTasksStore, type Task } from '@/stores/tasks'

/** Tasks, newest first; optionally only those of one target (backend). */
const props = defineProps<{ target?: string; limit?: number }>()

const { t, te } = useI18n()
const vars = useThemeVars()
const store = useTasksStore()
const open = ref<string>()

const list = computed(() =>
  store.tasks.filter((x) => !props.target || x.target?.toLowerCase() === props.target.toLowerCase()).slice(0, props.limit ?? 20),
)

const typeLabel = (x: Task) => (te(`tasks.types.${x.type}`) ? t(`tasks.types.${x.type}`) : x.type)
const statusType = (x: Task) =>
  ({ running: 'info', succeeded: 'success', failed: 'error', unknown: 'warning' })[x.status] as 'info' | 'success' | 'error' | 'warning'
const time = (s: string) => new Intl.DateTimeFormat(currentLocale.value, { timeStyle: 'medium' }).format(new Date(s))
</script>

<template>
  <NEmpty v-if="!list.length" :description="t('tasks.none')" size="small" />
  <div v-for="x in list" :key="x.id" class="task">
    <div class="row">
      <NTag size="small" :type="statusType(x)" :bordered="false">{{ t(`tasks.status.${x.status}`) }}</NTag>
      <span class="name">{{ typeLabel(x) }}</span>
      <span class="muted mono">{{ x.target }}</span>
      <span class="muted time">{{ time(x.started) }}</span>
      <NButton v-if="x.log.length" text size="tiny" @click="open = open === x.id ? undefined : x.id">{{ t('tasks.log') }}</NButton>
    </div>
    <NProgress
      v-if="x.status === 'running' && (x.total ?? 0) > 0"
      type="line"
      :percentage="Math.min(100, Math.round(((x.current ?? 0) / (x.total ?? 1)) * 100))"
      :height="4"
      :show-indicator="false"
    />
    <div v-if="x.message" class="muted msg">{{ x.message }}</div>
    <pre v-if="open === x.id" class="log mono">{{ x.log.join('\n') }}</pre>
  </div>
</template>

<style scoped>
.task + .task {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid v-bind('vars.dividerColor');
}

.row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.name {
  font-weight: 500;
}

.time {
  margin-left: auto;
  font-size: 12px;
}

.msg {
  font-size: 12px;
  margin-top: 4px;
  overflow-wrap: anywhere;
}

.muted {
  color: v-bind('vars.textColor3');
}

.log {
  max-height: 220px;
  overflow: auto;
  margin: 6px 0 0;
  padding: 8px;
  font-size: 11.5px;
  border-radius: 4px;
  background: v-bind('vars.codeColor');
  white-space: pre-wrap;
}
</style>
