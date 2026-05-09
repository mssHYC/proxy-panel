<template>
  <RadioGroupRoot
    :model-value="modelValue"
    class="rg"
    @update:model-value="(v: any) => emit('update:modelValue', String(v))"
  >
    <label v-for="opt in options" :key="String(opt.value)" class="rg__opt">
      <RadioGroupItem :value="String(opt.value)" class="rg__dot">
        <RadioGroupIndicator class="rg__ind" />
      </RadioGroupItem>
      <span class="rg__label">{{ opt.label }}</span>
    </label>
  </RadioGroupRoot>
</template>

<script setup lang="ts">
import { RadioGroupRoot, RadioGroupItem, RadioGroupIndicator } from 'reka-ui'

defineProps<{ modelValue?: string; options: { label: string; value: string | number }[] }>()
const emit = defineEmits<{ 'update:modelValue': [v: string] }>()
</script>

<style scoped>
.rg { display: inline-flex; gap: 16px; flex-wrap: wrap; }
.rg__opt { display: inline-flex; align-items: center; gap: 8px; cursor: pointer; }
.rg__dot {
  width: 16px; height: 16px;
  border-radius: 999px;
  border: 1px solid var(--color-ink-soft);
  background: var(--color-surface-raised);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  cursor: pointer;
}
.rg__dot:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.rg__dot[data-state='checked'] { border-color: var(--color-accent); }
.rg__ind {
  display: block;
  width: 8px; height: 8px;
  border-radius: 999px;
  background: var(--color-accent);
}
.rg__label { font-size: 13px; color: var(--color-ink-base); }
</style>
