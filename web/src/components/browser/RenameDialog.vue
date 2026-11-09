<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCheckbox, NForm, NFormItem, NInput, NModal, NSpace, useMessage } from 'naive-ui'
import { api, call, type Entry } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const props = defineProps<{ entry: Entry }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ renamed: [oldDN: string, newDN: string] }>()

const { t } = useI18n()
const message = useMessage()
const newRdn = ref('')
const newParent = ref('')
const deleteOldRdn = ref(true)
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (v) {
    newRdn.value = props.entry.rdn
    newParent.value = props.entry.parent
    deleteOldRdn.value = true
    error.value = undefined
  }
})

async function run() {
  busy.value = true
  error.value = undefined
  try {
    const res = await call(
      api.POST('/api/v1/entries/rename', {
        body: {
          dn: props.entry.dn,
          newRdn: newRdn.value.trim(),
          deleteOldRdn: deleteOldRdn.value,
          newParent: newParent.value.trim() || undefined,
        },
      }),
    )
    message.success(t('rename.done', { dn: res.dn }))
    show.value = false
    emit('renamed', props.entry.dn, res.dn)
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="t('rename.title')" :style="{ width: 'min(620px, 92vw)' }" :mask-closable="!busy">
    <NForm label-placement="top" @submit.prevent="run">
      <NFormItem :label="t('rename.newRdn')">
        <NInput v-model:value="newRdn" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NFormItem :label="t('rename.newParent')">
        <NInput v-model:value="newParent" :input-props="{ class: 'mono', spellcheck: false }" />
      </NFormItem>
      <NCheckbox v-model:checked="deleteOldRdn">{{ t('rename.deleteOldRdn') }}</NCheckbox>
    </NForm>
    <NAlert v-if="error" type="error" :title="t('rename.error')" class="top">
      {{ error.summary }}
      <div v-if="error.detail" class="mono">{{ error.detail }}</div>
    </NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="primary" :loading="busy" :disabled="!newRdn.trim()" @click="run">{{ t('rename.apply') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.top {
  margin-top: 12px;
}
</style>
