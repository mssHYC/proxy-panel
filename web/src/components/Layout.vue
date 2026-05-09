<template>
  <div class="app-shell">
    <aside class="app-aside">
      <div class="brand">
        <span class="brand__mark">P</span>
        <span class="brand__word">ProxyPanel</span>
      </div>

      <nav class="nav">
        <template v-for="group in groups" :key="group.label">
          <p class="nav__group">{{ group.label }}</p>
          <ul class="nav__list">
            <li v-for="item in group.items" :key="item.path">
              <router-link :to="item.path" class="nav__item" :class="{ 'is-active': isActive(item.path) }">
                <component :is="item.icon" class="nav__icon" :size="16" :stroke-width="1.6" />
                <span>{{ item.label }}</span>
              </router-link>
            </li>
          </ul>
        </template>
      </nav>

      <div class="aside-foot">
        <button class="logout" type="button" @click="handleLogout">
          <LogOut :size="16" :stroke-width="1.6" />
          <span>退出登录</span>
        </button>
      </div>
    </aside>

    <main class="app-main">
      <header class="page-head">
        <p class="eyebrow">{{ section }}</p>
        <h1 class="page-head__title">{{ pageTitle }}</h1>
      </header>

      <div class="page-body">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Gauge, BarChart3, Users as UsersIcon, Package, Server, Layers,
  Filter, ScrollText, Settings as SettingsIcon, LogOut,
} from 'lucide-vue-next'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const groups = [
  {
    label: '运行',
    items: [
      { path: '/',        label: '仪表盘',   icon: Gauge },
      { path: '/traffic', label: '流量统计', icon: BarChart3 },
    ],
  },
  {
    label: '资源',
    items: [
      { path: '/users',       label: '用户',     icon: UsersIcon },
      { path: '/plans',       label: '套餐',     icon: Package },
      { path: '/nodes',       label: '节点',     icon: Server },
      { path: '/node-groups', label: '节点分组', icon: Layers },
    ],
  },
  {
    label: '系统',
    items: [
      { path: '/routing',    label: '分流规则', icon: Filter },
      { path: '/audit-logs', label: '审计日志', icon: ScrollText },
      { path: '/settings',   label: '设置',     icon: SettingsIcon },
    ],
  },
]

const meta: Record<string, { title: string; section: string }> = {
  Dashboard:  { title: '仪表盘',   section: 'Overview' },
  Users:      { title: '用户',     section: 'Resources' },
  Plans:      { title: '套餐',     section: 'Resources' },
  Nodes:      { title: '节点',     section: 'Resources' },
  NodeGroups: { title: '节点分组', section: 'Resources' },
  Traffic:    { title: '流量统计', section: 'Overview' },
  Routing:    { title: '分流规则', section: 'System' },
  AuditLogs:  { title: '审计日志', section: 'System' },
  Settings:   { title: '设置',     section: 'System' },
}

const current = computed(() => meta[route.name as string] ?? { title: 'ProxyPanel', section: '' })
const pageTitle = computed(() => current.value.title)
const section = computed(() => current.value.section)

function isActive(path: string) {
  if (path === '/') return route.path === '/'
  return route.path === path || route.path.startsWith(path + '/')
}

function handleLogout() {
  auth.logout()
  router.push('/login')
}
</script>

<style scoped>
.app-shell {
  display: grid;
  grid-template-columns: 240px 1fr;
  min-height: 100vh;
  background: var(--color-surface-base);
}

.app-aside {
  position: sticky;
  top: 0;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-raised);
  border-right: 1px solid var(--color-ink-faint);
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 64px;
  padding: 0 20px;
  border-bottom: 1px solid var(--color-ink-faint);
}
.brand__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: var(--color-accent);
  color: white;
  font-family: var(--font-serif);
  font-size: 18px;
  font-weight: 600;
  letter-spacing: -0.02em;
}
.brand__word {
  font-family: var(--font-serif);
  font-size: 17px;
  font-weight: 600;
  color: var(--color-ink-strong);
  letter-spacing: -0.01em;
}

.nav {
  flex: 1;
  overflow-y: auto;
  padding: 12px 0 24px;
}
.nav__group {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--color-ink-muted);
  padding: 14px 20px 4px;
  margin: 0;
}
.nav__list { list-style: none; padding: 0; margin: 0; }

.nav__item {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  padding: 0 12px;
  margin: 1px 8px;
  border-radius: 6px;
  font-size: 13.5px;
  color: var(--color-ink-base);
  text-decoration: none;
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out);
}
.nav__item:hover { background: var(--color-surface-sunken); color: var(--color-ink-strong); }
.nav__item.is-active {
  background: var(--color-accent-soft);
  color: var(--color-accent-ink);
  font-weight: 600;
}
.nav__icon {
  color: var(--color-ink-muted);
  flex-shrink: 0;
}
.nav__item:hover .nav__icon { color: var(--color-ink-strong); }
.nav__item.is-active .nav__icon { color: var(--color-accent); }

.aside-foot {
  padding: 12px 16px 16px;
  border-top: 1px solid var(--color-ink-faint);
}
.logout {
  width: 100%;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 6px;
  background: transparent;
  border: 0;
  color: var(--color-ink-muted);
  font-size: 13px;
  cursor: pointer;
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out);
}
.logout:hover {
  background: var(--color-status-crit-soft);
  color: var(--color-status-crit);
}

.app-main { display: flex; flex-direction: column; min-width: 0; }
.page-head { padding: 40px 48px 8px; max-width: 1240px; width: 100%; }
.page-head__title {
  font-family: var(--font-serif);
  font-size: 30px;
  line-height: 1.2;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--color-ink-strong);
  margin: 4px 0 0;
}
.page-body { padding: 24px 48px 64px; max-width: 1240px; width: 100%; }

@media (max-width: 1024px) {
  .page-head, .page-body { padding-left: 24px; padding-right: 24px; }
}
@media (max-width: 768px) {
  .app-shell { grid-template-columns: 1fr; }
  .app-aside { position: static; height: auto; }
}
</style>
