<script setup lang="ts">
/** Create or edit a user-defined object class. */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NDynamicTags, NForm, NFormItem, NInput, NModal, NRadioButton, NRadioGroup, NSelect, NSpace } from 'naive-ui'
import { api, call, type ObjectClass, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useSchemaStore } from '@/stores/schema'

type Def = Schemas['ObjectClassDef']

const props = defineProps<{ edit?: ObjectClass }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ saved: [name: string] }>()

const { t } = useI18n()
const store = useSchemaStore()
const def = ref<Def>({ oid: '', names: [], kind: 'auxiliary', must: [], may: [] })
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (!v) return
  error.value = undefined
  const c = props.edit
  def.value = c
    ? { oid: c.oid, names: [...c.names], desc: c.desc, sup: [...(c.sup ?? [])], kind: c.kind, must: [...c.must], may: [...c.may] }
    : { oid: '', names: [], kind: 'auxiliary', sup: ['top'], must: [], may: [] }
})

const attrOptions = computed(() =>
  (store.schema?.attributeTypes ?? [])
    .map((a) => a.names[0] ?? a.oid)
    .sort((a, b) => a.localeCompare(b))
    .map((n) => ({ label: n, value: n })),
)
const classOptions = computed(() =>
  (store.schema?.objectClasses ?? [])
    .map((c) => c.names[0] ?? c.oid)
    .sort((a, b) => a.localeCompare(b))
    .map((n) => ({ label: n, value: n })),
)

const complete = computed(() => !!def.value.oid.trim() && def.value.names.length > 0)

function useNameOid() {
  if (def.value.names[0]) def.value.oid = `${def.value.names[0]}-oid`
}

async function save() {
  if (!complete.value) return
  busy.value = true
  error.value = undefined
  const body = { ...def.value }
  try {
    if (props.edit) {
      await call(api.PUT('/api/v1/schema/objectclasses/{name}', { params: { path: { name: props.edit.oid } }, body }))
    } else {
      await call(api.POST('/api/v1/schema/objectclasses', { body }))
    }
    await store.load(true)
    show.value = false
    emit('saved', body.names[0]!)
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    :title="edit ? t('schemaView.editClass') : t('schemaView.newClass')"
    :style="{ width: 'min(620px, 94vw)' }"
    :mask-closable="!busy"
  >
    <NForm label-placement="top" @submit.prevent="save">
      <NFormItem :label="t('schemaView.names')" :feedback="t('schemaView.namesHint')">
        <NDynamicTags v-model:value="def.names" />
      </NFormItem>
      <NFormItem label="OID" :feedback="t('schemaView.oidHint')">
        <div class="grow">
          <NInput v-model:value="def.oid" :disabled="!!edit" :input-props="{ class: 'mono', spellcheck: false }" placeholder="1.3.6.1.4.1.…" />
          <NButton v-if="!edit" :disabled="!def.names[0]" @click="useNameOid">{{ t('schemaView.nameOid') }}</NButton>
        </div>
      </NFormItem>
      <NFormItem :label="t('schemaView.desc')">
        <NInput v-model:value="def.desc" />
      </NFormItem>
      <NFormItem :label="t('schemaView.kind')" :feedback="t('schemaView.kindHint')">
        <NRadioGroup v-model:value="def.kind">
          <NRadioButton v-for="k in ['structural', 'auxiliary', 'abstract']" :key="k" :value="k">{{ t(`schemaView.kinds.${k}`) }}</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem :label="t('schemaView.supClasses')">
        <NSelect v-model:value="def.sup" :options="classOptions" multiple filterable />
      </NFormItem>
      <NFormItem :label="t('schemaView.must')">
        <NSelect v-model:value="def.must" :options="attrOptions" multiple filterable />
      </NFormItem>
      <NFormItem :label="t('schemaView.may')">
        <NSelect v-model:value="def.may" :options="attrOptions" multiple filterable />
      </NFormItem>
    </NForm>
    <NAlert v-if="error" type="error" class="top">
      {{ error.summary }}
      <div v-if="error.detail" class="mono">{{ error.detail }}</div>
    </NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :disabled="!complete" :loading="busy" @click="save">{{ t('pages._save') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.grow {
  display: flex;
  gap: 8px;
  width: 100%;
}

.grow > .n-input {
  flex: 1;
}

.top {
  margin-top: 12px;
}
</style>
