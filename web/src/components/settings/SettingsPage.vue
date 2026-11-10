<script setup lang="ts">
/** Renders and saves one declarative settings page (GET/PATCH /pages/{id}). */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NSkeleton, NSpace, useDialog, useMessage, useThemeVars } from 'naive-ui'
import { api, call, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useServerStore } from '@/stores/server'
import { changedAttrs, fieldKey, type PageField } from '@/settings/fields'
import SettingsField from './SettingsField.vue'

type PageState = Schemas['PageState']

const props = defineProps<{ id: string }>()
const dirty = defineModel<boolean>('dirty', { default: false })

const { t, te } = useI18n()
const vars = useThemeVars()
const message = useMessage()
const dialog = useDialog()
const server = useServerStore()

const state = ref<PageState>()
const form = ref<Record<string, string[]>>({})
const loadError = ref<LocalizedError>()
const saving = ref(false)

function reset() {
  const s = state.value
  if (!s) return
  form.value = Object.fromEntries(s.page.fields.map((f) => [f.attr, [...(s.values[f.attr] ?? [])]]))
}

async function load() {
  state.value = undefined
  loadError.value = undefined
  try {
    state.value = await call(api.GET('/api/v1/pages/{id}', { params: { path: { id: props.id } } }))
    reset()
  } catch (e) {
    loadError.value = localizeError(e)
  }
}
watch(() => props.id, load, { immediate: true })

const changed = computed(() => (state.value ? changedAttrs(form.value, state.value.values) : []))
watch(changed, (c) => (dirty.value = c.length > 0), { immediate: true })

/** Fields grouped under their headings, in definition order. */
const groups = computed(() => {
  const out: { name: string; fields: PageField[] }[] = []
  for (const f of state.value?.page.fields ?? []) {
    const name = f.group ?? ''
    let g = out.find((x) => x.name === name)
    if (!g) out.push((g = { name, fields: [] }))
    g.fields.push(f)
  }
  return out
})

const title = computed(() => t(`pages.${props.id}.title`))
const description = computed(() => (te(`pages.${props.id}.description`) ? t(`pages.${props.id}.description`) : ''))

function confirmDanger(fields: PageField[]): Promise<boolean> {
  return new Promise((resolve) => {
    dialog.warning({
      title: t('pages._dangerTitle'),
      content: t('pages._dangerConfirm', { fields: fields.map((f) => t(`${fieldKey(props.id, f)}.label`)).join(', ') }),
      positiveText: t('pages._saveAnyway'),
      negativeText: t('edit.cancel'),
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false),
    })
  })
}

async function save() {
  const s = state.value
  if (!s || !changed.value.length) return
  const danger = s.page.fields.filter((f) => f.danger && changed.value.includes(f.attr))
  if (danger.length && !(await confirmDanger(danger))) return
  saving.value = true
  try {
    const values = Object.fromEntries(changed.value.map((a) => [a, form.value[a] ?? []]))
    const res = await call(api.PATCH('/api/v1/pages/{id}', { params: { path: { id: props.id } }, body: { values } }))
    state.value = res.state
    reset()
    if (res.restart.length) {
      message.warning(t('pages._savedRestart', { n: res.restart.length }, res.restart.length), { duration: 6000 })
      void server.refreshState()
    } else {
      message.success(t('pages._saved'))
    }
  } catch (e) {
    const err = localizeError(e)
    message.error(`${t('pages._saveError')}: ${err.summary}${err.detail ? ` — ${err.detail}` : ''}`, { duration: 8000 })
  } finally {
    saving.value = false
  }
}

defineExpose({ save, reset })
</script>

<template>
  <NCard size="small" class="page">
    <template #header>{{ title }}</template>
    <template #header-extra>
      <code class="dn">{{ state?.page.dn }}</code>
    </template>

    <NAlert v-if="loadError" type="error" :title="t('pages._loadError')">
      {{ loadError.summary }}
      <div v-if="loadError.detail" class="mono">{{ loadError.detail }}</div>
    </NAlert>
    <NSkeleton v-else-if="!state" text :repeat="6" />

    <template v-else>
      <p v-if="description" class="description">{{ description }}</p>
      <form @submit.prevent="save">
        <section v-for="g in groups" :key="g.name" class="group">
          <h3 v-if="g.name" class="group-title">{{ t(`pages._groups.${g.name}`) }}</h3>
          <SettingsField
            v-for="f in g.fields"
            :key="f.attr"
            v-model="form[f.attr]!"
            :page-id="id"
            :field="f"
            :options="state.options?.[f.attr]"
            :changed="changed.includes(f.attr)"
          />
        </section>
      </form>
    </template>

    <template v-if="state" #action>
      <div class="actions">
        <span class="status">
          {{ changed.length ? t('pages._unsaved', { n: changed.length }, changed.length) : t('pages._noChanges') }}
        </span>
        <NSpace>
          <NButton :disabled="!changed.length || saving" @click="reset">{{ t('pages._reset') }}</NButton>
          <NButton type="primary" :disabled="!changed.length" :loading="saving" @click="save">{{ t('pages._save') }}</NButton>
        </NSpace>
      </div>
    </template>
  </NCard>
</template>

<style scoped>
/* Long pages: keep Save in reach. */
.page :deep(.n-card__action) {
  position: sticky;
  bottom: 0;
  z-index: 2;
  border-radius: 0 0 var(--n-border-radius) var(--n-border-radius);
}

.dn {
  font-size: 12px;
  color: v-bind('vars.textColor3');
  overflow-wrap: anywhere;
}

.description {
  margin: 0 0 8px;
  max-width: 820px;
  color: v-bind('vars.textColor2');
}

.group + .group {
  margin-top: 20px;
}

.group-title {
  margin: 0 0 4px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  color: v-bind('vars.textColor3');
}

.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.status {
  font-size: 13px;
  color: v-bind('vars.textColor3');
}
</style>
