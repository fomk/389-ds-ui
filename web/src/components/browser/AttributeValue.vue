<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NIcon, NImage, NTag, useThemeVars } from 'naive-ui'
import { Download } from '@lucide/vue'
import type { AttributeType, EntryAttribute } from '@/api/client'
import { currentLocale } from '@/i18n'
import { base64Size, downloadBase64, formatBytes, parseLdapTime, UTC_TIME_SYNTAX } from '@/ldap/values'

const props = defineProps<{
  attribute: EntryAttribute
  value: string
  index: number
  type?: AttributeType
}>()

const { t } = useI18n()
const vars = useThemeVars()

const kind = computed(() => {
  if (props.attribute.redacted) return 'password'
  if (props.attribute.binary) return 'binary'
  return props.type?.kind ?? 'string'
})

const time = computed(() =>
  kind.value === 'time' ? parseLdapTime(props.value, props.type?.syntax === UTC_TIME_SYNTAX) : null,
)
const timeText = computed(() =>
  time.value
    ? new Intl.DateTimeFormat(currentLocale.value, { dateStyle: 'medium', timeStyle: 'medium' }).format(time.value)
    : props.value,
)

const isJpeg = computed(() => kind.value === 'binary' && props.attribute.name.toLowerCase().startsWith('jpegphoto'))
const size = computed(() => formatBytes(base64Size(props.value), currentLocale.value))

function download() {
  const name = props.attribute.name.split(';')[0]!
  downloadBase64(props.value, `${name}-${props.index + 1}${isJpeg.value ? '.jpg' : '.bin'}`)
}
</script>

<template>
  <RouterLink v-if="kind === 'dn'" :to="{ path: '/browser', query: { dn: value } }" class="mono dn">{{ value }}</RouterLink>

  <span v-else-if="kind === 'time'" :title="value">{{ timeText }}</span>

  <NTag v-else-if="kind === 'boolean'" size="small" :type="value.toUpperCase() === 'TRUE' ? 'success' : 'default'" :bordered="false">
    {{ value }}
  </NTag>

  <span v-else-if="kind === 'password'" class="muted">
    •••••••• <span class="mono">{{ t('browser.passwordHash', { scheme: value }) }}</span>
  </span>

  <span v-else-if="kind === 'binary'" class="binary">
    <NImage v-if="isJpeg" :src="`data:image/jpeg;base64,${value}`" width="64" object-fit="cover" />
    <span class="muted">{{ t('browser.binary', { size }) }}</span>
    <NButton size="tiny" quaternary @click="download">
      <template #icon><NIcon><Download /></NIcon></template>
      {{ t('browser.download') }}
    </NButton>
  </span>

  <span v-else class="text">{{ value }}</span>
</template>

<style scoped>
.text {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.dn {
  overflow-wrap: anywhere;
  color: v-bind('vars.primaryColor');
  text-decoration: none;
}

.dn:hover {
  text-decoration: underline;
}

.binary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.muted {
  opacity: 0.65;
}
</style>
