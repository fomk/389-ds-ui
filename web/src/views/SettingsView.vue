<script setup lang="ts">
/** Server settings: the declarative pages (internal/pages/defs) by section. */
import { computed, onMounted, ref, type Component } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NCard, NGrid, NGridItem, NH1, NIcon, NSkeleton, NSwitch, useDialog, useThemeVars } from 'naive-ui'
import { Database, ScrollText, Server, ShieldCheck, SlidersHorizontal } from '@lucide/vue'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import RootPasswordPanel from '@/components/settings/RootPasswordPanel.vue'
import SaslMappingsPanel from '@/components/settings/SaslMappingsPanel.vue'
import SettingsPage from '@/components/settings/SettingsPage.vue'

type PageSummary = Schemas['PageSummary']

const { t } = useI18n()
const vars = useThemeVars()
const route = useRoute()
const router = useRouter()
const dialog = useDialog()

const list = ref<PageSummary[]>()
const error = ref<LocalizedError>()
const dirty = ref(false)

/** Show LDAP attribute names under the labels (per-browser preference). */
const ATTRS_KEY = 'dsui.settings.attrs'
const showAttrs = ref(false)
try {
  showAttrs.value = localStorage.getItem(ATTRS_KEY) === '1'
} catch {
  // storage unavailable: keep the default
}
function setShowAttrs(v: boolean) {
  showAttrs.value = v
  try {
    localStorage.setItem(ATTRS_KEY, v ? '1' : '0')
  } catch {
    // ignore
  }
}

/** Section order in the menu; unknown sections go last. */
const SECTION_ORDER = ['server', 'database', 'logging', 'security']
const SECTION_ICONS: Record<string, typeof Server> = { server: Server, database: Database, logging: ScrollText, security: ShieldCheck }

/**
 * Hand-written screens shown among the declarative pages (titles in
 * pages.<id>.title like the others).
 */
const CUSTOM: (PageSummary & { component: Component })[] = [
  { id: 'sasl-mappings', section: 'security', order: 15, component: SaslMappingsPanel },
  { id: 'root-password', section: 'security', order: 25, component: RootPasswordPanel },
]
const custom = computed(() => CUSTOM.find((c) => c.id === selected.value))

/** Pages that have their own section; old links are redirected there. */
const ELSEWHERE: Record<string, string> = { 'password-policy': '/policies' }
if (typeof route.query.page === 'string' && ELSEWHERE[route.query.page]) router.replace(ELSEWHERE[route.query.page]!)

const sections = computed(() => {
  const out: { name: string; pages: PageSummary[] }[] = []
  // Plug-in pages live on the Plugins page.
  const pages = list.value ? [...list.value.filter((x) => !x.plugin && !x.entryClass && !ELSEWHERE[x.id] && x.section !== 'tls'), ...CUSTOM] : []
  pages.sort((a, b) => a.order - b.order)
  for (const p of pages) {
    let s = out.find((x) => x.name === p.section)
    if (!s) out.push((s = { name: p.section, pages: [] }))
    s.pages.push(p)
  }
  const rank = (n: string) => (SECTION_ORDER.includes(n) ? SECTION_ORDER.indexOf(n) : SECTION_ORDER.length)
  return out.sort((a, b) => rank(a.name) - rank(b.name))
})

const selected = computed(() => {
  const q = route.query.page
  const ids = sections.value.flatMap((s) => s.pages.map((p) => p.id))
  return typeof q === 'string' && ids.includes(q) ? q : ids[0]
})

onMounted(async () => {
  try {
    list.value = await call(api.GET('/api/v1/pages'))
  } catch (e) {
    error.value = localizeError(e)
  }
})

function select(id: string) {
  router.replace({ path: '/settings', query: { page: id } })
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
  <div class="title-row">
    <NH1 class="title">{{ t('nav.settings') }}</NH1>
    <label class="attrs-toggle">
      <NSwitch :value="showAttrs" size="small" @update:value="setShowAttrs" />
      {{ t('pages._showAttrs') }}
    </label>
  </div>

  <NAlert v-if="error" type="error" :title="t('pages._loadError')">{{ error.summary }}</NAlert>

  <NGrid v-else cols="1 l:4" responsive="screen" :x-gap="16" :y-gap="16">
    <NGridItem>
      <NCard size="small" class="menu-card">
        <NSkeleton v-if="!list" text :repeat="8" />
        <nav v-else :aria-label="t('nav.settings')">
          <div v-for="s in sections" :key="s.name" class="section">
            <div class="section-title">
              <NIcon :size="16"><component :is="SECTION_ICONS[s.name] ?? SlidersHorizontal" /></NIcon>
              {{ t(`pages._sections.${s.name}`) }}
            </div>
            <button
              v-for="p in s.pages"
              :key="p.id"
              type="button"
              class="item"
              :class="{ active: p.id === selected }"
              :aria-current="p.id === selected ? 'page' : undefined"
              @click="select(p.id)"
            >
              {{ t(`pages.${p.id}.title`) }}
            </button>
          </div>
        </nav>
      </NCard>
    </NGridItem>
    <NGridItem span="1 l:3">
      <component :is="custom.component" v-if="custom" />
      <SettingsPage v-else-if="selected" :id="selected" :key="selected" v-model:dirty="dirty" :show-attrs="showAttrs" />
    </NGridItem>
  </NGrid>
</template>

<style scoped>
.title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 20px;
}

.title {
  margin: 0;
}

.attrs-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: v-bind('vars.textColor2');
  cursor: pointer;
}

.menu-card {
  position: sticky;
  top: 0;
}

.section + .section {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid v-bind('vars.dividerColor');
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px 6px;
  font-size: 13px;
  font-weight: 600;
  color: v-bind('vars.textColor1');
}

.section-title .n-icon {
  color: v-bind('vars.primaryColor');
}

.item {
  display: block;
  width: 100%;
  padding: 6px 10px 6px 34px;
  font-size: 13.5px;
  color: v-bind('vars.textColor2');
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.item:hover {
  background: v-bind('vars.hoverColor');
}

.item.active {
  background: color-mix(in srgb, v-bind('vars.primaryColor') 12%, transparent);
  color: v-bind('vars.textColor1');
  font-weight: 500;
}
</style>
