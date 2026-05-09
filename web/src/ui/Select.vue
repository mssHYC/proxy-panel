<template>
  <SelectRoot
    :model-value="modelValue == null ? undefined : String(modelValue)"
    :disabled="disabled"
    @update:model-value="onUpdate"
  >
    <SelectTrigger :class="['sel', { 'sel--invalid': invalid }]">
      <SelectValue :placeholder="placeholder">
        <span v-if="selectedOption" class="sel__value">{{ selectedOption.label }}</span>
      </SelectValue>
      <SelectIcon class="sel__chev">
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 4.5l3 3 3-3"/></svg>
      </SelectIcon>
    </SelectTrigger>
    <SelectPortal>
      <SelectContent class="sel__pop" :side-offset="4" position="popper">
        <SelectViewport class="sel__list">
          <SelectItem
            v-for="opt in options"
            :key="String(opt.value)"
            :value="String(opt.value)"
            :disabled="opt.disabled"
            class="sel__opt"
          >
            <SelectItemText>{{ opt.label }}</SelectItemText>
            <SelectItemIndicator class="sel__check">
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 6l3 3 5-6"/></svg>
            </SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  SelectRoot, SelectTrigger, SelectValue, SelectIcon,
  SelectPortal, SelectContent, SelectViewport,
  SelectItem, SelectItemText, SelectItemIndicator,
} from 'reka-ui'

export interface SelectOption { label: string; value: string | number; disabled?: boolean }

const props = defineProps<{
  modelValue?: string | number | null
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
  invalid?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [v: string | number | null] }>()

const selectedOption = computed(() => props.options.find((o) => String(o.value) === String(props.modelValue)))

function onUpdate(v: any) {
  if (v === undefined || v === null) {
    emit('update:modelValue', null)
    return
  }
  // try restore original value type
  const orig = props.options.find((o) => String(o.value) === String(v))
  emit('update:modelValue', orig ? orig.value : v)
}
</script>

<style scoped>
.sel {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  height: 36px;
  padding: 0 12px;
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  font-family: var(--font-sans);
  font-size: 14px;
  color: var(--color-ink-strong);
  cursor: pointer;
  text-align: left;
  gap: 8px;
  transition: border-color 150ms var(--ease-out), box-shadow 150ms var(--ease-out);
}
.sel:focus { outline: 0; border-color: var(--color-accent); box-shadow: 0 0 0 3px oklch(0.48 0.13 28 / 0.18); }
.sel[data-state='open'] { border-color: var(--color-accent); }
.sel[data-disabled] { opacity: 0.6; cursor: not-allowed; }
.sel--invalid { border-color: var(--color-status-crit); }
.sel__value { color: var(--color-ink-strong); }
.sel :deep([data-placeholder]) { color: var(--color-ink-soft); }
.sel__chev { color: var(--color-ink-muted); transition: transform 150ms var(--ease-out); }
.sel[data-state='open'] .sel__chev { transform: rotate(180deg); }
</style>

<style>
/* Portal popper — global so the popped DOM picks them up */
.sel__pop {
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 8px;
  box-shadow: var(--shadow-raised);
  overflow: hidden;
  z-index: 200;
  min-width: var(--reka-select-trigger-width);
  max-height: var(--reka-select-content-available-height, 300px);
  outline: none;
}
.sel__list { padding: 4px; }
.sel__opt {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  font-size: 13px;
  color: var(--color-ink-base);
  border-radius: 4px;
  cursor: pointer;
  outline: none;
}
.sel__opt[data-highlighted] {
  background: var(--color-surface-sunken);
  color: var(--color-ink-strong);
}
.sel__opt[data-state='checked'] {
  color: var(--color-accent-ink);
  font-weight: 600;
}
.sel__opt[data-disabled] { opacity: 0.4; cursor: not-allowed; }
.sel__check { color: var(--color-accent); display: inline-flex; }
</style>
