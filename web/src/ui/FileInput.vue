<template>
  <label class="fi">
    <input
      ref="inp"
      type="file"
      :accept="accept"
      :multiple="multiple"
      :disabled="disabled"
      class="fi__native"
      @change="onChange"
    />
    <slot>
      <span class="fi__face">
        <span class="fi__icon">↑</span>
        <span class="fi__label">{{ label || '选择文件' }}</span>
      </span>
    </slot>
  </label>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  accept?: string
  multiple?: boolean
  disabled?: boolean
  label?: string
}>()

const emit = defineEmits<{ change: [files: File[]] }>()
const inp = ref<HTMLInputElement>()

function onChange(e: Event) {
  const files = Array.from((e.target as HTMLInputElement).files || [])
  emit('change', files)
  // reset so same file can re-trigger
  if (inp.value) inp.value.value = ''
}
</script>

<style scoped>
.fi { display: inline-block; cursor: pointer; }
.fi__native {
  position: absolute;
  width: 1px; height: 1px;
  opacity: 0;
  pointer-events: none;
}
.fi__face {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 36px;
  padding: 0 14px;
  border-radius: 6px;
  border: 1px solid var(--color-ink-faint);
  background: var(--color-surface-raised);
  color: var(--color-ink-base);
  font-size: 13px;
  font-weight: 500;
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out), border-color 150ms var(--ease-out);
}
.fi:hover .fi__face {
  background: var(--color-surface-sunken);
  color: var(--color-ink-strong);
  border-color: var(--color-ink-soft);
}
.fi__icon { font-family: var(--font-mono); font-weight: 700; }
</style>
