<template>
  <span :class="['tag', `tag--${variant}`, { 'tag--mono': mono }]">
    <slot />
    <button v-if="closable" type="button" class="tag__close" @click.stop="emit('close')">×</button>
  </span>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  variant?: 'neutral' | 'accent' | 'ok' | 'warn' | 'crit' | 'info'
  mono?: boolean
  closable?: boolean
}>(), { variant: 'neutral', mono: true, closable: false })

const emit = defineEmits<{ close: [] }>()
</script>

<style scoped>
.tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  padding: 0 8px;
  border-radius: 4px;
  background: var(--color-surface-sunken);
  color: var(--color-ink-base);
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0;
  line-height: 1;
  white-space: nowrap;
}
.tag--mono { font-family: var(--font-mono); }

.tag--accent { background: var(--color-accent-soft); color: var(--color-accent-ink); }
.tag--ok    { background: var(--color-status-ok-soft); color: var(--color-status-ok); }
.tag--warn  { background: var(--color-status-warn-soft); color: oklch(0.42 0.10 70); }
.tag--crit  { background: var(--color-status-crit-soft); color: var(--color-status-crit); }
.tag--info  { background: oklch(0.94 0.025 235); color: var(--color-status-info); }

.tag__close {
  background: transparent;
  border: 0;
  color: currentColor;
  opacity: 0.6;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 0 0 0 2px;
}
.tag__close:hover { opacity: 1; }
</style>
