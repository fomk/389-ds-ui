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
} from '@lucide/vue'
import BrowserView from '@/views/BrowserView.vue'
import DatabasesView from '@/views/DatabasesView.vue'
import MonitorView from '@/views/MonitorView.vue'
import OverviewView from '@/views/OverviewView.vue'
import PlaceholderView from '@/views/PlaceholderView.vue'

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
  { path: '/logs', key: 'logs', icon: ScrollText, stage: 2 },
  { path: '/settings', key: 'settings', icon: Settings, stage: 3 },
  { path: '/plugins', key: 'plugins', icon: Puzzle, stage: 3 },
  { path: '/schema', key: 'schema', icon: BookOpen, stage: 4 },
  { path: '/policies', key: 'policies', icon: KeyRound, stage: 4 },
  { path: '/replication', key: 'replication', icon: RefreshCw, stage: 5 },
  { path: '/security', key: 'security', icon: Lock, stage: 6 },
]

declare module 'vue-router' {
  interface RouteMeta {
    nav?: NavKey
  }
}

const routes: RouteRecordRaw[] = [
  ...nav.map((n) =>
    n.view
      ? { path: n.path, component: n.view, meta: { nav: n.key } }
      : { path: n.path, component: PlaceholderView, props: { navKey: n.key, stage: n.stage }, meta: { nav: n.key } },
  ),
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})
