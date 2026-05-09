<template>
  <!-- Read-only: render as inline pill list, no input affordance. -->
  <div v-if="disabled" class="ti-ro">
    <Tag
      v-for="(t, i) in modelValue"
      :key="i + t"
      :mono="false"
      class="ti-ro__tag"
    >{{ t }}</Tag>
    <span v-if="!modelValue.length" class="ti-ro__empty">—</span>
  </div>

  <!-- Editable: tags + freeform input -->
  <div v-else class="ti" @click="focus">
    <Tag
      v-for="(t, i) in modelValue"
      :key="i + t"
      :mono="false"
      closable
      @close="remove(i)"
    >{{ t }}</Tag>
    <input
      ref="inp"
      :placeholder="modelValue.length === 0 ? (placeholder || '输入并回车添加') : ''"
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
/* Read-only inline pill list */
.ti-ro {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
  min-height: 28px;
  padding: 2px 0;
}
.ti-ro__tag {
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
}
.ti-ro__empty {
  color: var(--color-ink-soft);
  font-size: 13px;
}

/* Editable input */
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
