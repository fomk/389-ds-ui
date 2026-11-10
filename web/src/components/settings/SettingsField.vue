<script setup lang="ts">
/** One attribute of a declarative settings page: label column + control. */
import { computed, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NIcon,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NTag,
  NTooltip,
  useThemeVars,
  type SelectOption,
} from 'naive-ui'
import { RotateCw, TriangleAlert } from '@lucide/vue'
import {
  fieldKey,
  flagsSet,
  flagsValue,
  pickUnit,
  unitsFor,
  unknownBits,
  type PageField,
  type Unit,
} from '@/settings/fields'

const props = defineProps<{ pageId: string; field: PageField; options?: string[]; changed?: boolean; showAttr?: boolean }>()
const model = defineModel<string[]>({ required: true })

const { t, te } = useI18n()
const vars = useThemeVars()

const key = computed(() => fieldKey(props.pageId, props.field))
const help = computed(() => (te(`${key.value}.help`) ? t(`${key.value}.help`) : ''))
const id = computed(() => `f-${props.field.attr}`)
const single = computed(() => model.value[0] ?? '')
const disabled = computed(() => !!props.field.readOnly)

function set(v: string | null | undefined) {
  model.value = v === null || v === undefined || v === '' ? [] : [v]
}

// --- numbers (int, size, duration, flags) ----------------------------------

const num = computed(() => {
  const n = Number(single.value)
  return single.value.trim() === '' || !Number.isFinite(n) ? null : n
})

const units = computed(() => unitsFor(props.field.type))
const unit = ref<Unit>()
let emitted: number | null | undefined
// Choose the unit from the server's value, but not while the user types.
watch(
  [num, units],
  ([n, us]) => {
    if (!us) return
    if (n !== emitted || !unit.value) unit.value = pickUnit(n ?? 0, us)
  },
  { immediate: true },
)

const scaled = computed(() => {
  const n = num.value
  if (n === null) return null
  const f = unit.value?.factor ?? 1
  return n > 0 ? n / f : n
})

function setScaled(v: number | null) {
  const f = unit.value?.factor ?? 1
  emitted = v === null ? null : v > 0 ? Math.round(v * f) : Math.round(v)
  set(emitted === null ? null : String(emitted))
}

function setUnit(k: string) {
  const current = scaled.value
  unit.value = units.value?.find((u) => u.key === k)
  if (current !== null) setScaled(current)
}

const unitOptions = computed(() => units.value?.map((u) => ({ value: u.key, label: t(`pages._units.${u.key}`) })) ?? [])

// Bounds are in the stored unit and must be scaled like the value. Zero and
// negative bounds stay as they are: special values (-1, 0) are never scaled.
const minScaled = computed(() => {
  const m = props.field.min
  return m === undefined || m <= 0 ? m : m / (unit.value?.factor ?? 1)
})
const maxScaled = computed(() => (props.field.max === undefined ? undefined : props.field.max / (unit.value?.factor ?? 1)))

// --- flags -----------------------------------------------------------------

const flags = computed(() => props.field.flags ?? [])
const flagNames = computed(() => flagsSet(num.value ?? 0, flags.value))
const flagOptions = computed(() =>
  flags.value.map((f) => ({ value: f.name, label: t(`pages.${props.pageId}.flags.${f.name}`), bit: f.value })),
)
/** Option label with its bit value (the menu is teleported: inline style). */
function renderFlag(o: SelectOption & { bit?: number }) {
  return [String(o.label), h('span', { style: { marginLeft: '6px', fontSize: '11px', opacity: 0.55, fontVariantNumeric: 'tabular-nums' } }, String(o.bit ?? ''))]
}
function setFlags(names: (string | number)[]) {
  const keep = unknownBits(num.value ?? 0, flags.value)
  set(String(flagsValue(names.map(String), flags.value, keep)))
}

// --- bool / enum / list ----------------------------------------------------

const boolLiterals = computed(() => props.field.bool ?? ['on', 'off'])
const checked = computed(() => single.value.toLowerCase() === boolLiterals.value[0]!.toLowerCase())

const readOnlyText = computed(() => {
  const v = single.value
  if (!v) return '—'
  return te(`${key.value}.values.${v}`) ? t(`${key.value}.values.${v}`) : v
})

const enumOptions = computed(() => {
  const vals = [...(props.field.values ?? []), ...(props.options ?? [])]
  if (single.value && !vals.some((v) => v.toLowerCase() === single.value.toLowerCase())) vals.push(single.value)
  return vals.map((v) => ({ value: v, label: te(`${key.value}.values.${v}`) ? t(`${key.value}.values.${v}`) : v }))
})
</script>

