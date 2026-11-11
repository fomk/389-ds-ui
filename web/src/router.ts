import type { Component } from 'vue'
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import {
  Activity,
  BookOpen,
  Database,
  House,
  KeyRound,
  Lock,
  Network,
  Puzzle,
  RefreshCw,
  ScrollText,
  Settings,
  UsersRound,
} from '@lucide/vue'
import BrowserView from '@/views/BrowserView.vue'
import DatabasesView from '@/views/DatabasesView.vue'
import LogsView from '@/views/LogsView.vue'
import MonitorView from '@/views/MonitorView.vue'
import OverviewView from '@/views/OverviewView.vue'
import PlaceholderView from '@/views/PlaceholderView.vue'
import PluginsView from '@/views/PluginsView.vue'
import PoliciesView from '@/views/PoliciesView.vue'
import ReplicationView from '@/views/ReplicationView.vue'
import RolesView from '@/views/RolesView.vue'
import SchemaView from '@/views/SchemaView.vue'
import SettingsView from '@/views/SettingsView.vue'
import TlsView from '@/views/TlsView.vue'
import LoginView from '@/views/LoginView.vue'
import { useAuthStore } from '@/stores/auth'

/** Message keys under "nav" in the locale files. */
export type NavKey =
  | 'overview'
  | 'browser'
  | 'monitor'
  | 'databases'
  | 'logs'
  | 'settings'
  | 'plugins'
  | 'schema'
  | 'policies'
  | 'roles'
  | 'replication'
  | 'security'

export interface NavItem {
  path: string
  key: NavKey
  icon: Component
  /** Roadmap stage in which the section is implemented (docs/roadmap.md). */
  stage: number
  /** Implemented view; sections without one render a placeholder. */
  view?: Component
}

/** Sidebar sections; the ones from later stages render a placeholder. */
export const nav: NavItem[] = [
  { path: '/', key: 'overview', icon: House, stage: 0, view: OverviewView },
  { path: '/browser', key: 'browser', icon: Network, stage: 1, view: BrowserView },
  { path: '/monitor', key: 'monitor', icon: Activity, stage: 2, view: MonitorView },
  { path: '/databases', key: 'databases', icon: Database, stage: 2, view: DatabasesView },
  { path: '/logs', key: 'logs', icon: ScrollText, stage: 2, view: LogsView },
  { path: '/settings', key: 'settings', icon: Settings, stage: 3, view: SettingsView },
  { path: '/plugins', key: 'plugins', icon: Puzzle, stage: 3, view: PluginsView },
  { path: '/schema', key: 'schema', icon: BookOpen, stage: 4, view: SchemaView },
  { path: '/policies', key: 'policies', icon: KeyRound, stage: 4, view: PoliciesView },
  { path: '/roles', key: 'roles', icon: UsersRound, stage: 4, view: RolesView },
  { path: '/replication', key: 'replication', icon: RefreshCw, stage: 5, view: ReplicationView },
  { path: '/security', key: 'security', icon: Lock, stage: 6, view: TlsView },
]

declare module 'vue-router' {
  interface RouteMeta {
    nav?: NavKey
    /** Shown without the application layout and without sign-in. */
    bare?: boolean
  }
}

const routes: RouteRecordRaw[] = [
  ...nav.map((n) =>
    n.view
      ? { path: n.path, component: n.view, meta: { nav: n.key } }
      : { path: n.path, component: PlaceholderView, props: { navKey: n.key, stage: n.stage }, meta: { nav: n.key } },
  ),
  { path: '/login', name: 'login', component: LoginView, meta: { bare: true } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Without a session (when sign-in is on) only the sign-in page is reachable.
router.beforeEach(async (to) => {
  const auth = useAuthStore()
  try {
    await auth.load()
  } catch {
    return true // API down: let the page show the error
  }
  if (to.name === 'login') return auth.ready && auth.enabled ? { path: '/' } : true
  if (!auth.ready) return { name: 'login', query: to.fullPath === '/' ? {} : { next: to.fullPath } }
  return true
})
