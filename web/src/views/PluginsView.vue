<script setup lang="ts">
/**
 * Plug-ins: the list with on/off switches, details of the selected one and,
 * for plug-ins that have a settings page (internal/pages/defs/plugins.yaml),
 * its configuration.
 */
import { computed, onMounted, ref } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NCard,
  NEmpty,
  NGrid,
  NGridItem,
  NH1,
  NIcon,
  NInput,
  NRadioButton,
  NRadioGroup,
  NSkeleton,
  NSpace,
  NSwitch,
  NTag,
  NTooltip,
  useDialog,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { Lock, Search, SlidersHorizontal } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'
import CollectionPanel from '@/components/settings/CollectionPanel.vue'
import PluginTaskButton from '@/components/settings/PluginTaskButton.vue'
import SettingsPage from '@/components/settings/SettingsPage.vue'

type Plugin = Schemas['Plugin']

const { t, te } = useI18n()
const vars = useThemeVars()
const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()

const plugins = ref<Plugin[]>()
const dynamic = ref(true)
const pages = ref<Record<string, string>>({}) // plug-in cn → page id
const collections = ref<Schemas['CollectionSummary'][]>([])
const pluginTasks = ref<Schemas['PluginTask'][]>([])
const error = ref<LocalizedError>()
const query = ref('')
/** all | configurable | enabled */
const show = ref<'configurable' | 'enabled' | 'all'>('configurable')
const busy = ref<string>()
const dirty = ref(false)

async function load() {
  try {
    const [list, summaries, cols, ptasks] = await Promise.all([
      call(api.GET('/api/v1/plugins')),
      call(api.GET('/api/v1/pages')),
      call(api.GET('/api/v1/collections')),
      call(api.GET('/api/v1/plugin-tasks')),
    ])
    collections.value = cols
    pluginTasks.value = ptasks
    plugins.value = list.plugins
    dynamic.value = list.dynamic
    pages.value = Object.fromEntries(summaries.filter((p) => p.plugin).map((p) => [p.plugin!.toLowerCase(), p.id]))
  } catch (e) {
    error.value = localizeError(e)
  }
}
onMounted(load)

const pageOf = (p: Plugin) => pages.value[p.name.toLowerCase()]
const same = (a?: string, b?: string) => !!a && !!b && a.toLowerCase() === b.toLowerCase()
/** Top-level collections of a plug-in; nested ones are shown inside their parent's items. */
const collectionsOf = (p: Plugin) => collections.value.filter((c) => same(c.plugin, p.name) && !c.under).sort((a, b) => a.order - b.order)
const nestedIn = (id: string) => collections.value.filter((c) => c.under === id).map((c) => c.id)
const tasksOf = (p: Plugin) => pluginTasks.value.filter((x) => same(x.plugin, p.name))
const configurable = (p: Plugin) => !!pageOf(p) || collectionsOf(p).length > 0 || tasksOf(p).length > 0

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return (plugins.value ?? []).filter((p) => {
    if (show.value === 'configurable' && !configurable(p)) return false
    if (show.value === 'enabled' && !p.enabled) return false
    return !q || p.name.toLowerCase().includes(q) || p.type.toLowerCase().includes(q)
  })
})

const selectedName = computed(() => (typeof route.query.p === 'string' ? route.query.p : filtered.value[0]?.name))
const selected = computed(() => plugins.value?.find((p) => p.name === selectedName.value))
const selectedPage = computed(() => (selected.value ? pageOf(selected.value) : undefined))

/** 389-ds fills unknown plug-in metadata with "none". */
const known = (v?: string) => !!v && v.toLowerCase() !== 'none'

function select(name: string) {
  router.replace({ path: '/plugins', query: { p: name } })
}

/** Our description for plug-ins with a page, the server's for the rest (English). */
function description(p: Plugin) {
  const id = pageOf(p)
  return id && te(`pages.${id}.description`) ? t(`pages.${id}.description`) : p.description
}

