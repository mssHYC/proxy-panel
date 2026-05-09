<template>
  <ComboboxRoot
    multiple
    :model-value="modelValueAsStrings"
    :open="open"
    :open-on-focus="true"
    :open-on-click="true"
    :reset-search-term-on-blur="true"
    :reset-search-term-on-select="true"
    @update:model-value="onUpdate"
    @update:open="(v: any) => (open = v)"
  >
    <ComboboxAnchor class="ms" :class="{ 'ms--invalid': invalid }" @click="onAnchorClick">
      <div class="ms__tags">
        <Tag
          v-for="opt in selectedOptions"
          :key="String(opt.value)"
          :mono="false"
          closable
          @close="removeValue(opt.value)"
        >{{ opt.label }}</Tag>
        <ComboboxInput
          ref="inputEl"
          :placeholder="selectedOptions.length === 0 ? placeholder : ''"
          class="ms__input"
        />
      </div>
      <span class="ms__chev">
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 4.5l3 3 3-3"/></svg>
      </span>
    </ComboboxAnchor>
    <ComboboxPortal>
      <ComboboxContent class="ms__pop" :side-offset="4" position="popper">
        <ComboboxViewport class="ms__list">
          <ComboboxEmpty class="ms__empty">无匹配结果</ComboboxEmpty>
          <ComboboxItem
            v-for="opt in options"
            :key="String(opt.value)"
            :value="String(opt.value)"
            class="ms__opt"
          >
            <span class="ms__opt-label">{{ opt.label }}</span>
            <ComboboxItemIndicator class="ms__check">
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 6l3 3 5-6"/></svg>
            </ComboboxItemIndicator>
          </ComboboxItem>
        </ComboboxViewport>
      </ComboboxContent>
    </ComboboxPortal>
  </ComboboxRoot>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  ComboboxRoot, ComboboxAnchor, ComboboxInput, ComboboxPortal,
  ComboboxContent, ComboboxViewport, ComboboxEmpty,
  ComboboxItem, ComboboxItemIndicator,
} from 'reka-ui'
import Tag from './Tag.vue'

export interface MSOption { label: string; value: string | number }

const props = defineProps<{
  modelValue?: (string | number)[]
  options: MSOption[]
  placeholder?: string
  invalid?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [v: (string | number)[]] }>()

const open = ref(false)
const inputEl = ref<any>()

const modelValueAsStrings = computed(() => (props.modelValue || []).map(String))
const selectedOptions = computed(() =>
  (props.modelValue || [])
    .map((v) => props.options.find((o) => String(o.value) === String(v)))
    .filter(Boolean) as MSOption[],
)

function onUpdate(vs: any) {
  if (!Array.isArray(vs)) return
  const restored = vs.map((s: string) => {
    const orig = props.options.find((o) => String(o.value) === s)
    return orig ? orig.value : s
  })
  emit('update:modelValue', restored)
}

function removeValue(v: string | number) {
  const next = (props.modelValue || []).filter((x) => String(x) !== String(v))
  emit('update:modelValue', next)
}

function onAnchorClick(e: MouseEvent) {
  open.value = true
  if ((e.target as HTMLElement).tagName !== 'INPUT') {
    inputEl.value?.$el?.focus?.()
  }
}
</script>

<style scoped>
.ms {
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 36px;
  padding: 4px 8px 4px 8px;
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  cursor: text;
  gap: 6px;
  transition: border-color 150ms var(--ease-out), box-shadow 150ms var(--ease-out);
}
.ms:focus-within { border-color: var(--color-accent); box-shadow: 0 0 0 3px oklch(0.48 0.13 28 / 0.18); }
.ms--invalid { border-color: var(--color-status-crit); }

.ms__tags {
  flex: 1;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-width: 0;
  align-items: center;
}
.ms__input {
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
.ms__input::placeholder { color: var(--color-ink-soft); }

.ms__chev { color: var(--color-ink-muted); }
</style>

<style>
.ms__pop {
  background: var(--color-surface-raised);
  border: 1px solid var(--color-ink-faint);
  border-radius: 8px;
  box-shadow: var(--shadow-raised);
  overflow: hidden;
  z-index: 200;
  min-width: var(--reka-combobox-trigger-width);
  max-height: 320px;
  outline: none;
}
.ms__list { padding: 4px; max-height: 300px; overflow-y: auto; }
.ms__empty { padding: 16px; text-align: center; font-size: 13px; color: var(--color-ink-muted); }
.ms__opt {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  font-size: 13px;
  color: var(--color-ink-base);
  border-radius: 4px;
  cursor: pointer;
  outline: none;
  gap: 12px;
}
.ms__opt[data-highlighted] { background: var(--color-surface-sunken); color: var(--color-ink-strong); }
.ms__opt[data-state='checked'] { color: var(--color-accent-ink); font-weight: 600; }
.ms__opt-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ms__check { color: var(--color-accent); display: inline-flex; }
</style>
