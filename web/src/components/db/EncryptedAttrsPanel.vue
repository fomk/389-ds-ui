<script setup lang="ts">
/** Attributes a backend stores encrypted (cn=encrypted attributes): list, add, remove. */
import { computed, h, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NDataTable, NEmpty, NIcon, NSelect, NSpace, useDialog, useMessage, type DataTableColumns } from 'naive-ui'
import { Plus, Trash2 } from '@lucide/vue'
import { ApiError, api, call, type Schemas } from '@/api/client'
import { localizeError } from '@/i18n/errors'
import { useAuthStore } from '@/stores/auth'
import { useSchemaStore } from '@/stores/schema'
import ConfirmByName from '@/components/ConfirmByName.vue'

type Encrypted = Schemas['EncryptedAttr']

const props = defineProps<{ backend: string }>()

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()
const schema = useSchemaStore()

const list = ref<Encrypted[]>()
const adding = ref<string | null>(null)
const busy = ref(false)

async function load() {
  try {
    list.value = await call(api.GET('/api/v1/backends/{name}/encrypted-attrs', { params: { path: { name: props.backend } } }))
  } catch (e) {
    message.error(localizeError(e).summary)
  }
}
onMounted(() => {
  void load()
  void schema.load()
})
watch(() => props.backend, load)

const attrOptions = computed(() => {
  const have = new Set((list.value ?? []).map((a) => a.attr.toLowerCase()))
  return (schema.schema?.attributeTypes ?? [])
    .filter((a) => (a.usage ?? 'userApplications') === 'userApplications')
    .map((a) => a.names[0] ?? a.oid)
    .filter((n) => !have.has(n.toLowerCase()))
    .sort((a, b) => a.localeCompare(b))
    .map((n) => ({ label: n, value: n }))
})

async function add() {
  if (!adding.value) return
  busy.value = true
  try {
    await call(api.POST('/api/v1/backends/{name}/encrypted-attrs', { params: { path: { name: props.backend } }, body: { attr: adding.value } }))
    message.success(t('encryption.added', { attr: adding.value }), { duration: 6000 })
    adding.value = null
    await load()
  } catch (e) {
    message.error(localizeError(e).summary)
  } finally {
    busy.value = false
  }
}

/** Removal: refused while entries have values; then the user may force it. */
const forcing = ref<string>()
async function remove(attr: string, force = false) {
  busy.value = true
  try {
    await call(
      api.DELETE('/api/v1/backends/{name}/encrypted-attrs/{attr}', {
        params: { path: { name: props.backend, attr }, query: force ? { force: true } : {} },
      }),
    )
    forcing.value = undefined
    await load()
  } catch (e) {
    if (!force && e instanceof ApiError && e.status === 409) forcing.value = attr
    else message.error(localizeError(e).summary)
  } finally {
    busy.value = false
  }
}
function confirmRemove(attr: string) {
  dialog.warning({
    title: t('encryption.remove'),
    content: t('encryption.removeConfirm', { attr }),
    positiveText: t('encryption.remove'),
    negativeText: t('edit.cancel'),
    onPositiveClick: () => remove(attr),
  })
}

const columns = computed<DataTableColumns<Encrypted>>(() => [
  { key: 'attr', title: t('encryption.attribute'), className: 'mono' },
  { key: 'algorithm', title: t('encryption.algorithm'), width: 140 },
  {
    key: 'actions',
    title: '',
    width: 56,
    render: (a) =>
      auth.readOnly
        ? null
        : h(
            NButton,
            { size: 'tiny', quaternary: true, 'aria-label': t('encryption.remove'), onClick: () => confirmRemove(a.attr) },
            { icon: () => h(NIcon, null, () => h(Trash2)) },
          ),
  },
])
</script>

<template>
  <div>
    <p class="hint">{{ t('encryption.description') }}</p>
    <NAlert type="warning" :show-icon="true" class="block">{{ t('encryption.keyWarning') }}</NAlert>
    <NSpace v-if="!auth.readOnly" class="block" align="center">
      <NSelect
        v-model:value="adding"
        filterable
        clearable
        :options="attrOptions"
        :placeholder="t('encryption.choose')"
        :style="{ width: '320px' }"
      />
      <NButton type="primary" :disabled="!adding" :loading="busy" @click="add">
        <template #icon><NIcon><Plus /></NIcon></template>
        {{ t('encryption.add') }}
      </NButton>
    </NSpace>
    <NDataTable :columns="columns" :data="list ?? []" :loading="!list" size="small" :row-key="(a: Encrypted) => a.attr">
      <template #empty><NEmpty size="small" :description="t('encryption.none')" /></template>
    </NDataTable>
    <p class="hint">{{ t('encryption.existingHint') }}</p>

    <ConfirmByName
      :show="!!forcing"
      :title="t('encryption.remove')"
      :warning="t('encryption.hasValues', { attr: forcing ?? '' })"
      :name="forcing ?? ''"
      :action="t('encryption.removeAnyway')"
      :busy="busy"
      @update:show="(v: boolean) => !v && (forcing = undefined)"
      @confirm="forcing && remove(forcing, true)"
    />
  </div>
</template>

<style scoped>
.hint {
  margin: 0 0 12px;
  font-size: 12.5px;
  opacity: 0.75;
}

.hint:last-of-type {
  margin-top: 12px;
}

.block {
  margin-bottom: 12px;
}
</style>
