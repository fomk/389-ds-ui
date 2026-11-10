<script setup lang="ts">
/**
 * Single-series time chart (one per metric: small multiples, never a second
 * y-axis). 2px line, hairline grid, crosshair + tooltip on hover. The title
 * outside names the series, so no legend. Colour: validated slot 1 of the
 * reference palette, separate light/dark steps.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useOsTheme, useThemeVars } from 'naive-ui'
import { currentLocale } from '@/i18n'

export interface Point {
  t: number // epoch ms
  v: number
}

const props = withDefaults(
  defineProps<{ points: Point[]; format: (v: number) => string; height?: number; label: string }>(),
  { height: 120 },
)

const vars = useThemeVars()
const osTheme = useOsTheme()
const stroke = computed(() => (osTheme.value === 'dark' ? '#3987e5' : '#2a78d6'))

const root = ref<HTMLElement>()
const width = ref(300)
let ro: ResizeObserver | undefined
onMounted(() => {
  ro = new ResizeObserver(([e]) => (width.value = Math.max(120, Math.floor(e!.contentRect.width))))
  if (root.value) ro.observe(root.value)
})
onBeforeUnmount(() => ro?.disconnect())

const pad = { top: 8, right: 8, bottom: 18, left: 44 }
const plotW = computed(() => width.value - pad.left - pad.right)
const plotH = computed(() => props.height - pad.top - pad.bottom)

const domain = computed(() => {
  const pts = props.points
  if (!pts.length) return { t0: 0, t1: 1, max: 1 }
  const max = Math.max(...pts.map((p) => p.v))
  return { t0: pts[0]!.t, t1: Math.max(pts.at(-1)!.t, pts[0]!.t + 1), max: niceMax(max) }
})

/** Rounds the axis maximum up to 1, 2 or 5 × 10^n so that ticks read well. */
function niceMax(v: number): number {
  if (v <= 0) return 1
  const exp = Math.pow(10, Math.floor(Math.log10(v)))
  for (const m of [1, 2, 5, 10]) if (v <= m * exp) return m * exp
  return 10 * exp
}

const x = (t: number) => pad.left + ((t - domain.value.t0) / (domain.value.t1 - domain.value.t0)) * plotW.value
const y = (v: number) => pad.top + plotH.value - (v / domain.value.max) * plotH.value

const path = computed(() =>
  props.points.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(''),
)
const grid = computed(() => [0, 0.5, 1].map((f) => ({ yPos: pad.top + plotH.value * (1 - f), value: domain.value.max * f })))

const timeFmt = computed(() => new Intl.DateTimeFormat(currentLocale.value, { hour: '2-digit', minute: '2-digit' }))
const timeFmtLong = computed(
  () => new Intl.DateTimeFormat(currentLocale.value, { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
)

// Hover: nearest point by time; the whole plot is the hit target.
const hover = ref<Point>()
function onMove(e: PointerEvent) {
  const pts = props.points
  if (!pts.length) return
  const rect = (e.currentTarget as SVGElement).getBoundingClientRect()
  const px = e.clientX - rect.left
  let best = pts[0]!
  for (const p of pts) if (Math.abs(x(p.t) - px) < Math.abs(x(best.t) - px)) best = p
  hover.value = best
}

const latest = computed(() => props.points.at(-1))
const summary = computed(() =>
  latest.value ? `${props.label}: ${props.format(latest.value.v)} (${timeFmtLong.value.format(latest.value.t)})` : props.label,
)
</script>

<template>
  <div ref="root" class="chart">
    <svg
      :width="width"
      :height="height"
      role="img"
      :aria-label="summary"
      @pointermove="onMove"
      @pointerleave="hover = undefined"
    >
      <g class="grid">
        <template v-for="g in grid" :key="g.yPos">
          <line :x1="pad.left" :x2="width - pad.right" :y1="g.yPos" :y2="g.yPos" />
          <text :x="pad.left - 6" :y="g.yPos + 4" text-anchor="end">{{ format(g.value) }}</text>
        </template>
        <template v-if="points.length > 1">
          <text :x="pad.left" :y="height - 4" text-anchor="start">{{ timeFmt.format(points[0]!.t) }}</text>
          <text :x="width - pad.right" :y="height - 4" text-anchor="end">{{ timeFmt.format(points.at(-1)!.t) }}</text>
        </template>
      </g>
      <path v-if="points.length > 1" :d="path" fill="none" :stroke="stroke" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
      <template v-if="hover">
        <line class="crosshair" :x1="x(hover.t)" :x2="x(hover.t)" :y1="pad.top" :y2="pad.top + plotH" />
        <circle :cx="x(hover.t)" :cy="y(hover.v)" r="4" :fill="stroke" :stroke="vars.cardColor" stroke-width="2" />
      </template>
    </svg>
    <div
      v-if="hover"
      class="tooltip"
      :style="{ left: `${Math.min(x(hover.t) + 10, width - 150)}px`, top: `${pad.top}px` }"
    >
      <div class="tt-time">{{ timeFmtLong.format(hover.t) }}</div>
      <div class="tt-value">{{ format(hover.v) }}</div>
    </div>
  </div>
</template>

<style scoped>
.chart {
  position: relative;
  width: 100%;
}

svg {
  display: block;
  overflow: visible;
}

.grid line {
  stroke: v-bind('vars.dividerColor');
  stroke-width: 1;
}

.grid text {
  fill: v-bind('vars.textColor3');
  font-size: 10px;
}

.crosshair {
  stroke: v-bind('vars.textColor3');
  stroke-width: 1;
}

.tooltip {
  position: absolute;
  pointer-events: none;
  min-width: 110px;
  padding: 4px 8px;
  border-radius: 4px;
  background: v-bind('vars.popoverColor');
  box-shadow: v-bind('vars.boxShadow2');
  font-size: 12px;
}

.tt-time {
  color: v-bind('vars.textColor3');
}

.tt-value {
  color: v-bind('vars.textColor1');
  font-weight: 600;
}
</style>
