<script setup lang="ts">
/** Enable replication for a suffix, optionally creating the replication manager. */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NCheckbox, NForm, NFormItem, NInput, NInputNumber, NRadioButton, NRadioGroup, useMessage } from 'naive-ui'
import { api, call } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const props = defineProps<{ suffix: string }>()
const emit = defineEmits<{ enabled: [] }>()
const { t } = useI18n()
const message = useMessage()

const MANAGER = 'cn=replication manager,cn=config'
const role = ref<'supplier' | 'hub' | 'consumer'>('supplier')
const replicaId = ref<number | null>(1)
const bindDN = ref(MANAGER)
const createManager = ref(true)
const password = ref('')
const busy = ref(false)
const error = ref<LocalizedError>()

const valid = computed(
  () =>
    (role.value !== 'supplier' || (replicaId.value !== null && replicaId.value >= 1 && replicaId.value <= 65534)) &&
    (!createManager.value || password.value.length >= 8),
)

async function enable() {
  busy.value = true
  error.value = undefined
  try {
    if (createManager.value) {
      await call(api.POST('/api/v1/replication/manager', { body: { dn: bindDN.value.trim(), password: password.value } }))
    }
    await call(
      api.POST('/api/v1/replication', {
        params: { query: { suffix: props.suffix } },
        body: { role: role.value, replicaId: role.value === 'supplier' ? (replicaId.value ?? 0) : 0, bindDNs: bindDN.value.trim() ? [bindDN.value.trim()] : [] },
      }),
    )
    message.success(t('replication.enabledMsg', { suffix: props.suffix }))
    emit('enabled')
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NCard size="small" :title="t('replication.disabledTitle')">
    <p class="hint">{{ t('replication.disabledHint') }}</p>
    <NForm label-placement="top" class="form" @submit.prevent="enable">
      <NFormItem :label="t('replication.role')" :feedback="t(`replication.roleHints.${role}`)">
        <NRadioGroup v-model:value="role">
          <NRadioButton v-for="r in ['supplier', 'hub', 'consumer']" :key="r" :value="r">{{ t(`replication.roles.${r}`) }}</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem v-if="role === 'supplier'" :label="t('replication.replicaId')" :feedback="t('replication.replicaIdHint')">
        <NInputNumber v-model:value="replicaId" :min="1" :max="65534" class="num" />
      </NFormItem>
      <NFormItem :label="t('replication.bindDN')" :feedback="t('replication.bindDNHint')">
        <NInput v-model:value="bindDN" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NCheckbox v-model:checked="createManager">{{ t('replication.createManager') }}</NCheckbox>
      <NFormItem v-if="createManager" :label="t('replication.managerPassword')" class="top">
        <NInput v-model:value="password" type="password" show-password-on="click" :input-props="{ autocomplete: 'new-password' }" />
      </NFormItem>
      <NButton type="primary" attr-type="submit" :disabled="!valid" :loading="busy" class="top">{{ t('replication.enable') }}</NButton>
    </NForm>
    <NAlert v-if="error" type="error" class="top">{{ error.summary }}<div v-if="error.detail" class="mono">{{ error.detail }}</div></NAlert>
  </NCard>
</template>

<style scoped>
.hint {
  margin: 0 0 14px;
}

.form {
  max-width: 560px;
}

.num {
  width: 180px;
}

.top {
  margin-top: 12px;
}
</style>
