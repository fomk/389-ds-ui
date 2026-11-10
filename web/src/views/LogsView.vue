<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NH1,
  NIcon,
  NInput,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSpin,
  NSwitch,
  useThemeVars,
} from 'naive-ui'
import { RefreshCw, Search } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { currentLocale } from '@/i18n'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { formatBytes } from '@/ldap/values'
import { useServerStore } from '@/stores/server'

type Log = Schemas['Log']

const { t } = useI18n()
const vars = useThemeVars()
const route = useRoute()
const router = useRouter()
const server = useServerStore()

const MAX_FOLLOW_LINES = 3000

const logs = ref<Log[]>()
const lines = ref<string[]>([])
const truncated = ref(false)
const error = ref<LocalizedError>()
const loading = ref(false)
const file = ref<string>('')
const count = ref(500)
const filter = ref('')
const follow = ref(false)
const viewer = ref<HTMLElement>()
let source: EventSource | undefined

const kind = computed({
  get: () => (typeof route.query.log === 'string' ? route.query.log : 'access'),
  set: (v: string) => router.replace({ path: '/logs', query: { log: v } }),
})
const current = computed(() => logs.value?.find((l) => l.kind === kind.value))
const canSeeFiles = computed(() => server.capabilities?.files ?? false)

const fileOptions = computed(() =>
  (current.value?.files ?? []).map((f, i) => ({
    value: i === 0 ? '' : f.name,
    label: `${f.name}${i === 0 ? ` (${t('logs.current')})` : ''} · ${formatBytes(f.size, currentLocale.value)}`,
  })),
)
const countOptions = [200, 500, 1000, 5000].map((n) => ({ value: n, label: String(n) }))

async function loadLogs() {
  if (!canSeeFiles.value) return
  try {
    logs.value = await call(api.GET('/api/v1/logs'))
  } catch (e) {
    error.value = localizeError(e)
  }
}

async function load() {
  if (!canSeeFiles.value || current.value?.sameAs) return
  stopFollow()
  loading.value = true
  error.value = undefined
  try {
    const res = await call(
      api.GET('/api/v1/logs/{kind}', {
        params: {
          path: { kind: kind.value as Log['kind'] },
          query: { file: file.value || undefined, lines: count.value, filter: filter.value.trim() || undefined },
        },
      }),
    )
    lines.value = res.lines
    truncated.value = res.truncated
    await scrollToEnd(true)
  } catch (e) {
    lines.value = []
    error.value = localizeError(e)
  } finally {
    loading.value = false
  }
  if (follow.value && !file.value) startFollow()
}

function startFollow() {
  stopFollow()
  const q = filter.value.trim() ? `?filter=${encodeURIComponent(filter.value.trim())}` : ''
  source = new EventSource(`/api/v1/logs/${encodeURIComponent(kind.value)}/stream${q}`)
  source.addEventListener('lines', async (e) => {
    const atEnd = isAtEnd()
    const batch = JSON.parse((e as MessageEvent).data) as string[]
    lines.value = [...lines.value, ...batch].slice(-MAX_FOLLOW_LINES)
    await scrollToEnd(atEnd)
  })
}

function stopFollow() {
  source?.close()
  source = undefined
}

