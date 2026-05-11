<template>
  <div class="settings">
    <nav class="settings-nav" role="tablist">
      <button
        v-for="tab in tabs"
        :key="tab.name"
        :class="['settings-nav__item', { 'is-active': activeTab === tab.name }]"
        role="tab"
        :aria-selected="activeTab === tab.name"
        @click="activeTab = tab.name"
      >
        <span class="settings-nav__label">{{ tab.label }}</span>
        <span class="settings-nav__hint">{{ tab.hint }}</span>
      </button>
    </nav>

    <section class="settings-pane">
      <component :is="paneMap[activeTab]" />
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, markRaw, type Component } from 'vue'
import AccountSection from './settings/AccountSection.vue'
import NotifySection from './settings/NotifySection.vue'
import AlertSection from './settings/AlertSection.vue'
import FirewallSection from './settings/FirewallSection.vue'
import BackupSection from './settings/BackupSection.vue'

type TabName = 'account' | 'notify' | 'alert' | 'firewall' | 'backup'

const tabs: { name: TabName; label: string; hint: string }[] = [
  { name: 'account',  label: '账号',   hint: '密码与两步验证' },
  { name: 'notify',   label: '通知',   hint: 'Telegram 与企业微信' },
  { name: 'alert',    label: '告警',   hint: '触发条件与防骚扰' },
  { name: 'firewall', label: '防火墙', hint: '面板访问控制' },
  { name: 'backup',   label: '备份',   hint: '导入与导出' },
]

const paneMap: Record<TabName, Component> = {
  account:  markRaw(AccountSection),
  notify:   markRaw(NotifySection),
  alert:    markRaw(AlertSection),
  firewall: markRaw(FirewallSection),
  backup:   markRaw(BackupSection),
}

const activeTab = ref<TabName>('account')
</script>

<style scoped>
.settings {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 48px;
  align-items: start;
}

.settings-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  position: sticky;
  top: 24px;
}
.settings-nav__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  text-align: left;
  padding: 10px 12px;
  border-radius: 6px;
  border: 0;
  background: transparent;
  cursor: pointer;
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out);
  font-family: inherit;
}
.settings-nav__item:hover { background: var(--color-surface-sunken); }
.settings-nav__item.is-active { background: var(--color-accent-soft); }
.settings-nav__label {
  font-size: 14px; font-weight: 600;
  color: var(--color-ink-base);
}
.settings-nav__item.is-active .settings-nav__label { color: var(--color-accent-ink); }
.settings-nav__hint { font-size: 12px; color: var(--color-ink-muted); }

.settings-pane { max-width: 720px; }

@media (max-width: 900px) {
  .settings { grid-template-columns: 1fr; gap: 24px; }
  .settings-nav {
    position: static;
    flex-direction: row;
    gap: 2px;
    overflow-x: auto;
    scroll-snap-type: x proximity;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: none;
    margin: 0 -16px;
    padding: 0 16px;
    border-bottom: 1px solid var(--color-ink-faint);
  }
  .settings-nav::-webkit-scrollbar { display: none; }
  .settings-nav__item {
    flex: 0 0 auto;
    flex-direction: row;
    align-items: center;
    gap: 6px;
    padding: 12px 14px;
    border-radius: 0;
    scroll-snap-align: start;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
  }
  .settings-nav__item:hover { background: transparent; }
  .settings-nav__item.is-active {
    background: transparent;
    border-bottom-color: var(--color-accent);
  }
  .settings-nav__hint { display: none; }
}
</style>
