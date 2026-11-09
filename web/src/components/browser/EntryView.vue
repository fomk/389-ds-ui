<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NAlert,
  NButton,
  NCard,
  NDropdown,
  NIcon,
  NSkeleton,
  NSpace,
  NSwitch,
  NTable,
  NTag,
  NTooltip,
  useMessage,
  useThemeVars,
} from 'naive-ui'
import { Copy, EllipsisVertical, Pencil, RefreshCw } from '@lucide/vue'
import { api, call, type Entry, type EntryAttribute } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useSchemaStore } from '@/stores/schema'
import AttributeValue from './AttributeValue.vue'
import DeleteDialog from './DeleteDialog.vue'
import EntryEditor from './EntryEditor.vue'
import PasswordDialog from './PasswordDialog.vue'
import RenameDialog from './RenameDialog.vue'

const props = defineProps<{ dn: string }>()
const emit = defineEmits<{ changed: [dn: string]; deleted: [dn: string]; renamed: [oldDN: string, newDN: string] }>()

const { t } = useI18n()
const message = useMessage()
const vars = useThemeVars()
const schema = useSchemaStore()

const entry = ref<Entry>()
const error = ref<LocalizedError>()
const loading = ref(false)
const showOperational = ref(false)
const editing = ref(false)
const dialog = ref<'rename' | 'password' | 'delete' | null>(null)