function isAtEnd() {
  const el = viewer.value
  return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

async function scrollToEnd(force: boolean) {
  await nextTick()
  if (force && viewer.value) viewer.value.scrollTop = viewer.value.scrollHeight
}

onMounted(async () => {
  await loadLogs()
  await load()
})
onUnmounted(stopFollow)
watch(canSeeFiles, async () => {
  await loadLogs()
  await load()
})
watch([kind, file, count], () => load())
watch(kind, () => (file.value = ''))
watch(follow, (on) => (on && !file.value ? startFollow() : stopFollow()))

// Access log: clicking a line narrows the view to its connection.
function onLineClick(line: string) {
  if (kind.value !== 'access') return
  const m = / conn=(\d+) /.exec(line)
  if (!m) return
  filter.value = `conn=${m[1]}`
  load()
}

// Highlight problems: access log results with err≠0, ERR/WARN in the errors log.
function lineClass(line: string): string | undefined {
  if (kind.value === 'access') {
    const m = / err=(\d+)/.exec(line)
    return m && m[1] !== '0' ? 'bad' : undefined
  }
  if (/ - (ERR|CRIT|EMERG|ALERT) - /.test(line) || /"event": "[A-Z_]*FAIL/.test(line)) return 'bad'
  if (/ - (WARN|WARNING) - /.test(line)) return 'warn'
  return undefined
}
</script>

<template>
  <NH1 class="title">{{ t('logs.title') }}</NH1>

  <NAlert v-if="!canSeeFiles" type="info">{{ t('logs.noFiles') }}</NAlert>

  <template v-else>
    <NRadioGroup v-model:value="kind" class="kinds">
      <NRadioButton v-for="l in logs ?? []" :key="l.kind" :value="l.kind">{{ t(`logs.kinds.${l.kind}`) }}</NRadioButton>
    </NRadioGroup>

    <NCard size="small">
      <div class="controls">
        <NSelect v-model:value="file" :options="fileOptions" size="small" class="file" :disabled="!!current?.sameAs" />
        <form class="filter" @submit.prevent="load">
          <NInput v-model:value="filter" size="small" clearable :placeholder="t('logs.filterHint')" :input-props="{ class: 'mono' }">
            <template #prefix><NIcon><Search /></NIcon></template>
          </NInput>
        </form>
        <NSelect v-model:value="count" :options="countOptions" size="small" class="count" />
        <label class="switch" :title="t('logs.follow')">
          <NSwitch v-model:value="follow" size="small" :disabled="!!file" />
          {{ t('logs.follow') }}
        </label>
        <NButton size="small" quaternary :loading="loading" :aria-label="t('logs.refresh')" @click="load">
          <template #icon><NIcon><RefreshCw /></NIcon></template>
        </NButton>
      </div>

      <NAlert v-if="current && !current.enabled" type="warning" class="note">{{ t('logs.disabled') }}</NAlert>
      <NAlert v-if="current?.sameAs" type="info" class="note">{{ t('logs.sameAs', { other: t(`logs.kinds.${current.sameAs}`) }) }}</NAlert>
      <NAlert v-if="current?.buffered && follow" type="info" class="note">{{ t('logs.buffered') }}</NAlert>
      <NAlert v-if="truncated" type="warning" class="note">{{ t('logs.truncated') }}</NAlert>
      <NAlert v-if="error" type="error" :title="t('logs.error')" class="note">{{ error.summary }}</NAlert>

      <div class="status">
        <span>{{ t('logs.shown', { n: lines.length }, lines.length) }}</span>
        <span v-if="follow && !file" class="following"><NSpin :size="12" /> {{ t('logs.streaming') }}</span>
      </div>
      <div ref="viewer" class="viewer mono" role="log" aria-live="off">
        <div v-if="!lines.length && !loading" class="empty">{{ t('logs.empty') }}</div>
        <div
          v-for="(l, i) in lines"
          :key="i"
          class="line"
          :class="[lineClass(l), { clickable: kind === 'access' }]"
          :title="kind === 'access' ? t('logs.clickHint') : undefined"
          @click="onLineClick(l)"
        >
          {{ l }}
        </div>
      </div>
    </NCard>
  </template>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.kinds {
  margin-bottom: 12px;
}

.controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}

.file {
  width: 300px;
}

.filter {
  flex: 1;
  min-width: 240px;
}

.count {
  width: 90px;
}

.switch {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  cursor: pointer;
}

.note {
  margin-bottom: 10px;
}

.status {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: v-bind('vars.textColor3');
  margin-bottom: 4px;
}

.following {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.viewer {
  height: calc(100vh - 380px);
  min-height: 300px;
  overflow: auto;
  padding: 8px 10px;
  border-radius: 4px;
  background: v-bind('vars.codeColor');
  font-size: 12px;
  line-height: 1.55;
}

.line {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.line.clickable {
  cursor: pointer;
}

.line.clickable:hover {
  background: v-bind('vars.hoverColor');
}

.line.bad {
  color: v-bind('vars.errorColor');
}

.line.warn {
  color: v-bind('vars.warningColor');
}

.empty {
  color: v-bind('vars.textColor3');
}
</style>
