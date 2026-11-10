<script setup lang="ts">
/** Server settings: the declarative pages (internal/pages/defs) by section. */
import { computed, onMounted, ref } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NCard, NGrid, NGridItem, NH1, NSkeleton, useDialog, useThemeVars } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
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

/** Section order in the menu; unknown sections go last. */
const SECTION_ORDER = ['server', 'database', 'logging', 'security']

const sections = computed(() => {
  const out: { name: string; pages: PageSummary[] }[] = []
  for (const p of list.value ?? []) {
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
  <NH1 class="title">{{ t('nav.settings') }}</NH1>

  <NAlert v-if="error" type="error" :title="t('pages._loadError')">{{ error.summary }}</NAlert>

  <NGrid v-else cols="1 l:4" responsive="screen" :x-gap="16" :y-gap="16">
    <NGridItem>
      <NCard size="small" class="menu-card">
        <NSkeleton v-if="!list" text :repeat="8" />
        <nav v-else :aria-label="t('nav.settings')">
          <div v-for="s in sections" :key="s.name" class="section">
            <div class="section-title">{{ t(`pages._sections.${s.name}`) }}</div>
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
      <SettingsPage v-if="selected" :id="selected" :key="selected" v-model:dirty="dirty" />
    </NGridItem>
  </NGrid>
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.menu-card {
  position: sticky;
  top: 0;
}

.section + .section {
  margin-top: 14px;
}

.section-title {
  padding: 0 10px 4px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  color: v-bind('vars.textColor3');
}

.item {
  display: block;
  width: 100%;
  padding: 7px 10px;
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
  font-weight: 500;
}
</style>
