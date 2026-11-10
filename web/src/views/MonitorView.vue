<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NGrid,
  NGridItem,
  NH1,
  NIcon,
  NProgress,
  NSkeleton,
  NStatistic,
  NTooltip,
  useThemeVars,
  type DataTableColumns,
} from 'naive-ui'
import { RefreshCw, TriangleAlert } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import LineChart, { type Point } from '@/components/charts/LineChart.vue'
import { currentLocale } from '@/i18n'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { formatBytes } from '@/ldap/values'

type Monitor = Schemas['Monitor']
type Sample = Schemas['Sample']
type Conn = Schemas['ConnectionInfo']
type Cache = Schemas['CacheStats']

const { t } = useI18n()
const vars = useThemeVars()

const data = ref<Monitor>()
const samples = ref<Sample[]>([])
const error = ref<LocalizedError>()
const loading = ref(false)
let timer: ReturnType<typeof setInterval> | undefined

async function load() {
  loading.value = true
  try {
    const [m, h] = await Promise.all([call(api.GET('/api/v1/monitor')), call(api.GET('/api/v1/monitor/history'))])
    data.value = m
    samples.value = h.samples
    error.value = undefined
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  timer = setInterval(load, 10_000)
})
onUnmounted(() => clearInterval(timer))

const nf = computed(() => new Intl.NumberFormat(currentLocale.value))
const num = (v: number) => nf.value.format(v)
const compact = (v: number) =>
  new Intl.NumberFormat(currentLocale.value, { notation: 'compact', maximumFractionDigits: 1 }).format(v)
const bytes = (v: number) => formatBytes(v, currentLocale.value)
const dateTime = (s: string) =>
  new Intl.DateTimeFormat(currentLocale.value, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(s))

const uptime = computed(() => {
  const s = data.value?.server.uptimeSeconds ?? 0
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  return d ? t('monitor.uptimeDays', { d, h }) : t('monitor.uptimeValue', { h, m })
})

// Counters are cumulative: chart their per-second rate. A drop means the
// server restarted, so that interval is skipped.
function rate(key: 'opsCompleted' | 'entriesSent' | 'bytesSent'): Point[] {
  const out: Point[] = []
  for (let i = 1; i < samples.value.length; i++) {
    const a = samples.value[i - 1]!
    const b = samples.value[i]!
    const dt = (Date.parse(b.t) - Date.parse(a.t)) / 1000
    const dv = b[key] - a[key]
    if (dt > 0 && dv >= 0) out.push({ t: Date.parse(b.t), v: dv / dt })
  }
  return out
}
const opsRate = computed(() => rate('opsCompleted'))
const entriesRate = computed(() => rate('entriesSent'))
const bytesRate = computed(() => rate('bytesSent'))
const connections = computed(() => samples.value.map((s) => ({ t: Date.parse(s.t), v: s.connections })))

const perSecond = (v: number) => t('monitor.perSecond', { v: compact(v) })
const bytesPerSecond = (v: number) => t('monitor.perSecond', { v: bytes(v) })

/** A full cache that misses most lookups is too small for the working set. */
const cacheTooSmall = (c: Cache) => c.tries > 1000 && c.hitRatio < 80 && c.maxSize > 0 && c.size >= c.maxSize * 0.9
const pct = (c: Cache) => (c.maxSize > 0 ? Math.min(100, Math.round((c.size / c.maxSize) * 100)) : 0)

const connColumns = computed<DataTableColumns<Conn>>(() => [
  { key: 'id', title: t('monitor.connId'), width: 80 },
  { key: 'opened', title: t('monitor.connOpened'), render: (r) => dateTime(r.opened) },
  { key: 'ops', title: t('monitor.connOps'), render: (r) => num(r.opsCompleted) },
  { key: 'bindDn', title: t('monitor.connBind'), ellipsis: { tooltip: true } },
  { key: 'address', title: t('monitor.connFrom') },
])

const opRows = computed(() =>
  Object.entries(data.value?.operations ?? {})
    .filter(([, v]) => v > 0)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, value]) => ({ name, value })),
)
const opColumns = computed<DataTableColumns<{ name: string; value: number }>>(() => [
  { key: 'name', title: t('monitor.opName'), render: (r) => r.name },
  { key: 'value', title: t('monitor.opValue'), align: 'right', render: (r) => num(r.value) },
])
</script>

