<template>
  <div class="app-shell" :class="{ 'app-shell--drawer-open': drawerOpen }">
    <!-- Mobile top bar: only renders < 1024 via CSS -->
    <header class="m-topbar">
      <button
        class="m-topbar__menu"
        type="button"
        aria-label="打开导航"
        @click="openDrawer"
      >
        <Menu :size="18" :stroke-width="1.7" />
      </button>
      <div class="m-topbar__title">
        <span class="m-topbar__eyebrow">{{ section }}</span>
        <h1 class="m-topbar__name">{{ pageTitle }}</h1>
      </div>
      <span class="m-topbar__brand">
        <span class="brand__mark brand__mark--sm">P</span>
      </span>
    </header>

    <!-- Drawer scrim (mobile only) -->
    <div
      v-if="drawerOpen"
      class="drawer-scrim"
      aria-hidden="true"
      @click="closeDrawer"
    ></div>

    <aside class="app-aside" :class="{ 'is-open': drawerOpen }">
      <div class="brand">
        <span class="brand__mark">P</span>
        <span class="brand__word">ProxyPanel</span>
        <button
          class="m-aside-close"
          type="button"
          aria-label="关闭导航"
          @click="closeDrawer"
        >
          <X :size="18" :stroke-width="1.7" />
        </button>
      </div>

      <nav class="nav">
        <template v-for="group in groups" :key="group.label">
          <p class="nav__group">{{ group.label }}</p>
          <ul class="nav__list">
            <li v-for="item in group.items" :key="item.path">
              <router-link
                :to="item.path"
                class="nav__item"
                :class="{ 'is-active': isActive(item.path) }"
                @click="closeDrawer"
              >
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
import { computed, ref, watch, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Gauge, BarChart3, Users as UsersIcon, Package, Server, Layers,
  Filter, ScrollText, Settings as SettingsIcon, LogOut, Menu, X,
} from 'lucide-vue-next'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const drawerOpen = ref(false)
function openDrawer() {
  drawerOpen.value = true
  document.body.classList.add('scroll-lock')
}
function closeDrawer() {
  drawerOpen.value = false
  document.body.classList.remove('scroll-lock')
}
watch(() => route.fullPath, closeDrawer)
onUnmounted(() => document.body.classList.remove('scroll-lock'))

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
  z-index: 30;
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
  flex-shrink: 0;
}
.brand__mark--sm { width: 24px; height: 24px; font-size: 15px; }
.brand__word {
  font-family: var(--font-serif);
  font-size: 17px;
  font-weight: 600;
  color: var(--color-ink-strong);
  letter-spacing: -0.01em;
}

.m-aside-close {
  margin-left: auto;
  display: none;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 0;
  background: transparent;
  border-radius: 6px;
  color: var(--color-ink-muted);
  cursor: pointer;
}
.m-aside-close:hover { background: var(--color-surface-sunken); color: var(--color-ink-strong); }

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

/* Mobile top bar — hidden on ≥1024 */
.m-topbar { display: none; }
.drawer-scrim { display: none; }

@media (max-width: 1023px) {
  /* Fixed aside still claims an implicit grid row even though it's
     position:fixed — switching to block layout removes the phantom track. */
  .app-shell { display: block; grid-template-columns: none; }

  .m-topbar {
    display: flex;
    align-items: center;
    gap: 12px;
    height: 56px;
    padding: 0 16px;
    background: var(--color-surface-raised);
    border-bottom: 1px solid var(--color-ink-faint);
    position: sticky;
    top: 0;
    z-index: 20;
  }
  .m-topbar__menu {
    width: 40px; height: 40px;
    margin-left: -8px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 0;
    border-radius: 6px;
    color: var(--color-ink-strong);
    cursor: pointer;
  }
  .m-topbar__menu:hover { background: var(--color-surface-sunken); }
  .m-topbar__title {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1;
  }
  .m-topbar__eyebrow {
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-ink-muted);
    line-height: 1.2;
  }
  .m-topbar__name {
    font-family: var(--font-serif);
    font-size: 17px;
    font-weight: 600;
    color: var(--color-ink-strong);
    letter-spacing: -0.005em;
    line-height: 1.25;
    margin: 1px 0 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .m-topbar__brand { display: inline-flex; }

  /* Aside becomes an off-canvas drawer */
  .app-aside {
    position: fixed;
    top: 0; left: 0;
    width: min(320px, 86vw);
    height: 100vh;
    height: 100dvh;
    box-shadow: 0 0 0 1px transparent;
    transform: translateX(-100%);
    transition: transform 220ms var(--ease-out);
    z-index: 60;
  }
  .app-aside.is-open {
    transform: translateX(0);
    box-shadow: 0 12px 40px oklch(0.2 0.01 80 / 0.18);
  }
  .m-aside-close { display: inline-flex; }

  .drawer-scrim {
    display: block;
    position: fixed;
    inset: 0;
    background: oklch(0.2 0.01 80 / 0.35);
    backdrop-filter: blur(2px);
    z-index: 55;
    animation: scrim-in 160ms var(--ease-out);
  }
  @keyframes scrim-in { from { opacity: 0; } }

  /* Hide desktop page-head; mobile uses m-topbar */
  .page-head { display: none; }
  .page-body { padding: 16px 16px 80px; }
}

@media (max-width: 767px) {
  .page-body { padding: 12px 16px 88px; }
}

@media (max-width: 1280px) and (min-width: 1024px) {
  .page-head, .page-body { padding-left: 32px; padding-right: 32px; }
}
</style>
