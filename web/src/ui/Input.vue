<template>
  <label :class="['input', { 'input--invalid': invalid, 'input--disabled': disabled }]">
    <span v-if="$slots.prefix" class="input__addon"><slot name="prefix" /></span>
    <input
      ref="el"
      :type="actualType"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      :name="name"
      :id="id"
      :autocomplete="autocomplete"
      :inputmode="inputmode as any"
      :maxlength="maxlength"
      class="input__field"
      v-bind="$attrs"
      @input="onInput"
      @keyup="emit('keyup', $event)"
      @keydown="emit('keydown', $event)"
      @blur="emit('blur', $event)"
      @focus="emit('focus', $event)"
    />
    <button
      v-if="type === 'password'"
      type="button"
      class="input__reveal"
      tabindex="-1"
      @click="reveal = !reveal"
      :aria-label="reveal ? '隐藏' : '显示'"
    >
      <svg v-if="reveal" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
      <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 19c-7 0-10-7-10-7a18.45 18.45 0 0 1 5.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 10 7 10 7a18.5 18.5 0 0 1-2.16 3.19"/><line x1="2" y1="2" x2="22" y2="22"/></svg>
    </button>
    <span v-if="$slots.suffix" class="input__addon"><slot name="suffix" /></span>
  </label>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  modelValue?: string | number | null
  type?: string
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  invalid?: boolean
  id?: string
  name?: string
  autocomplete?: string
  inputmode?: string
  maxlength?: number | string
}>(), {
  type: 'text',
  disabled: false,
  readonly: false,
  invalid: false,
})

const emit = defineEmits<{
  'update:modelValue': [v: string]
  keyup: [e: KeyboardEvent]
  keydown: [e: KeyboardEvent]
  blur: [e: FocusEvent]
  focus: [e: FocusEvent]
}>()

const el = ref<HTMLInputElement>()
const reveal = ref(false)
const actualType = computed(() => (props.type === 'password' && reveal.value ? 'text' : props.type))

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLInputElement).value)
}

defineExpose({ focus: () => el.value?.focus() })
</script>

<style scoped>
.input {
  display: inline-flex;
  align-items: center;
  width: 100%;
  height: 36px;
  padding: 0 12px;
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  transition: border-color 150ms var(--ease-out), box-shadow 150ms var(--ease-out);
  cursor: text;
  gap: 8px;
}
.input:focus-within {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px oklch(0.48 0.13 28 / 0.18);
}
.input--invalid {
  border-color: var(--color-status-crit);
}
.input--invalid:focus-within {
  box-shadow: 0 0 0 3px oklch(0.50 0.16 28 / 0.18);
}
.input--disabled {
  background: var(--color-surface-sunken);
  cursor: not-allowed;
  opacity: 0.7;
}

.input__field {
  flex: 1;
  min-width: 0;
  background: transparent;
  border: 0;
  outline: 0;
  font-family: inherit;
  font-size: 14px;
  color: var(--color-ink-strong);
  padding: 0;
  height: 100%;
}
.input__field::placeholder { color: var(--color-ink-soft); }

.input__addon {
  display: inline-flex;
  align-items: center;
  color: var(--color-ink-muted);
  font-size: 13px;
}

.input__reveal {
  background: transparent;
  border: 0;
  padding: 4px;
  color: var(--color-ink-muted);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
}
.input__reveal:hover { color: var(--color-ink-strong); }
</style>