<template>
  <div class="head">
    <NH1 class="title">{{ t('monitor.title') }}</NH1>
    <NTooltip>
      <template #trigger>
        <NButton quaternary :loading="loading" :aria-label="t('monitor.refresh')" @click="load">
          <template #icon><NIcon><RefreshCw /></NIcon></template>
        </NButton>
      </template>
      {{ t('monitor.autoRefresh') }}
    </NTooltip>
  </div>

  <NAlert v-if="error" type="error" :title="t('monitor.loadError')" class="block">{{ error.summary }}</NAlert>

  <template v-if="data">
    <!-- Headline numbers: stat tiles, not charts. -->
    <NGrid cols="2 m:3 l:6" responsive="screen" :x-gap="12" :y-gap="12" class="block">
      <NGridItem>
        <NCard size="small">
          <NStatistic :label="t('monitor.uptime')" :value="uptime" />
          <div class="sub">{{ t('monitor.started', { time: dateTime(data.server.startTime) }) }}</div>
        </NCard>
      </NGridItem>
      <NGridItem><NCard size="small"><NStatistic :label="t('monitor.connections')" :value="num(data.server.currentConnections)" /></NCard></NGridItem>
      <NGridItem><NCard size="small"><NStatistic :label="t('monitor.threads')" :value="num(data.server.threads)" /></NCard></NGridItem>
      <NGridItem><NCard size="small"><NStatistic :label="t('monitor.opsCompleted')" :value="compact(data.server.opsCompleted)" /></NCard></NGridItem>
      <NGridItem><NCard size="small"><NStatistic :label="t('monitor.entriesSent')" :value="compact(data.server.entriesSent)" /></NCard></NGridItem>
      <NGridItem><NCard size="small"><NStatistic :label="t('monitor.bytesSent')" :value="bytes(data.server.bytesSent)" /></NCard></NGridItem>
    </NGrid>

    <!-- Small multiples: one metric per chart, never two y-scales. -->
    <NAlert v-if="samples.length < 3" type="info" class="block">{{ t('monitor.charts.collecting') }}</NAlert>
    <NGrid v-else cols="1 m:2" responsive="screen" :x-gap="12" :y-gap="12" class="block">
      <NGridItem>
        <NCard size="small" :title="t('monitor.charts.ops')"><LineChart :points="opsRate" :format="perSecond" :label="t('monitor.charts.ops')" /></NCard>
      </NGridItem>
      <NGridItem>
        <NCard size="small" :title="t('monitor.charts.connections')"><LineChart :points="connections" :format="num" :label="t('monitor.charts.connections')" /></NCard>
      </NGridItem>
      <NGridItem>
        <NCard size="small" :title="t('monitor.charts.entries')"><LineChart :points="entriesRate" :format="perSecond" :label="t('monitor.charts.entries')" /></NCard>
      </NGridItem>
      <NGridItem>
        <NCard size="small" :title="t('monitor.charts.bytes')"><LineChart :points="bytesRate" :format="bytesPerSecond" :label="t('monitor.charts.bytes')" /></NCard>
      </NGridItem>
    </NGrid>

    <NGrid cols="1 l:2" responsive="screen" :x-gap="12" :y-gap="12" class="block">
      <NGridItem>
        <NCard size="small" :title="t('monitor.caches')" class="full">
          <div v-for="be in data.backends" :key="be.name" class="backend">
            <div class="be-name">
              <span class="mono">{{ be.name }}</span> <span class="sub mono">{{ be.suffix }}</span>
            </div>
            <div v-for="[label, c] in ([[t('monitor.entryCache'), be.entryCache], [t('monitor.dnCache'), be.dnCache]] as const)" :key="label" class="cache">
              <div class="cache-head">
                <span>{{ label }}</span>
                <span>
                  <strong>{{ c.hitRatio }}%</strong> {{ t('monitor.hitRatio') }} ·
                  {{ t('monitor.usage', { used: bytes(c.size), max: bytes(c.maxSize) }) }} ·
                  {{ t('monitor.entriesCount', { n: num(c.count) }) }}
                </span>
              </div>
              <NProgress type="line" :percentage="pct(c)" :show-indicator="false" :height="6" />
              <div v-if="cacheTooSmall(c)" class="warn">
                <NIcon :color="vars.warningColor"><TriangleAlert /></NIcon>
                {{ t('monitor.cacheWarning') }}
              </div>
            </div>
          </div>
          <div class="cache">
            <div class="cache-head">
              <span>{{ t('monitor.ndnCache') }}</span>
              <span>
                <strong>{{ data.ndnCache.hitRatio }}%</strong> {{ t('monitor.hitRatio') }} ·
                {{ t('monitor.usage', { used: bytes(data.ndnCache.size), max: bytes(data.ndnCache.maxSize) }) }}
              </span>
            </div>
            <NProgress type="line" :percentage="pct(data.ndnCache)" :show-indicator="false" :height="6" />
          </div>
        </NCard>
      </NGridItem>
      <NGridItem>
        <NCard size="small" :title="t('monitor.disks')" class="full">
          <div v-for="d in data.disks" :key="d.partition" class="cache">
            <div class="cache-head">
              <span class="mono">{{ d.partition }}</span>
              <span><strong>{{ d.usePct }}%</strong> · {{ t('monitor.diskFree', { free: bytes(d.available), size: bytes(d.size) }) }}</span>
            </div>
            <NProgress
              type="line"
              :percentage="d.usePct"
              :status="d.usePct >= 90 ? 'error' : d.usePct >= 80 ? 'warning' : 'default'"
              :show-indicator="false"
              :height="6"
            />
          </div>
        </NCard>
      </NGridItem>
    </NGrid>

    <NGrid cols="1 l:2" responsive="screen" :x-gap="12" :y-gap="12">
      <NGridItem>
        <NCard size="small" :title="t('monitor.connList')">
          <NDataTable :columns="connColumns" :data="data.connections" :row-key="(r: Conn) => r.id" size="small" :max-height="320" />
        </NCard>
      </NGridItem>
      <NGridItem>
        <NCard size="small" :title="t('monitor.ops')">
          <NDataTable :columns="opColumns" :data="opRows" :row-key="(r: { name: string }) => r.name" size="small" :max-height="320" />
        </NCard>
      </NGridItem>
    </NGrid>
  </template>
  <NSkeleton v-else-if="!error" text :repeat="10" />
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.title {
  margin: 0;
}

.block {
  margin-bottom: 12px;
}

.sub {
  font-size: 12px;
  color: v-bind('vars.textColor3');
}

.full {
  height: 100%;
}

.backend + .backend,
.backend + .cache {
  margin-top: 16px;
}

.be-name {
  margin-bottom: 6px;
}

.cache + .cache {
  margin-top: 12px;
}

.cache-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  margin-bottom: 4px;
}

.warn {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 6px;
  font-size: 12px;
  color: v-bind('vars.textColor2');
}
</style>
