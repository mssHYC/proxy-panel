<template>
  <textarea
    :value="modelValue"
    :rows="rows"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readonly"
    :class="['textarea', { 'textarea--mono': mono }]"
    v-bind="$attrs"
    @input="(e) => $emit('update:modelValue', (e.target as HTMLTextAreaElement).value)"
  />
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })
withDefaults(defineProps<{
  modelValue?: string
  rows?: number
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  mono?: boolean
}>(), { rows: 4, mono: false })

defineEmits<{ 'update:modelValue': [v: string] }>()
</script>

<style scoped>
.textarea {
  display: block;
  width: 100%;
  padding: 10px 12px;
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  font-family: var(--font-sans);
  font-size: 14px;
  line-height: 1.55;
  color: var(--color-ink-strong);
  resize: vertical;
  outline: none;
  transition: border-color 150ms var(--ease-out), box-shadow 150ms var(--ease-out);
}
.textarea--mono {
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.7;
}
.textarea::placeholder { color: var(--color-ink-soft); }
.textarea:focus {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px oklch(0.48 0.13 28 / 0.18);
}
.textarea:disabled { background: var(--color-surface-sunken); opacity: 0.7; cursor: not-allowed; }
</style>
