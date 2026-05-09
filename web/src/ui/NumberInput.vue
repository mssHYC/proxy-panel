<template>
  <div :class="['ninput', { 'ninput--disabled': disabled }]">
    <input
      ref="el"
      type="text"
      inputmode="decimal"
      :value="display"
      :placeholder="placeholder"
      :disabled="disabled"
      class="ninput__field"
      @input="onInput"
      @blur="onBlur"
    />
    <div class="ninput__steppers">
      <button type="button" class="ninput__step" tabindex="-1" :disabled="disabled || atMax" @click="step(1)">+</button>
      <button type="button" class="ninput__step" tabindex="-1" :disabled="disabled || atMin" @click="step(-1)">−</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

const props = withDefaults(defineProps<{
  modelValue?: number | null
  min?: number
  max?: number
  step?: number
  precision?: number
  placeholder?: string
  disabled?: boolean
}>(), {
  step: 1,
  disabled: false,
})

const emit = defineEmits<{ 'update:modelValue': [v: number] }>()
const el = ref<HTMLInputElement>()

const display = computed(() => {
  if (props.modelValue === null || props.modelValue === undefined || Number.isNaN(props.modelValue)) return ''
  if (typeof props.precision === 'number') return Number(props.modelValue).toFixed(props.precision)
  return String(props.modelValue)
})

const atMax = computed(() => typeof props.max === 'number' && (props.modelValue ?? 0) >= props.max)
const atMin = computed(() => typeof props.min === 'number' && (props.modelValue ?? 0) <= props.min)

function clamp(v: number) {
  if (typeof props.min === 'number' && v < props.min) return props.min
  if (typeof props.max === 'number' && v > props.max) return props.max
  return v
}

function emitNumber(v: number) {
  if (typeof props.precision === 'number') {
    const f = Math.pow(10, props.precision)
    v = Math.round(v * f) / f
  }
  emit('update:modelValue', clamp(v))
}

function onInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value.replace(/[^\d.\-]/g, '')
  if (raw === '' || raw === '-') return
  const n = Number(raw)
  if (Number.isFinite(n)) emitNumber(n)
}

function onBlur() {
  // normalize display
  if (props.modelValue !== null && props.modelValue !== undefined) emitNumber(Number(props.modelValue))
}

function step(dir: number) {
  emitNumber((Number(props.modelValue) || 0) + dir * props.step)
}

defineExpose({ focus: () => el.value?.focus() })
</script>

<style scoped>
.ninput {
  display: inline-flex;
  align-items: stretch;
  width: 100%;
  height: 36px;
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  transition: border-color 150ms var(--ease-out), box-shadow 150ms var(--ease-out);
  overflow: hidden;
}
.ninput:focus-within {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px oklch(0.48 0.13 28 / 0.18);
}
.ninput--disabled { background: var(--color-surface-sunken); opacity: 0.7; }

.ninput__field {
  flex: 1;
  min-width: 0;
  padding: 0 12px;
  background: transparent;
  border: 0;
  outline: 0;
  font-family: var(--font-mono);
  font-feature-settings: 'tnum';
  font-size: 14px;
  color: var(--color-ink-strong);
}
.ninput__field::placeholder { color: var(--color-ink-soft); font-family: var(--font-sans); }

.ninput__steppers {
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--color-ink-faint);
}
.ninput__step {
  flex: 1;
  width: 26px;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-ink-muted);
  cursor: pointer;
  transition: background 100ms var(--ease-out), color 100ms var(--ease-out);
}
.ninput__step:last-child { border-bottom: 0; }
.ninput__step:hover:not(:disabled) {
  background: var(--color-surface-sunken);
  color: var(--color-ink-strong);
}
.ninput__step:disabled { opacity: 0.3; cursor: not-allowed; }
</style>
