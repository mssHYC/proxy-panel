<template>
  <div class="pb">
    <div class="pb__bar">
      <div
        class="pb__fill"
        :style="{ width: Math.min(100, Math.max(0, percent)) + '%' }"
        :data-state="state"
      />
    </div>
    <span v-if="showLabel" class="pb__label num" :data-state="state">{{ Math.round(percent) }}%</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{
  percent: number
  showLabel?: boolean
  thresholds?: { warn?: number; crit?: number }
}>()

const state = computed(() => {
  const t = props.thresholds || {}
  if (typeof t.crit === 'number' && props.percent >= t.crit) return 'crit'
  if (typeof t.warn === 'number' && props.percent >= t.warn) return 'warn'
  return 'ok'
})
</script>

<style scoped>
.pb { display: flex; align-items: center; gap: 10px; }
.pb__bar {
  flex: 1;
  height: 4px;
  background: var(--color-surface-sunken);
  border-radius: 999px;
  overflow: hidden;
}
.pb__fill {
  height: 100%;
  background: var(--color-status-ok);
  transition: width 600ms var(--ease-out);
}
.pb__fill[data-state='warn'] { background: var(--color-status-warn); }
.pb__fill[data-state='crit'] { background: var(--color-status-crit); }
.pb__label {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-ink-base);
  min-width: 36px;
  text-align: right;
}
.pb__label[data-state='warn'] { color: var(--color-status-warn); font-weight: 600; }
.pb__label[data-state='crit'] { color: var(--color-status-crit); font-weight: 600; }
</style>
