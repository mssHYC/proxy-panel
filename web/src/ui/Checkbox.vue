<template>
  <label class="cb">
    <CheckboxRoot
      :model-value="modelValue"
      :disabled="disabled"
      class="cb__box"
      @update:model-value="(v: any) => emit('update:modelValue', !!v)"
    >
      <CheckboxIndicator class="cb__ind">
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 5l2 2 4-4"/></svg>
      </CheckboxIndicator>
    </CheckboxRoot>
    <span v-if="$slots.default" class="cb__label"><slot /></span>
  </label>
</template>

<script setup lang="ts">
import { CheckboxRoot, CheckboxIndicator } from 'reka-ui'
defineProps<{ modelValue?: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean] }>()
</script>

<style scoped>
.cb { display: inline-flex; align-items: center; gap: 8px; cursor: pointer; }
.cb__box {
  width: 16px; height: 16px;
  border: 1px solid var(--color-ink-soft);
  border-radius: 4px;
  background: var(--color-surface-raised);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 150ms var(--ease-out), border-color 150ms var(--ease-out);
  padding: 0;
}
.cb__box:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.cb__box[data-state='checked'] {
  background: var(--color-accent);
  border-color: var(--color-accent);
}
.cb__ind { color: white; display: inline-flex; }
.cb__label { font-size: 13px; color: var(--color-ink-base); }
</style>
