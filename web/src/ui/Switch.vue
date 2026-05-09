<template>
  <SwitchRoot
    :model-value="modelValue"
    :disabled="disabled"
    class="sw"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
  >
    <SwitchThumb class="sw__thumb" />
  </SwitchRoot>
</template>

<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'reka-ui'

defineProps<{ modelValue?: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [v: boolean] }>()
</script>

<style scoped>
.sw {
  width: 32px;
  height: 18px;
  background: var(--color-ink-faint);
  border-radius: 999px;
  border: 0;
  position: relative;
  cursor: pointer;
  padding: 0;
  transition: background 180ms var(--ease-out);
  flex-shrink: 0;
}
.sw[data-state='checked'] { background: var(--color-accent); }
.sw[data-disabled] { opacity: 0.5; cursor: not-allowed; }
.sw:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }

.sw__thumb {
  display: block;
  width: 14px;
  height: 14px;
  background: white;
  border-radius: 999px;
  box-shadow: 0 1px 2px oklch(0.2 0.01 80 / 0.2);
  transition: transform 180ms var(--ease-out);
  transform: translateX(2px);
  will-change: transform;
}
.sw[data-state='checked'] .sw__thumb { transform: translateX(16px); }
</style>
