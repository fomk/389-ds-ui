<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NCard, NIcon, NSpin, NTag, useThemeVars } from 'naive-ui'
import { Users } from '@lucide/vue'
import { api, call } from '@/api/client'
import { ancestorsUnder } from '@/ldap/dn'
import { escapeFilterValue } from '@/ldap/filter'
import { useServerStore } from '@/stores/server'

/**
 * Groups that reference dn. Found by searching member/uniqueMember, so it
 * works without the memberOf plugin and also shows nested groups' parents.
 */
const props = defineProps<{ dn: string; version: string }>()

const { t } = useI18n()
const vars = useThemeVars()
const server = useServerStore()
const groups = ref<string[]>()

const suffix = computed(() => server.info?.server.namingContexts.find((nc) => ancestorsUnder(props.dn, nc) !== null))

async function load() {
  if (!suffix.value) {
    groups.value = []
    return
  }
  const v = escapeFilterValue(props.dn)
  try {
    const res = await call(
      api.POST('/api/v1/search', {
        body: { base: suffix.value, scope: 'sub', filter: `(|(member=${v})(uniqueMember=${v}))`, attrs: ['1.1'], limit: 500 },
      }),
    )
    groups.value = res.entries.map((e) => e.dn)
  } catch {
    groups.value = []
  }
}

watch(() => [props.dn, props.version, suffix.value], load, { immediate: true })
</script>

<template>
  <NCard size="small" class="card" :title="t('groups.memberOf')">
    <template #header-extra>
      <span class="hint">{{ t('groups.memberOfHint') }}</span>
    </template>
    <NSpin v-if="!groups" size="small" />
    <div v-else-if="groups.length" class="list">
      <RouterLink v-for="g in groups" :key="g" :to="{ path: '/browser', query: { dn: g } }">
        <NTag size="small" :bordered="false" class="tag">
          <template #icon><NIcon><Users /></NIcon></template>
          <span class="mono">{{ g }}</span>
        </NTag>
      </RouterLink>
    </div>
    <span v-else class="hint">{{ t('groups.memberOfNone') }}</span>
  </NCard>
</template>

<style scoped>
.card {
  margin-top: 12px;
}

.list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.tag {
  cursor: pointer;
}

.hint {
  font-size: 12px;
  color: v-bind('vars.textColor3');
}
</style>
