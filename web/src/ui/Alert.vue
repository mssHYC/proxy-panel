<template>
  <div :class="['alert', `alert--${tone}`]">
    <span v-if="dot" class="status-dot" :data-state="dotState" />
    <div class="alert__body">
      <p v-if="title" class="alert__title">{{ title }}</p>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
const props = withDefaults(defineProps<{
  tone?: 'info' | 'warn' | 'crit'
  title?: string
  dot?: boolean
}>(), { tone: 'info', dot: true })

const dotState = computed(() => (props.tone === 'info' ? 'info' : props.tone))
</script>

<style scoped>
.alert {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 12px 14px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--color-ink-base);
  line-height: 1.55;
}
.alert--info { background: oklch(0.96 0.02 235); }
.alert--warn { background: var(--color-status-warn-soft); color: oklch(0.42 0.10 70); }
.alert--crit { background: var(--color-status-crit-soft); color: var(--color-status-crit); }

.alert .status-dot { margin-top: 8px; margin-right: 0; }
.alert__body { flex: 1; min-width: 0; }
.alert__title { margin: 0 0 2px; font-weight: 600; color: inherit; }
</style>
