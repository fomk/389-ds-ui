<script setup lang="ts">
import { computed, onMounted, onUnmounted, watchEffect } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  darkTheme,
  useOsTheme,
  type GlobalThemeOverrides,
} from 'naive-ui'
import AppLayout from '@/layout/AppLayout.vue'
import { currentLocale } from '@/i18n'
import { naiveLocaleFor } from '@/i18n/naive'
import { useServerStore } from '@/stores/server'

const { t } = useI18n()
const route = useRoute()

const osTheme = useOsTheme()
const theme = computed(() => (osTheme.value === 'dark' ? darkTheme : null))
const naive = computed(() => naiveLocaleFor(currentLocale.value))

const overrides: GlobalThemeOverrides = {
  common: {
    fontFamily: "Inter, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
    fontFamilyMono: "ui-monospace, 'JetBrains Mono', 'Cascadia Code', Menlo, monospace",
  },
}

watchEffect(() => {
  const section = route.meta.nav ? t(`nav.${route.meta.nav}`) : ''
  document.title = section ? `${section} · ${t('app.title')}` : t('app.title')
})

const server = useServerStore()
onMounted(() => server.start())
onUnmounted(() => server.stop())
</script>

<template>
  <NConfigProvider
    :theme="theme"
    :theme-overrides="overrides"
    :locale="naive.locale"
    :date-locale="naive.dateLocale"
  >
    <NMessageProvider>
      <NDialogProvider>
        <AppLayout>
          <RouterView />
        </AppLayout>
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>
