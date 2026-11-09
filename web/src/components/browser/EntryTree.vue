<script setup lang="ts">
import { h, onMounted, reactive, ref, shallowReactive, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NIcon, NSpin, NSwitch, NTag, NTree, useMessage, type TreeInst, type TreeOption } from 'naive-ui'
import { Ellipsis, FileText, Folder, Globe, RefreshCw, Settings, User, Users } from '@lucide/vue'
import { api, call, type EntrySummary } from '@/api/client'
import { localizeError } from '@/i18n/errors'
import { ancestorsUnder, dnKey } from '@/ldap/dn'

const props = defineProps<{ selected?: string }>()
const emit = defineEmits<{ select: [dn: string] }>()

const { t } = useI18n()
const message = useMessage()

const PAGE = 200
const SYSTEM_KEY = 'dsui.browser.showSystem'

interface Node extends TreeOption {
  key: string
  label: string
  summary?: EntrySummary
  /** Pseudo-node that loads the next page of its parent's children. */
  more?: { parent: string; cursor: string; shown: number; total: number }
  children?: Node[]
}

const roots = ref<Node[]>([])
const expanded = ref<string[]>([])
const loading = ref(false)
const treeRef = ref<TreeInst>()
const showSystem = ref(readBool(SYSTEM_KEY))
// key → node, for lookups when revealing and paging.
const nodes = shallowReactive(new Map<string, Node>())

function readBool(key: string) {
  try {
    return localStorage.getItem(key) === '1'
  } catch {
    return false
  }
}

// Nodes are reactive from the start: lazy loading mutates them through the
// lookup map, and those mutations must reach the tree.
function toNode(s: EntrySummary): Node {
  const n = reactive<Node>({ key: dnKey(s.dn), label: s.rdn, isLeaf: !s.hasChildren, summary: s })
  nodes.set(n.key, n)
  return n
}

function moreNode(parent: string, cursor: string, shown: number, total: number): Node {
  return { key: `more:${parent}:${cursor}`, label: '', isLeaf: true, more: { parent, cursor, shown, total } }
}

async function fetchPage(dn: string, cursor?: string, refresh = false) {
  return call(
    api.GET('/api/v1/entries/children', {
      params: { query: { dn, limit: PAGE, ...(cursor ? { cursor } : {}), ...(refresh ? { refresh } : {}) } },
    }),
  )
}

async function loadChildren(node: Node, refresh = false) {
  const dn = node.summary!.dn
  const page = await fetchPage(dn, undefined, refresh)
  const children = page.items.map(toNode)
  if (page.nextCursor) children.push(moreNode(node.key, page.nextCursor, page.items.length, page.total))
  node.children = children
  node.isLeaf = children.length === 0
}

function onLoad(option: TreeOption): Promise<void> {
  return loadChildren(option as Node).catch((e) => {
    const err = localizeError(e)
    message.error(`${t('browser.treeError')}: ${err.summary}`)
    ;(option as Node).children = []
  })
}

async function loadMore(more: Node) {
  const parent = nodes.get(more.more!.parent)
  if (!parent?.children) return
  try {
    const page = await fetchPage(parent.summary!.dn, more.more!.cursor)
    const items = page.items.map(toNode)
    const shown = more.more!.shown + items.length
    if (page.nextCursor) items.push(moreNode(parent.key, page.nextCursor, shown, page.total))
    const idx = parent.children.indexOf(more)
    parent.children.splice(idx, 1, ...items)
  } catch (e) {
    message.error(`${t('browser.treeError')}: ${localizeError(e).summary}`)
  }
}

async function loadRoots() {
  loading.value = true
  nodes.clear()
  try {
    const list = await call(api.GET('/api/v1/entries/roots', { params: { query: { system: showSystem.value } } }))
    roots.value = list.map((s) => {
      const n = toNode(s)
      if (!s.exists) n.isLeaf = true
      return n
    })
    expanded.value = []
    if (props.selected) await reveal(props.selected)
  } catch (e) {
    message.error(`${t('browser.treeError')}: ${localizeError(e).summary}`)
  } finally {
    loading.value = false
  }
}

