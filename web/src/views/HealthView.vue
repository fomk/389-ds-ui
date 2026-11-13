<script setup lang="ts">
/** Health check: problems found on the server, most serious first (GET /health). */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NEmpty, NH1, NIcon, NSkeleton, NTag, useThemeVars } from 'naive-ui'
import { CircleAlert, CircleCheck, Info, RefreshCw, TriangleAlert } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError, type LocalizedError } from '@/i18n/errors'

type Finding = Schemas['Finding']

const { t, te } = useI18n()
const vars = useThemeVars()
const report = ref<Schemas['Report']>()
const error = ref<LocalizedError>()
const busy = ref(false)

async function run() {
  busy.value = true
  error.value = undefined
  try {
    report.value = await call(api.GET('/api/v1/health'))
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
onMounted(run)

const counts = computed(() => {
  const c: Record<string, number> = { error: 0, warning: 0, info: 0 }
  for (const f of report.value?.findings ?? []) c[f.severity] = (c[f.severity] ?? 0) + 1
  return c
})
const tagType = (s: string) => (s === 'error' ? 'error' : s === 'warning' ? 'warning' : 'info')
const icon = (s: string) => (s === 'error' ? CircleAlert : s === 'warning' ? TriangleAlert : Info)
const color = (s: string) => (s === 'error' ? vars.value.errorColor : s === 'warning' ? vars.value.warningColor : vars.value.infoColor)
const text = (f: Finding, part: string) => {
  const key = `health.findings.${f.id}.${part}`
  return te(key) ? t(key, f.params ?? {}) : part === 'title' ? f.id : ''
}
const when = computed(() => (report.value ? new Date(report.value.started).toLocaleString(currentLocale.value) : ''))
</script>

<template>
  <div class="head">
    <NH1 class="title">{{ t('nav.health') }}</NH1>
    <NButton size="small" :loading="busy" @click="run">
      <template #icon><NIcon><RefreshCw /></NIcon></template>
      {{ t('health.run') }}
    </NButton>
  </div>
  <p class="intro">{{ t('health.description') }}</p>

  <NAlert v-if="error" type="error">{{ error.summary }}<div v-if="error.detail" class="mono">{{ error.detail }}</div></NAlert>
  <NSkeleton v-else-if="!report" text :repeat="6" />
  <template v-else>
    <div class="summary">
      <NTag v-for="s in ['error', 'warning', 'info']" :key="s" :type="tagType(s)" :bordered="false" round>
        {{ t(`health.severity.${s}`) }}: {{ counts[s] }}
      </NTag>
      <span class="muted">{{ t('health.ran', { checks: report.checks, when, seconds: report.duration.toFixed(1) }) }}</span>
    </div>

    <NAlert v-for="f in report.failed" :key="f.check" type="default" class="block">
      {{ t('health.checkFailed', { check: f.check }) }} <span class="mono">{{ f.message }}</span>
    </NAlert>

    <NCard v-if="!report.findings.length" size="small">
      <NEmpty :description="t('health.allGood')">
        <template #icon><NIcon :color="vars.successColor"><CircleCheck /></NIcon></template>
      </NEmpty>
    </NCard>
    <NCard v-for="(f, i) in report.findings" :key="i" size="small" class="finding">
      <div class="row">
        <NIcon :size="20" :color="color(f.severity)" class="icon"><component :is="icon(f.severity)" /></NIcon>
        <div class="body">
          <div class="title-row">
            <span class="finding-title">{{ text(f, 'title') }}</span>
            <NTag size="tiny" :bordered="false">{{ t(`health.areas.${f.area}`) }}</NTag>
          </div>
          <p v-if="text(f, 'detail')" class="detail">{{ text(f, 'detail') }}</p>
          <p v-if="text(f, 'fix')" class="fix">{{ text(f, 'fix') }}</p>
        </div>
        <RouterLink v-if="f.link" :to="f.link" class="open">{{ t('health.open') }}</RouterLink>
      </div>
    </NCard>
  </template>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.title {
  margin: 0;
}

.intro {
  margin: 8px 0 16px;
  color: v-bind('vars.textColor3');
  font-size: 13px;
}

.summary {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}

.muted {
  color: v-bind('vars.textColor3');
  font-size: 12.5px;
}

.block,
.finding {
  margin-bottom: 10px;
}

.row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.icon {
  margin-top: 1px;
  flex: none;
}

.body {
  flex: 1;
  min-width: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.finding-title {
  font-weight: 500;
}

.detail,
.fix {
  margin: 4px 0 0;
  font-size: 13px;
  overflow-wrap: anywhere;
}

.fix {
  color: v-bind('vars.textColor3');
}

.open {
  flex: none;
  font-size: 13px;
}
</style>
