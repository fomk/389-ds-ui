<script setup lang="ts">
/** An ACI with syntax highlighting. */
import { computed } from 'vue'
import { useThemeVars } from 'naive-ui'
import { tokenize } from '@/ldap/aci'

const props = defineProps<{ aci: string }>()
const vars = useThemeVars()
const tokens = computed(() => tokenize(props.aci))
const permClass = (text: string) => (text.toLowerCase() === 'deny' ? 'deny' : 'allow')
</script>

<template>
  <code class="aci"><span
    v-for="(tk, i) in tokens"
    :key="i"
    :class="tk.kind === 'perm' ? permClass(tk.text) : tk.kind"
  >{{ tk.text }}</span></code>
</template>

<style scoped>
.aci {
  display: block;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12.5px;
  line-height: 1.55;
}

.keyword {
  color: v-bind('vars.infoColor');
  font-weight: 600;
}

.string {
  color: v-bind('vars.successColorPressed');
}

.right {
  color: v-bind('vars.primaryColor');
}

.allow {
  color: v-bind('vars.successColor');
  font-weight: 700;
}

.deny {
  color: v-bind('vars.errorColor');
  font-weight: 700;
}

.op,
.punct {
  color: v-bind('vars.textColor3');
}
</style>
