<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCheckbox, NModal, NSpace, useMessage } from 'naive-ui'
import { api, call, type Entry } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'

const props = defineProps<{ entry: Entry }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ deleted: [dn: string] }>()

const { t } = useI18n()
const message = useMessage()
const subtree = ref(false)
const busy = ref(false)
const error = ref<LocalizedError>()

watch(show, (v) => {
  if (v) {
    subtree.value = false
    error.value = undefined
  }
})

async function run() {
  busy.value = true
  error.value = undefined
  try {
    const res = await call(
      api.DELETE('/api/v1/entries', { params: { query: { dn: props.entry.dn, subtree: subtree.value } } }),
    )
    message.success(t('delete.done', { n: res.deleted }, res.deleted))
    show.value = false
    emit('deleted', props.entry.dn)
  } catch (e) {
    error.value = localizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="t('delete.title')" :style="{ width: 'min(560px, 92vw)' }" :mask-closable="!busy">
    <p class="mono dn">{{ t('delete.confirm', { dn: entry.dn }) }}</p>
    <template v-if="entry.hasChildren">
      <NAlert type="warning" class="block">{{ t('delete.hasChildren') }}</NAlert>
      <NCheckbox v-model:checked="subtree">
        {{ t('delete.subtree', { n: entry.numSubordinates }, entry.numSubordinates) }}
      </NCheckbox>
    </template>
    <NAlert v-if="error" type="error" :title="t('delete.error')" class="block top">
      {{ error.summary }}
      <div v-if="error.detail" class="mono">{{ error.detail }}</div>
    </NAlert>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="error" :loading="busy" @click="run">{{ t('actions.delete') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.dn {
  overflow-wrap: anywhere;
}

.block {
  margin-bottom: 12px;
}

.top {
  margin-top: 12px;
}
</style>