async function load() {
  loading.value = true
  error.value = undefined
  try {
    const [e] = await Promise.all([
      call(api.GET('/api/v1/entries', { params: { query: { dn: props.dn } } })),
      schema.load(),
    ])
    entry.value = e
  } catch (e) {
    entry.value = undefined
    error.value = localizeError(e)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.dn,
  () => {
    editing.value = false
    load()
  },
  { immediate: true },
)

function onSaved(updated: Entry) {
  entry.value = updated
  editing.value = false
  emit('changed', updated.dn)
}

const canSetPassword = computed(() => {
  if (!entry.value) return false
  const { must, may, any } = schema.allowed(entry.value.objectClasses)
  return any || [...must, ...may].some((a) => a.toLowerCase() === 'userpassword')
})

const actionOptions = computed(() => [
  { key: 'rename', label: t('actions.rename') },
  ...(canSetPassword.value ? [{ key: 'password', label: t('actions.password') }] : []),
  { type: 'divider', key: 'd1' },
  { key: 'delete', label: t('actions.delete'), props: { style: { color: vars.value.errorColor } } },
])

function onAction(key: string) {
  dialog.value = key as typeof dialog.value
}

const attributes = computed(() =>
  (entry.value?.attributes ?? []).filter(
    // objectClass is shown as tags above the table.
    (a) => a.name.toLowerCase() !== 'objectclass' && (showOperational.value || !a.operational),
  ),
)

function typeOf(a: EntryAttribute) {
  return schema.attr(a.name)
}

async function copyDN() {
  try {
    await navigator.clipboard.writeText(props.dn)
    message.success(t('browser.copied'))
  } catch {
    // Clipboard API unavailable (non-secure context): nothing sensible to do.
  }
}

defineExpose({ reload: load })
</script>

<template>
  <NAlert v-if="error" type="error" :title="t('browser.loadError')">
    {{ error.summary }}
    <div v-if="error.detail" class="mono muted">{{ error.detail }}</div>
  </NAlert>

  <NCard v-else-if="entry" size="small" class="entry">
    <template #header>
      <div class="title">
        <span class="mono rdn">{{ entry.rdn }}</span>
        <NTag v-if="entry.numSubordinates" size="small" :bordered="false">
          {{ t('browser.children', { n: entry.numSubordinates }, entry.numSubordinates) }}
        </NTag>
      </div>
      <div class="dn mono">
        {{ entry.dn }}
        <NButton size="tiny" quaternary :title="t('browser.copyDN')" :aria-label="t('browser.copyDN')" @click="copyDN">
          <template #icon><NIcon><Copy /></NIcon></template>
        </NButton>
      </div>
    </template>
    <template #header-extra>
      <NSpace v-if="!editing" align="center" :size="12">
        <NButton size="small" @click="editing = true">
          <template #icon><NIcon><Pencil /></NIcon></template>
          {{ t('edit.edit') }}
        </NButton>
        <NDropdown trigger="click" :options="actionOptions" @select="onAction">
          <NButton size="small" quaternary :title="t('actions.more')" :aria-label="t('actions.more')">
            <template #icon><NIcon><EllipsisVertical /></NIcon></template>
          </NButton>
        </NDropdown>
        <label class="switch">
          <NSwitch v-model:value="showOperational" size="small" />
          {{ t('browser.showOperational') }}
        </label>
        <NButton size="small" quaternary :loading="loading" :title="t('browser.refresh')" @click="load">
          <template #icon><NIcon><RefreshCw /></NIcon></template>
        </NButton>
      </NSpace>
    </template>

    <EntryEditor v-if="editing" :entry="entry" @saved="onSaved" @cancel="editing = false" @reload="load().then(() => (editing = true))" />

    <template v-else>
    <NSpace class="classes" :size="6">
      <NTooltip v-for="oc in entry.objectClasses" :key="oc" :disabled="!schema.objectClass(oc)?.desc">
        <template #trigger>
          <NTag size="small" type="info" :bordered="false">{{ oc }}</NTag>
        </template>
        {{ schema.objectClass(oc)?.desc }}
      </NTooltip>
    </NSpace>

    <NTable size="small" :bordered="false" :single-line="false" class="attrs">
      <thead>
        <tr>
          <th class="name-col">{{ t('browser.attribute') }}</th>
          <th>{{ t('browser.values') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="a in attributes" :key="a.name" :class="{ operational: a.operational }">
          <td class="name-col">
            <NTooltip placement="right">
              <template #trigger>
                <span class="attr-name">{{ a.name }}</span>
              </template>
              <template v-if="typeOf(a)">
                <div v-if="typeOf(a)!.desc">{{ typeOf(a)!.desc }}</div>
                <div class="mono">{{ typeOf(a)!.names.join(', ') }} · {{ typeOf(a)!.oid }}</div>
                <div>{{ t('browser.attrInfo.syntax') }}: <span class="mono">{{ typeOf(a)!.syntax }}</span></div>
                <div>
                  <template v-if="typeOf(a)!.singleValue">{{ t('browser.attrInfo.singleValue') }} </template>
                  <template v-if="a.operational">· {{ t('browser.attrInfo.operational') }} </template>
                  <template v-if="typeOf(a)!.noUserModification">· {{ t('browser.attrInfo.readOnly') }}</template>
                </div>
              </template>
              <span v-else>{{ t('browser.attrInfo.notInSchema') }}</span>
            </NTooltip>
          </td>
          <td>
            <div v-for="(v, i) in a.values" :key="i" class="value">
              <AttributeValue :attribute="a" :value="v" :index="i" :type="typeOf(a)" />
            </div>
          </td>
        </tr>
      </tbody>
    </NTable>
    </template>

    <RenameDialog :show="dialog === 'rename'" :entry="entry" @update:show="(v) => !v && (dialog = null)" @renamed="(o, n) => emit('renamed', o, n)" />
    <PasswordDialog :show="dialog === 'password'" :entry="entry" @update:show="(v) => !v && (dialog = null)" @changed="load" />
    <DeleteDialog :show="dialog === 'delete'" :entry="entry" @update:show="(v) => !v && (dialog = null)" @deleted="(dn) => emit('deleted', dn)" />
  </NCard>

  <NCard v-else size="small">
    <NSkeleton text :repeat="8" />
  </NCard>
</template>

<style scoped>
.title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 18px;
  font-weight: 600;
}

.dn {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 2px;
  font-size: 12px;
  font-weight: normal;
  color: v-bind('vars.textColor3');
  overflow-wrap: anywhere;
}

.switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}

.classes {
  margin-bottom: 12px;
}

.name-col {
  width: 28%;
  min-width: 140px;
  vertical-align: top;
}

.attr-name {
  font-weight: 500;
  cursor: help;
  overflow-wrap: anywhere;
}

.operational .attr-name {
  color: v-bind('vars.textColor3');
  font-weight: normal;
}

.value + .value {
  margin-top: 4px;
}

.muted {
  color: v-bind('vars.textColor3');
}
</style>
