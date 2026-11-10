<script setup lang="ts">
/** Roles and Class of Service. */
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NH1, NRadioButton, NRadioGroup } from 'naive-ui'
import CosPanel from '@/components/roles/CosPanel.vue'
import RolesPanel from '@/components/roles/RolesPanel.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const section = computed({
  get: () => (route.query.section === 'cos' ? 'cos' : 'roles'),
  set: (v: string) => router.replace({ path: '/roles', query: v === 'roles' ? {} : { section: v } }),
})
const selected = computed(() => (typeof route.query.dn === 'string' ? route.query.dn : undefined))
function select(dn: string | undefined) {
  router.replace({ path: '/roles', query: { ...(section.value === 'cos' ? { section: 'cos' } : {}), ...(dn ? { dn } : {}) } })
}
</script>

<template>
  <NH1 class="title">{{ t('nav.roles') }}</NH1>
  <NRadioGroup v-model:value="section" class="sections">
    <NRadioButton value="roles">{{ t('roles.tab') }}</NRadioButton>
    <NRadioButton value="cos">{{ t('cos.tab') }}</NRadioButton>
  </NRadioGroup>
  <RolesPanel v-if="section === 'roles'" :selected="selected" @select="select" />
  <CosPanel v-else :selected="selected" @select="select" />
</template>

<style scoped>
.title {
  margin: 0 0 20px;
}

.sections {
  margin-bottom: 16px;
}
</style>
