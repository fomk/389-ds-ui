<script setup lang="ts">
/** Create or edit a user-defined attribute type. */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCheckbox, NDynamicTags, NForm, NFormItem, NInput, NModal, NSelect, NSpace } from 'naive-ui'
import { api, call, type AttributeType, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useSchemaStore } from '@/stores/schema'
import { ruleOptions } from './options'

type Def = Schemas['AttributeTypeDef']

const props = defineProps<{ edit?: AttributeType }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ saved: [name: string] }>()

const { t } = useI18n()
const store = useSchemaStore()
const def = ref<Def>({ oid: '', names: [] })
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (!v) return
  error.value = undefined
  const a = props.edit
  def.value = a
    ? {
        oid: a.oid,
        names: [...a.names],
        desc: a.desc,
        sup: a.sup,
        equality: a.equality,
        ordering: a.ordering,
        substr: a.substr,
        syntax: a.syntax,
        singleValue: a.singleValue,
      }
    : { oid: '', names: [], syntax: '1.3.6.1.4.1.1466.115.121.1.15', equality: 'caseIgnoreMatch', substr: 'caseIgnoreSubstringsMatch' }
})

const attrOptions = computed(() =>
  (store.schema?.attributeTypes ?? [])
    .map((a) => a.names[0] ?? a.oid)
    .sort((a, b) => a.localeCompare(b))
    .map((n) => ({ label: n, value: n })),
)
const syntaxOptions = computed(() =>
  (store.schema?.syntaxes ?? [])
    .map((s) => ({ label: s.desc ? `${s.desc} (${s.oid})` : s.oid, value: s.oid }))
    .sort((a, b) => a.label.localeCompare(b.label)),
)
const rules = computed(() => ruleOptions(store.schema?.matchingRules ?? []))

const complete = computed(() => !!def.value.oid.trim() && def.value.names.length > 0 && (!!def.value.syntax || !!def.value.sup))

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
      await call(api.PUT('/api/v1/schema/attributetypes/{name}', { params: { path: { name: props.edit.oid } }, body }))
    } else {
      await call(api.POST('/api/v1/schema/attributetypes', { body }))
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
    :title="edit ? t('schemaView.editAttr') : t('schemaView.newAttr')"
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
      <NFormItem :label="t('schemaView.syntax')">
        <NSelect v-model:value="def.syntax" :options="syntaxOptions" filterable clearable />
      </NFormItem>
      <NFormItem :label="t('schemaView.sup')" :feedback="t('schemaView.supHint')">
        <NSelect v-model:value="def.sup" :options="attrOptions" filterable clearable />
      </NFormItem>
      <div class="rules">
        <NFormItem :label="t('schemaView.equality')">
          <NSelect v-model:value="def.equality" :options="rules.equality" filterable clearable />
        </NFormItem>
        <NFormItem :label="t('schemaView.ordering')">
          <NSelect v-model:value="def.ordering" :options="rules.ordering" filterable clearable />
        </NFormItem>
        <NFormItem :label="t('schemaView.substr')">
          <NSelect v-model:value="def.substr" :options="rules.substr" filterable clearable />
        </NFormItem>
      </div>
      <NCheckbox v-model:checked="def.singleValue">{{ t('schemaView.singleValue') }}</NCheckbox>
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

.rules {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0 12px;
}

@media (max-width: 600px) {
  .rules {
    grid-template-columns: 1fr;
  }
}

.top {
  margin-top: 12px;
}
</style>
