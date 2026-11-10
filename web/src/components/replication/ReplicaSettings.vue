<script setup lang="ts">
/** Role, replica ID, bind DNs of an enabled replica; manager password; disable. */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NCard, NDynamicTags, NForm, NFormItem, NInput, NInputNumber, NRadioButton, NRadioGroup, NSpace, useMessage, useThemeVars } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import ConfirmByName from '@/components/ConfirmByName.vue'
import { localizeError } from '@/i18n/errors'

const props = defineProps<{ replica: Schemas['Replica'] }>()
const emit = defineEmits<{ changed: []; disabled: [] }>()
const { t } = useI18n()
const vars = useThemeVars()
const message = useMessage()

const form = ref({ role: 'supplier', replicaId: 1 as number | null, bindDNs: [] as string[], bindGroup: '', purgeDelay: null as number | null })
function reset() {
  const r = props.replica
  form.value = { role: r.role ?? 'supplier', replicaId: r.replicaId ?? null, bindDNs: [...r.bindDNs], bindGroup: r.bindGroup ?? '', purgeDelay: r.purgeDelay ?? null }
}
watch(() => props.replica, reset, { immediate: true })

const dirty = computed(() => {
  const r = props.replica
  const f = form.value
  return (
    f.role !== r.role ||
    (f.role === 'supplier' && f.replicaId !== r.replicaId) ||
    f.bindDNs.join('\n') !== r.bindDNs.join('\n') ||
    f.bindGroup !== (r.bindGroup ?? '') ||
    (f.purgeDelay ?? 0) !== (r.purgeDelay ?? 0)
  )
})
const saving = ref(false)
async function save() {
  saving.value = true
  try {
    const f = form.value
    const body: Schemas['ReplicaPatch'] = { bindDNs: f.bindDNs, bindGroup: f.bindGroup }
    if (f.role !== props.replica.role || f.replicaId !== props.replica.replicaId) {
      body.role = f.role as Schemas['ReplicaPatch']['role']
      if (f.role === 'supplier' && f.replicaId !== null) body.replicaId = f.replicaId
    }
    if (f.purgeDelay !== null) body.purgeDelay = f.purgeDelay
    await call(api.PATCH('/api/v1/replication', { params: { query: { suffix: props.replica.suffix } }, body }))
    message.success(t('replication.saved'))
    emit('changed')
  } catch (e) {
    message.error(`${t('replication.error')}: ${localizeError(e).summary}`, { duration: 8000 })
  } finally {
    saving.value = false
  }
}

const managerDN = ref('cn=replication manager,cn=config')
const managerPw = ref('')
async function setManager() {
  try {
    await call(api.POST('/api/v1/replication/manager', { body: { dn: managerDN.value.trim(), password: managerPw.value } }))
    managerPw.value = ''
    message.success(t('replication.managerSaved'))
  } catch (e) {
    message.error(`${t('replication.error')}: ${localizeError(e).summary}`)
  }
}

const confirming = ref(false)
const disabling = ref(false)
async function disable() {
  disabling.value = true
  try {
    await call(api.DELETE('/api/v1/replication', { params: { query: { suffix: props.replica.suffix } } }))
    message.success(t('replication.disabledMsg'))
    confirming.value = false
    emit('disabled')
  } catch (e) {
    message.error(`${t('replication.error')}: ${localizeError(e).summary}`)
  } finally {
    disabling.value = false
  }
}
</script>

<template>
  <NCard size="small">
    <NForm label-placement="top" class="form" @submit.prevent="save">
      <NFormItem :label="t('replication.role')" :feedback="t(`replication.roleHints.${form.role}`)">
        <NRadioGroup v-model:value="form.role">
          <NRadioButton v-for="r in ['supplier', 'hub', 'consumer']" :key="r" :value="r">{{ t(`replication.roles.${r}`) }}</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem v-if="form.role === 'supplier'" :label="t('replication.replicaId')" :feedback="t('replication.replicaIdHint')">
        <NInputNumber v-model:value="form.replicaId" :min="1" :max="65534" class="num" />
      </NFormItem>
      <NFormItem :label="t('replication.bindDNs')" :feedback="t('replication.bindDNHint')">
        <NDynamicTags v-model:value="form.bindDNs" />
      </NFormItem>
      <NFormItem :label="t('replication.bindGroup')">
        <NInput v-model:value="form.bindGroup" :input-props="{ class: 'mono', spellcheck: false }" :placeholder="t('pages._default')" />
      </NFormItem>
      <NFormItem :label="t('replication.purgeDelay')" :feedback="t('replication.purgeDelayHint')">
        <NInputNumber v-model:value="form.purgeDelay" :min="-1" class="num" :placeholder="t('pages._default')" />
      </NFormItem>
      <NSpace>
        <NButton type="primary" attr-type="submit" :disabled="!dirty" :loading="saving">{{ t('pages._save') }}</NButton>
        <NButton :disabled="!dirty" @click="reset">{{ t('pages._reset') }}</NButton>
      </NSpace>
    </NForm>
  </NCard>

  <NCard size="small" :title="t('replication.manager')" class="top">
    <p class="hint">{{ t('replication.managerHint') }}</p>
    <form class="manager" @submit.prevent="setManager">
      <NInput v-model:value="managerDN" :input-props="{ class: 'mono', spellcheck: false }" />
      <NInput v-model:value="managerPw" type="password" show-password-on="click" :placeholder="t('replication.managerPassword')" :input-props="{ autocomplete: 'new-password' }" />
      <NButton attr-type="submit" :disabled="managerPw.length < 8">{{ t('replication.setPassword') }}</NButton>
    </form>
  </NCard>

  <div class="danger">
    <div>
      <div class="danger-title">{{ t('replication.disable') }}</div>
      <div class="hint">{{ t('replication.disableShort') }}</div>
    </div>
    <NButton type="error" ghost @click="confirming = true">{{ t('replication.disable') }}</NButton>
  </div>
  <ConfirmByName
    v-model:show="confirming"
    :title="t('replication.disable')"
    :warning="t('replication.disableWarning', { suffix: replica.suffix })"
    :name="replica.suffix"
    :action="t('replication.disable')"
    :busy="disabling"
    @confirm="disable"
  />
</template>

<style scoped>
.form {
  max-width: 620px;
}

.num {
  width: 180px;
}

.top {
  margin-top: 16px;
}

.hint {
  margin: 0 0 10px;
  font-size: 13px;
  color: v-bind('vars.textColor3');
}

.manager {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) auto;
  gap: 8px;
}

@media (max-width: 760px) {
  .manager {
    grid-template-columns: 1fr;
  }
}

.danger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-top: 20px;
  padding: 12px 16px;
  border: 1px solid v-bind('vars.errorColor');
  border-radius: 6px;
}

.danger-title {
  font-weight: 600;
}
</style>
