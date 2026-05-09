<template>
  <div :class="['ti', { 'ti--disabled': disabled }]" @click="focus">
    <Tag
      v-for="(t, i) in modelValue"
      :key="i + t"
      :mono="false"
      :closable="!disabled"
      @close="remove(i)"
    >{{ t }}</Tag>
    <input
      ref="inp"
      :placeholder="modelValue.length === 0 ? placeholder : ''"
      :disabled="disabled"
      v-model="text"
      class="ti__input"
      @keydown.enter.prevent="commit"
      @keydown.delete="onBackspace"
      @blur="commit"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Tag } from '../../../ui'

const props = defineProps<{
  modelValue: string[]
  placeholder?: string
  disabled?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [v: string[]] }>()

const inp = ref<HTMLInputElement>()
const text = ref('')

function focus() {
  inp.value?.focus()
}

function commit() {
  const v = text.value.trim()
  if (!v) return
  if (props.modelValue.includes(v)) { text.value = ''; return }
  emit('update:modelValue', [...props.modelValue, v])
  text.value = ''
}

function remove(i: number) {
  const next = props.modelValue.slice()
  next.splice(i, 1)
  emit('update:modelValue', next)
}

function onBackspace() {
  if (text.value === '' && props.modelValue.length > 0) {
    remove(props.modelValue.length - 1)
  }
}
</script>

<style scoped>
.ti {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  width: 100%;
  min-height: 36px;
  padding: 4px 8px;
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  cursor: text;
  align-items: center;
  transition: border-color 150ms var(--ease-out), box-shadow 150ms var(--ease-out);
}
.ti:focus-within {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px oklch(0.48 0.13 28 / 0.18);
}
.ti--disabled { background: var(--color-surface-sunken); opacity: 0.7; cursor: not-allowed; }

.ti__input {
  flex: 1;
  min-width: 80px;
  background: transparent;
  border: 0;
  outline: 0;
  font-family: inherit;
  font-size: 14px;
  color: var(--color-ink-strong);
  padding: 4px 0;
}
.ti__input::placeholder { color: var(--color-ink-soft); }
</style>
