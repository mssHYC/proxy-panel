<template>
  <div class="pg" v-if="total > 0">
    <p class="pg__total">
      共 <span class="num">{{ total }}</span> 条
    </p>
    <div class="pg__controls">
      <Select
        v-if="pageSizes && pageSizes.length"
        :model-value="size"
        :options="pageSizes.map((n) => ({ label: `${n}/页`, value: n }))"
        class="pg__size"
        @update:model-value="(v) => emit('update:size', Number(v))"
      />
      <button
        class="pg__btn"
        :disabled="page <= 1"
        @click="emit('update:page', page - 1)"
      >‹</button>
      <span class="pg__cur">
        <span class="num">{{ page }}</span>
        <span class="pg__sep">/</span>
        <span class="num">{{ totalPages }}</span>
      </span>
      <button
        class="pg__btn"
        :disabled="page >= totalPages"
        @click="emit('update:page', page + 1)"
      >›</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Select from './Select.vue'

const props = defineProps<{
  total: number
  page: number
  size: number
  pageSizes?: number[]
}>()

const emit = defineEmits<{
  'update:page': [v: number]
  'update:size': [v: number]
}>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / Math.max(1, props.size))))
</script>

<style scoped>
.pg {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 0;
  font-size: 13px;
  color: var(--color-ink-muted);
}
.pg__total { margin: 0; }
.pg__total .num { color: var(--color-ink-strong); font-weight: 600; font-family: var(--font-mono); }

.pg__controls { display: inline-flex; align-items: center; gap: 8px; }
.pg__size { width: 110px; }

.pg__btn {
  width: 32px; height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  color: var(--color-ink-muted);
  cursor: pointer;
  font-size: 16px;
  font-family: var(--font-mono);
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out);
}
.pg__btn:hover:not(:disabled) { background: var(--color-surface-sunken); color: var(--color-ink-strong); }
.pg__btn:disabled { opacity: 0.4; cursor: not-allowed; }

.pg__cur {
  font-family: var(--font-mono);
  font-feature-settings: 'tnum';
  color: var(--color-ink-strong);
  font-weight: 600;
  padding: 0 4px;
}
.pg__sep { color: var(--color-ink-soft); margin: 0 4px; }
</style>
