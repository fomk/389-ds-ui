<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NDropdown, NIcon } from 'naive-ui'
import { Languages } from '@lucide/vue'
import { availableLocales, currentLocale, setLocale } from '@/i18n'

const { t } = useI18n()

const options = computed(() =>
  availableLocales.map((l) => ({
    key: l.code,
    label: l.name,
    disabled: l.code === currentLocale.value,
  })),
)
const currentName = computed(() => availableLocales.find((l) => l.code === currentLocale.value)?.name)
</script>

<template>
  <NDropdown v-if="availableLocales.length > 1" trigger="click" :options="options" @select="setLocale">
    <NButton quaternary size="small" :title="t('app.language')" :aria-label="t('app.language')">
      <template #icon>
        <NIcon><Languages /></NIcon>
      </template>
      {{ currentName }}
    </NButton>
  </NDropdown>
</template>
