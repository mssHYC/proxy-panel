<template>
  <AccordionRoot
    :type="multiple ? 'multiple' : 'single'"
    :model-value="modelValue as any"
    :collapsible="true"
    class="acc"
    @update:model-value="(v: any) => emit('update:modelValue', v)"
  >
    <AccordionItem
      v-for="item in items"
      :key="item.value"
      :value="item.value"
      class="acc__item"
    >
      <AccordionHeader>
        <AccordionTrigger class="acc__trigger">
          <span class="acc__title">{{ item.title }}</span>
          <span class="acc__chev">
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 4.5l3 3 3-3"/></svg>
          </span>
        </AccordionTrigger>
      </AccordionHeader>
      <AccordionContent class="acc__content">
        <div class="acc__inner">
          <slot :name="item.value" :item="item" />
        </div>
      </AccordionContent>
    </AccordionItem>
  </AccordionRoot>
</template>

<script setup lang="ts">
import {
  AccordionRoot, AccordionItem, AccordionHeader, AccordionTrigger, AccordionContent,
} from 'reka-ui'

defineProps<{
  modelValue?: string | string[]
  items: { value: string; title: string }[]
  multiple?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [v: any] }>()
</script>

<style scoped>
.acc { display: flex; flex-direction: column; }
.acc__item { border-bottom: 1px solid var(--color-ink-faint); }
.acc__trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 14px 4px;
  background: transparent;
  border: 0;
  font-family: inherit;
  font-size: 14px;
  font-weight: 600;
  color: var(--color-ink-strong);
  cursor: pointer;
  text-align: left;
}
.acc__chev {
  color: var(--color-ink-muted);
  transition: transform 200ms var(--ease-out);
}
.acc__trigger[data-state='open'] .acc__chev { transform: rotate(180deg); }

.acc__content {
  overflow: hidden;
}
.acc__content[data-state='open']  { animation: acc-down 220ms var(--ease-out); }
.acc__content[data-state='closed'] { animation: acc-up   200ms var(--ease-out); }
@keyframes acc-down { from { height: 0; } to { height: var(--reka-accordion-content-height); } }
@keyframes acc-up   { from { height: var(--reka-accordion-content-height); } to { height: 0; } }

.acc__inner {
  padding: 0 4px 18px;
  font-size: 13px;
  color: var(--color-ink-base);
  line-height: 1.65;
}
</style>
