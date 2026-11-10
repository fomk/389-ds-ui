<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NIcon,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLayoutSider,
  NMenu,
  NTag,
  useDialog,
  useMessage,
  useThemeVars,
  type MenuOption,
} from 'naive-ui'
import { RotateCw } from '@lucide/vue'
import { api, call } from '@/api/client'
import LocaleSwitcher from '@/components/LocaleSwitcher.vue'
import { localizeError } from '@/i18n/errors'
import { nav } from '@/router'
import { useServerStore } from '@/stores/server'

const server = useServerStore()
const route = useRoute()
const vars = useThemeVars()
const { t } = useI18n()

// Sidebar collapse state is a per-browser convenience; storage may be unavailable.
const COLLAPSED_KEY = 'dsui.sidebarCollapsed'
const collapsed = ref(readCollapsed())
watch(collapsed, (v) => {
  try {
    localStorage.setItem(COLLAPSED_KEY, v ? '1' : '0')
  } catch {
    // not remembered
  }
})
function readCollapsed() {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === '1'
  } catch {
    return false
  }
}

const menuOptions = computed<MenuOption[]>(() =>
  nav.map((item) => ({
    key: item.path,
    label: () =>
      h(
        RouterLink,
        !item.view
          ? {
              to: item.path,
              // Collapsed menu shows its own tooltip with the label; a native title would overlap it.
              title: collapsed.value ? undefined : t('nav.comingInStage', { stage: item.stage }),
              // Muted only in the expanded menu: in the collapsed tooltip it would be unreadable.
              style: collapsed.value ? undefined : { color: vars.value.textColor3 },
            }
          : { to: item.path },
        () => t(`nav.${item.key}`),
      ),
    icon: () => h(NIcon, null, () => h(item.icon, { size: 18 })),
  })),
)

const activeKey = computed(() => {
  const match = nav.filter((n) => (n.path === '/' ? route.path === '/' : route.path.startsWith(n.path)))
  return match.at(-1)?.path ?? null
})

type TagType = 'default' | 'success' | 'warning' | 'error'
const status = computed<{ label: string; type: TagType }>(() => {
  const s = server.state
  if (!s) return { label: t('status.checking'), type: 'default' }
  if (s.ldap === 'ok') return { label: t('status.running'), type: 'success' }
  if (s.server === 'restarting') return { label: t('status.restarting'), type: 'warning' }
  if (s.server === 'stopped') return { label: t('status.stopped'), type: 'error' }
  if (s.ldap === 'unreachable') return { label: t('status.unreachable'), type: 'error' }
  return { label: t('status.error'), type: 'error' }
})
const statusHint = computed(() => (server.apiDown ? t('status.apiDown') : server.state?.ldapError))

const dialog = useDialog()
const message = useMessage()
const canRestart = computed(() => !!server.capabilities && server.capabilities.lifecycle !== 'none')

function restart() {
  dialog.warning({
    title: t('monitor.restart'),
    content: t('monitor.restartConfirm'),
    positiveText: t('monitor.restart'),
    negativeText: t('edit.cancel'),
    onPositiveClick: async () => {
      try {
        await call(api.POST('/api/v1/system/restart'))
        message.info(t('monitor.restartStarted'))
        // Poll faster than usual so the status tag follows the restart.
        for (let i = 0; i < 12; i++) setTimeout(() => server.refreshState(), 1500 * (i + 1))
      } catch (e) {
        message.error(localizeError(e).summary)
      }
    },
  })
}
</script>

<template>
  <NLayout has-sider position="absolute">
    <NLayoutSider
      v-model:collapsed="collapsed"
      bordered
      :width="240"
      collapse-mode="width"
      :collapsed-width="64"
      show-trigger="arrow-circle"
      :native-scrollbar="false"
    >
      <RouterLink to="/" class="brand" :title="collapsed ? t('app.title') : undefined">
        <img src="/favicon.svg" alt="" width="24" height="24" />
        <span v-show="!collapsed" class="brand-text">{{ t('app.title') }}</span>
      </RouterLink>
      <NMenu
        :options="menuOptions"
        :value="activeKey"
        :collapsed="collapsed"
        :collapsed-width="64"
        :collapsed-icon-size="20"
      />
    </NLayoutSider>

    <NLayout>
      <NLayoutHeader bordered class="header">
        <div class="server">
          <span class="mono instance">slapd-{{ server.info?.instance ?? '…' }}</span>
          <span v-if="server.info" class="muted">389-ds {{ server.info.server.version }}</span>
        </div>
        <div class="header-right">
          <NButton v-if="canRestart" size="small" quaternary @click="restart">
            <template #icon><NIcon><RotateCw /></NIcon></template>
            {{ t('monitor.restart') }}
          </NButton>
          <LocaleSwitcher />
          <NTag :type="status.type" size="small" round :title="statusHint">{{ status.label }}</NTag>
        </div>
      </NLayoutHeader>

      <NLayoutContent :native-scrollbar="false" content-class="content">
        <NAlert type="warning" :show-icon="true" class="noauth">
          {{ t('app.noAuthWarning') }}
        </NAlert>
        <NAlert v-if="server.state?.restartRequired" type="info" class="noauth">
          {{ t('monitor.restartRequired', { reasons: server.state.restartReasons.join(', ') }) }}
          <NButton v-if="canRestart" size="small" class="restart-now" @click="restart">{{ t('monitor.restart') }}</NButton>
        </NAlert>
        <slot />
      </NLayoutContent>
    </NLayout>
  </NLayout>
</template>

<style scoped>
.brand {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  height: 56px;
  padding: 0 20px;
  font-weight: 600;
  font-size: 15px;
  color: v-bind('vars.textColor1');
  text-decoration: none;
  white-space: nowrap;
  overflow: hidden;
}

.header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
}

.server {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.instance {
  font-weight: 600;
}

.muted {
  color: v-bind('vars.textColor3');
}

.noauth {
  margin-bottom: 20px;
}

.restart-now {
  margin-left: 12px;
}

:deep(.content) {
  padding: 24px;
  max-width: 1600px;
}
</style>