async function toggle(p: Plugin, enabled: boolean) {
  busy.value = p.name
  try {
    const res = await call(api.PATCH('/api/v1/plugins/{name}', { params: { path: { name: p.name } }, body: { enabled } }))
    const i = plugins.value!.findIndex((x) => x.name === p.name)
    plugins.value!.splice(i, 1, res.plugin)
    const msg = t(enabled ? 'plugins.enabled' : 'plugins.disabled', { name: p.name })
    if (res.restart) {
      message.warning(`${msg} ${t('plugins.afterRestart')}`, { duration: 6000 })
      void server.refreshState()
    } else {
      message.success(msg)
    }
  } catch (e) {
    const err = localizeError(e)
    message.error(`${t('plugins.toggleError')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
  } finally {
    busy.value = undefined
  }
}

function confirmLeave(): Promise<boolean> {
  if (!dirty.value) return Promise.resolve(true)
  return new Promise((resolve) => {
    dialog.warning({
      title: t('pages._leaveTitle'),
      content: t('pages._leaveText'),
      positiveText: t('pages._leave'),
      negativeText: t('pages._stay'),
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false),
    })
  })
}
onBeforeRouteUpdate(confirmLeave)
onBeforeRouteLeave(confirmLeave)
</script>

<template>
  <NH1 class="title">{{ t('nav.plugins') }}</NH1>

  <NAlert v-if="error" type="error" :title="t('plugins.loadError')">{{ error.summary }}</NAlert>

  <template v-else>
    <NAlert v-if="plugins && !dynamic" type="info" class="block">
      {{ t('plugins.notDynamic') }}
      <RouterLink :to="{ path: '/settings', query: { page: 'server-general' } }">{{ t('plugins.notDynamicLink') }}</RouterLink>
    </NAlert>

    <NGrid cols="1 l:5" responsive="screen" :x-gap="16" :y-gap="16">
      <NGridItem span="1 l:2">
        <NCard size="small" class="list-card">
          <div class="filters">
            <NInput v-model:value="query" :placeholder="t('plugins.search')" clearable size="small">
              <template #prefix><NIcon><Search /></NIcon></template>
            </NInput>
            <NRadioGroup v-model:value="show" size="small">
              <NRadioButton value="configurable">{{ t('plugins.showConfigurable') }}</NRadioButton>
              <NRadioButton value="enabled">{{ t('plugins.showEnabled') }}</NRadioButton>
              <NRadioButton value="all">{{ t('plugins.showAll') }}</NRadioButton>
            </NRadioGroup>
          </div>
          <NSkeleton v-if="!plugins" text :repeat="10" />
          <NEmpty v-else-if="!filtered.length" :description="t('plugins.none')" />
          <div v-else class="list">
            <div
              v-for="p in filtered"
              :key="p.dn"
              class="item"
              :class="{ active: p.name === selectedName }"
              role="button"
              tabindex="0"
              @click="select(p.name)"
              @keydown.enter="select(p.name)"
            >
              <span class="item-text">
                <span class="name">
                  {{ p.name }}
                  <NIcon v-if="configurable(p)" :size="13" class="cfg" :title="t('plugins.hasSettings')"><SlidersHorizontal /></NIcon>
                </span>
                <span class="muted">{{ p.type }}</span>
              </span>
              <NTooltip v-if="p.protected">
                <template #trigger><NIcon :size="15" class="lock"><Lock /></NIcon></template>
                {{ t('plugins.protected') }}
              </NTooltip>
              <NSwitch
                v-else
                size="small"
                :value="p.enabled"
                :loading="busy === p.name"
                :aria-label="p.name"
                @click.stop
                @update:value="(v: boolean) => toggle(p, v)"
              />
            </div>
          </div>
        </NCard>
      </NGridItem>

      <NGridItem span="1 l:3">
        <NCard v-if="selected" size="small" :title="selected.name">
          <template #header-extra>
            <NTag :type="selected.enabled ? 'success' : 'default'" size="small" :bordered="false">
              {{ selected.enabled ? t('plugins.on') : t('plugins.off') }}
            </NTag>
          </template>
          <p v-if="!selectedPage && description(selected)" class="description">{{ description(selected) }}</p>
          <dl class="facts">
            <dt>{{ t('plugins.type') }}</dt><dd class="mono">{{ selected.type }}</dd>
            <template v-if="known(selected.version)"><dt>{{ t('plugins.version') }}</dt><dd>{{ selected.version }}</dd></template>
            <template v-if="known(selected.vendor)"><dt>{{ t('plugins.vendor') }}</dt><dd>{{ selected.vendor }}</dd></template>
            <template v-if="selected.path"><dt>{{ t('plugins.path') }}</dt><dd class="mono">{{ selected.path }}</dd></template>
            <template v-if="selected.dependsOn.length">
              <dt>{{ t('plugins.dependsOn') }}</dt><dd>{{ selected.dependsOn.join(', ') }}</dd>
            </template>
            <dt>DN</dt><dd class="mono">{{ selected.dn }}</dd>
          </dl>
          <NSpace v-if="tasksOf(selected).length" class="tasks">
            <PluginTaskButton v-for="task in tasksOf(selected)" :key="task.id" :task="task" />
          </NSpace>
          <NAlert v-if="configurable(selected) && !selected.enabled" type="default" :show-icon="false" class="hint">
            {{ t('plugins.disabledHint') }}
          </NAlert>
        </NCard>
        <SettingsPage v-if="selectedPage" :id="selectedPage" :key="selectedPage" v-model:dirty="dirty" class="settings" />
        <CollectionPanel v-for="c in selected ? collectionsOf(selected) : []" :id="c.id" :key="c.id" :nested="nestedIn(c.id)" class="settings" />
      </NGridItem>
    </NGrid>
  </template>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.block {
  margin-bottom: 16px;
}

.filters {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 10px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: calc(100vh - 330px);
  overflow-y: auto;
}

.item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 10px;
  border-radius: 6px;
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
  flex: 1;
  min-width: 0;
}

.name {
  display: flex;
  align-items: center;
  gap: 6px;
  overflow-wrap: anywhere;
}

.cfg {
  color: v-bind('vars.primaryColor');
}

.lock {
  color: v-bind('vars.textColor3');
}

.muted {
  font-size: 12px;
  color: v-bind('vars.textColor3');
}

.description {
  margin: 0 0 12px;
  color: v-bind('vars.textColor2');
}

.facts {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 4px 16px;
  margin: 0;
  font-size: 13px;
}

.facts dt {
  color: v-bind('vars.textColor3');
}

.facts dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.hint {
  margin-top: 12px;
}

.tasks {
  margin-top: 12px;
}

.settings {
  margin-top: 16px;
}
</style>
