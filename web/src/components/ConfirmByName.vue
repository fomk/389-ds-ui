<script setup lang="ts">
/**
 * Confirmation for destructive operations: the user types an exact name
 * (e.g. the backend) before the action button is enabled.
 */
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NInput, NModal, NSpace } from 'naive-ui'

const props = defineProps<{ title: string; warning: string; name: string; action: string; busy?: boolean }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ confirm: [] }>()

const { t } = useI18n()
const typed = ref('')
watch(show, (v) => v && (typed.value = ''))

function confirm() {
  if (typed.value === props.name) emit('confirm')
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="title" :style="{ width: 'min(480px, 92vw)' }" :mask-closable="!busy">
    <NAlert type="error" :show-icon="true" class="warning">{{ warning }}</NAlert>
    <form @submit.prevent="confirm">
      <label class="label" for="confirm-name">
        {{ t('confirm.typeName') }} <code class="name">{{ name }}</code>
      </label>
      <NInput
        v-model:value="typed"
        :placeholder="name"
        :input-props="{ id: 'confirm-name', class: 'mono', autocomplete: 'off', spellcheck: false }"
      />
    </form>
    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="busy" @click="show = false">{{ t('edit.cancel') }}</NButton>
        <NButton type="error" :disabled="typed !== name" :loading="busy" @click="confirm">{{ action }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.warning {
  margin-bottom: 16px;
}

.label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
}

.name {
  font-weight: 600;
}
</style>