<template>
  <div class="row">
    <div class="label">
      <label :for="id" class="name" :title="showAttr ? undefined : field.attr">
        {{ t(`${key}.label`) }}
        <NTooltip v-if="field.restart">
          <template #trigger>
            <NTag size="tiny" :bordered="false" type="info" class="tag">
              <template #icon><NIcon><RotateCw /></NIcon></template>
              {{ t('pages._restart') }}
            </NTag>
          </template>
          {{ t('pages._restartHint') }}
        </NTooltip>
        <NTooltip v-if="field.danger">
          <template #trigger>
            <NIcon class="danger" :size="15" :aria-label="t('pages._danger')"><TriangleAlert /></NIcon>
          </template>
          {{ t('pages._dangerHint') }}
        </NTooltip>
        <span v-if="changed" class="dot" :title="t('pages._changed')" />
      </label>
      <code v-if="showAttr" class="attr">{{ field.attr }}</code>
      <div v-if="help" class="help">{{ help }}</div>
    </div>

    <div class="control">
      <span v-if="field.readOnly" :id="id" class="ro">{{ readOnlyText }}</span>

      <NSwitch
        v-else-if="field.type === 'bool'"
        :value="checked"
        :disabled="disabled"
        @update:value="(v: boolean) => set(v ? boolLiterals[0] : boolLiterals[1])"
      />

      <NSelect
        v-else-if="field.type === 'enum'"
        :value="single || null"
        :options="enumOptions"
        :disabled="disabled"
        :input-props="{ id }"
        filterable
        class="w-select"
        @update:value="set"
      />

      <div v-else-if="field.type === 'flags'" class="flags">
        <NSelect
          :value="flagNames"
          :options="flagOptions"
          :render-label="renderFlag"
          :disabled="disabled"
          multiple
          filterable
          class="w-flags"
          @update:value="setFlags"
        />
        <NInputNumber
          :value="num"
          :min="0"
          :show-button="false"
          :disabled="disabled"
          :input-props="{ id, 'aria-label': t('pages._flagsTotal') }"
          :title="t('pages._flagsTotal')"
          class="w-flag"
          @update:value="(v: number | null) => set(v === null ? null : String(v))"
        />
      </div>

      <div v-else-if="field.type === 'int' || units" class="number">
        <NInputNumber
          :value="units ? scaled : num"
          :min="units ? minScaled : field.min"
          :max="units ? maxScaled : field.max"
          :precision="units ? undefined : 0"
          :disabled="disabled"
          :input-props="{ id }"
          :placeholder="t('pages._default')"
          class="w-num"
          @update:value="units ? setScaled($event) : set($event === null ? null : String($event))"
        >
          <template v-if="field.unit" #suffix>{{ t(`pages._units.${field.unit}`) }}</template>
        </NInputNumber>
        <NSelect
          v-if="units"
          :value="unit?.key"
          :options="unitOptions"
          :disabled="disabled"
          class="w-unit"
          @update:value="setUnit"
        />
        <span v-if="units && num !== null && unit && unit.factor > 1" class="raw">= {{ num }}</span>
      </div>

      <NSelect
        v-else-if="field.type === 'list'"
        v-model:value="model"
        multiple
        filterable
        tag
        :show-arrow="false"
        :show="false"
        :disabled="disabled"
        :input-props="{ id }"
        class="w-text"
      />

      <NInput
        v-else
        :value="single"
        :disabled="disabled"
        :input-props="{ id, class: field.type === 'dn' ? 'mono' : undefined, spellcheck: false }"
        :placeholder="t('pages._default')"
        clearable
        class="w-text"
        @update:value="set"
      />
    </div>
  </div>
</template>

<style scoped>
.row {
  display: grid;
  grid-template-columns: minmax(200px, 340px) minmax(0, 1fr);
  gap: 4px 24px;
  padding: 12px 0;
  border-top: 1px solid v-bind('vars.dividerColor');
}

@media (max-width: 760px) {
  .row {
    grid-template-columns: 1fr;
  }
}

.name {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-weight: 500;
}

.tag {
  cursor: default;
}

.danger {
  color: v-bind('vars.warningColor');
}

.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: v-bind('vars.primaryColor');
}

.attr {
  display: block;
  font-size: 11.5px;
  color: v-bind('vars.textColor3');
  overflow-wrap: anywhere;
}

.help {
  margin-top: 4px;
  font-size: 12.5px;
  line-height: 1.45;
  color: v-bind('vars.textColor3');
}

.control {
  display: flex;
  align-items: flex-start;
  min-width: 0;
}

.number {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.w-num {
  width: 180px;
}

.w-unit {
  width: 96px;
}

.w-select {
  max-width: 280px;
}

.w-text {
  max-width: 480px;
}

.raw {
  font-size: 12px;
  color: v-bind('vars.textColor3');
  font-variant-numeric: tabular-nums;
}

.flags {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.w-flags {
  flex: 1;
  min-width: 0;
  max-width: 520px;
}

.w-flag {
  width: 110px;
  flex: none;
}

.ro {
  line-height: 34px;
  font-weight: 500;
}

:deep(.bit),
.bit {
  margin-left: 4px;
  font-size: 11px;
  color: v-bind('vars.textColor3');
  font-variant-numeric: tabular-nums;
}
</style>
