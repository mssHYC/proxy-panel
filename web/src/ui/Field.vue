<template>
  <div :class="['field', { 'field--row': layout === 'row', 'field--invalid': !!error }]">
    <label v-if="label" :for="forId" class="field__label">
      {{ label }}
      <small v-if="hint && layout === 'row'" class="field__sublabel">{{ hint }}</small>
    </label>
    <div class="field__control">
      <slot :id="forId" :invalid="!!error" />
      <p v-if="error" class="field__error">{{ error }}</p>
      <p v-else-if="hint && layout !== 'row'" class="field__hint">{{ hint }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label?: string
  hint?: string
  error?: string
  for?: string
  layout?: 'stack' | 'row'
}>(), { layout: 'stack' })

const forId = computed(() => props.for || `field-${Math.random().toString(36).slice(2, 8)}`)
</script>

<style scoped>
.field { display: flex; flex-direction: column; gap: 6px; }

.field--row {
  display: grid;
  grid-template-columns: 168px minmax(0, 1fr);
  column-gap: 24px;
  align-items: start;
  padding: 14px 0;
  border-top: 1px solid var(--color-ink-faint);
}
.field--row + .field--row { border-top: 0; }
.field--row:first-child { border-top: 1px solid var(--color-ink-faint); }
.field--row:last-child { border-bottom: 1px solid var(--color-ink-faint); }

.field__label {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-ink-strong);
}
.field--row .field__label {
  padding-top: 8px;
}
.field__sublabel {
  display: block;
  font-size: 12px;
  font-weight: 400;
  color: var(--color-ink-muted);
  margin-top: 2px;
}

.field__control { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.field__error { margin: 2px 0 0; font-size: 12px; color: var(--color-status-crit); }
.field__hint { margin: 2px 0 0; font-size: 12px; color: var(--color-ink-muted); }
</style>