/** Expands the ancestors of dn (loading them as needed) and scrolls to it. */
async function reveal(dn: string) {
  for (const root of roots.value) {
    const chain = ancestorsUnder(dn, root.summary!.dn)
    if (!chain) continue
    for (const ancestor of chain) {
      const node = nodes.get(dnKey(ancestor))
      if (!node) return // beyond a loaded page: leave the tree as is
      if (!node.children) {
        try {
          await loadChildren(node)
        } catch {
          return
        }
      }
      if (!expanded.value.includes(node.key)) expanded.value = [...expanded.value, node.key]
    }
    treeRef.value?.scrollTo({ key: dnKey(dn) })
    return
  }
}

function onSelect(_keys: unknown, options: Array<TreeOption | null>) {
  const node = options[0] as Node | null | undefined
  if (!node) return
  if (node.more) loadMore(node)
  else if (node.summary?.exists) emit('select', node.summary.dn)
}

const icons: Array<[Component, string[]]> = [
  [User, ['person', 'inetorgperson', 'posixaccount', 'organizationalperson', 'nsaccount']],
  [Users, ['groupofnames', 'groupofuniquenames', 'posixgroup', 'nsroledefinition']],
  [Folder, ['organizationalunit', 'nscontainer']],
  [Globe, ['domain', 'dcobject', 'organization']],
]

function iconFor(node: Node): Component {
  if (node.more) return Ellipsis
  if (node.summary?.system) return Settings
  const ocs = new Set(node.summary?.objectClasses.map((o) => o.toLowerCase()))
  for (const [icon, classes] of icons) if (classes.some((c) => ocs.has(c))) return icon
  return FileText
}

function renderPrefix({ option }: { option: TreeOption }) {
  return h(NIcon, { size: 16 }, () => h(iconFor(option as Node)))
}

function renderLabel({ option }: { option: TreeOption }) {
  const n = option as Node
  if (n.more) return h('span', { class: 'more' }, t('browser.loadMore', { shown: n.more.shown, total: n.more.total }))
  return h('span', { class: 'mono label', title: n.summary?.dn }, n.label)
}

function renderSuffix({ option }: { option: TreeOption }) {
  const n = option as Node
  if (n.summary && !n.summary.exists) {
    return h(NTag, { size: 'tiny', bordered: false, type: 'warning' }, () => t('browser.notCreated'))
  }
  return null
}

watch(showSystem, (v) => {
  try {
    localStorage.setItem(SYSTEM_KEY, v ? '1' : '0')
  } catch {
    // not remembered
  }
  loadRoots()
})

watch(
  () => props.selected,
  (dn) => {
    if (dn && roots.value.length) reveal(dn)
  },
)

onMounted(loadRoots)

defineExpose({ reload: loadRoots })
</script>

<template>
  <div class="tree-pane">
    <div class="toolbar">
      <label class="switch" :title="t('browser.showSystemHint')">
        <NSwitch v-model:value="showSystem" size="small" />
        {{ t('browser.showSystem') }}
      </label>
      <NButton size="small" quaternary :loading="loading" :title="t('browser.refresh')" @click="loadRoots">
        <template #icon><NIcon><RefreshCw /></NIcon></template>
      </NButton>
    </div>
    <NSpin :show="loading && !roots.length" class="tree-wrap">
      <NTree
        v-if="roots.length"
        ref="treeRef"
        :data="roots"
        :expanded-keys="expanded"
        :selected-keys="selected ? [dnKey(selected)] : []"
        :on-load="onLoad"
        :render-prefix="renderPrefix"
        :render-label="renderLabel"
        :render-suffix="renderSuffix"
        block-line
        virtual-scroll
        class="tree"
        @update:expanded-keys="(k: string[]) => (expanded = k)"
        @update:selected-keys="onSelect"
      />
      <NEmpty v-else-if="!loading" class="empty" />
    </NSpin>
  </div>
</template>

<style scoped>
.tree-pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 4px 8px;
}

.switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}

.tree-wrap {
  flex: 1;
  min-height: 0;
}

.tree-wrap :deep(.n-spin-content) {
  height: 100%;
}

.tree {
  height: 100%;
}

.tree :deep(.label) {
  white-space: nowrap;
}

.tree :deep(.more) {
  font-style: italic;
  opacity: 0.75;
}

.empty {
  padding-top: 40px;
}
</style>
