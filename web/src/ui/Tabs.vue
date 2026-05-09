<template>
  <TabsRoot
    :model-value="modelValue"
    :class="['tabs', `tabs--${variant}`]"
    @update:model-value="(v: any) => emit('update:modelValue', String(v))"
  >
    <TabsList class="tabs__list">
      <TabsIndicator v-if="variant === 'underline'" class="tabs__indicator">
        <div class="tabs__indicator-bar" />
      </TabsIndicator>
      <TabsTrigger
        v-for="tab in tabs"
        :key="tab.value"
        :value="tab.value"
        class="tabs__tab"
      >{{ tab.label }}</TabsTrigger>
    </TabsList>
    <slot />
  </TabsRoot>
</template>

<script setup lang="ts">
import { TabsRoot, TabsList, TabsTrigger, TabsIndicator } from 'reka-ui'

withDefaults(defineProps<{
  modelValue?: string
  tabs: { label: string; value: string }[]
  variant?: 'pill' | 'underline'
}>(), { variant: 'pill' })

const emit = defineEmits<{ 'update:modelValue': [v: string] }>()
</script>

<style scoped>
.tabs__list {
  position: relative;
  display: inline-flex;
}

.tabs--pill .tabs__list {
  background: var(--color-surface-sunken);
  border-radius: 6px;
  padding: 2px;
  gap: 2px;
}
.tabs--pill .tabs__tab {
  background: transparent;
  border: 0;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  color: var(--color-ink-muted);
  border-radius: 4px;
  cursor: pointer;
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out);
  font-family: inherit;
}
.tabs--pill .tabs__tab:hover { color: var(--color-ink-strong); }
.tabs--pill .tabs__tab[data-state='active'] {
  background: var(--color-surface-raised);
  color: var(--color-ink-strong);
  font-weight: 600;
  box-shadow: 0 1px 2px oklch(0.2 0.01 80 / 0.06);
}

.tabs--underline .tabs__list {
  border-bottom: 1px solid var(--color-ink-faint);
  gap: 24px;
}
.tabs--underline .tabs__tab {
  background: transparent;
  border: 0;
  padding: 10px 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--color-ink-muted);
  cursor: pointer;
  transition: color 150ms var(--ease-out);
  font-family: inherit;
}
.tabs--underline .tabs__tab:hover { color: var(--color-ink-strong); }
.tabs--underline .tabs__tab[data-state='active'] { color: var(--color-ink-strong); font-weight: 600; }
.tabs--underline .tabs__indicator {
  position: absolute;
  bottom: -1px;
  left: 0;
  height: 2px;
  width: var(--reka-tabs-indicator-size);
  transform: translateX(var(--reka-tabs-indicator-position));
  transition: transform 200ms var(--ease-out), width 200ms var(--ease-out);
}
.tabs--underline .tabs__indicator-bar {
  width: 100%;
  height: 100%;
  background: var(--color-accent);
}

.tabs__tab:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; border-radius: 4px; }
</style>
