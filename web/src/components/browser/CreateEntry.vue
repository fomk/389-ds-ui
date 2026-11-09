<script setup lang="ts">
import { computed, onMounted, ref, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NGrid, NGridItem, NIcon, NSkeleton, useThemeVars } from 'naive-ui'
import { ArrowLeft, FileText, Folder, User, Users } from '@lucide/vue'
import { api, call, type Entry, type Schemas } from '@/api/client'
import { localizeError, type LocalizedError } from '@/i18n/errors'
import { useSchemaStore } from '@/stores/schema'
import EntryEditor, { type CreateSpec } from './EntryEditor.vue'

type Template = Schemas['Template']

const props = defineProps<{ parent: string }>()
const emit = defineEmits<{ created: [entry: Entry]; cancel: [] }>()

const { t, te } = useI18n()
const vars = useThemeVars()
const schema = useSchemaStore()

const templates = ref<Template[]>([])
const chosen = ref<Template>()
const error = ref<LocalizedError>()

onMounted(async () => {
  try {
    const [list] = await Promise.all([call(api.GET('/api/v1/templates')), schema.load()])
    templates.value = list
  } catch (e) {
    error.value = localizeError(e)
  }
})

const icons: Record<string, Component> = { user: User, group: Users, folder: Folder, entry: FileText }

// Template texts come from the locale files (templates.<id>.*); fall back to the id.
const title = (tpl: Template) => (te(`templates.${tpl.id}.title`) ? t(`templates.${tpl.id}.title`) : tpl.id)
const description = (tpl: Template) =>
  te(`templates.${tpl.id}.description`) ? t(`templates.${tpl.id}.description`) : tpl.objectClasses.join(', ')

const spec = computed<CreateSpec | undefined>(() =>
  chosen.value
    ? {
        parent: props.parent,
        objectClasses: chosen.value.objectClasses,
        rdn: chosen.value.rdn,
        custom: !!chosen.value.custom,
        password: !!chosen.value.password,
        fields: chosen.value.fields,
      }
    : undefined,
)
</script>

<template>
  <NCard size="small">
    <template #header>
      <div class="head">
        <NButton v-if="chosen" size="small" quaternary :title="t('create.back')" @click="chosen = undefined">
          <template #icon><NIcon><ArrowLeft /></NIcon></template>
        </NButton>
        <span>{{ chosen ? title(chosen) : t('create.chooseTemplate') }}</span>
      </div>
      <div class="parent mono">{{ t('create.title', { parent }) }}</div>
    </template>

    <NAlert v-if="error" type="error">{{ error.summary }}</NAlert>

    <EntryEditor v-else-if="spec" :create="spec" @created="(e) => emit('created', e)" @cancel="emit('cancel')" />

    <NGrid v-else-if="templates.length" cols="1 s:2 l:3" responsive="screen" :x-gap="12" :y-gap="12">
      <NGridItem v-for="tpl in templates" :key="tpl.id">
        <NCard hoverable size="small" class="tpl" role="button" tabindex="0" @click="chosen = tpl" @keydown.enter="chosen = tpl">
          <div class="tpl-title">
            <NIcon :size="20" :color="vars.primaryColor"><component :is="icons[tpl.icon] ?? FileText" /></NIcon>
            {{ title(tpl) }}
          </div>
          <div class="tpl-desc">{{ description(tpl) }}</div>
        </NCard>
      </NGridItem>
    </NGrid>

    <NSkeleton v-else text :repeat="4" />
  </NCard>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 17px;
  font-weight: 600;
}

.parent {
  margin-top: 2px;
  font-size: 12px;
  font-weight: normal;
  color: v-bind('vars.textColor3');
  overflow-wrap: anywhere;
}

.tpl {
  cursor: pointer;
  height: 100%;
}

.tpl-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  margin-bottom: 6px;
}

.tpl-desc {
  font-size: 13px;
  color: v-bind('vars.textColor3');
}
</style>
