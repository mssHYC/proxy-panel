<template>
  <component
    :is="tag"
    :type="tag === 'button' ? type : undefined"
    :class="['btn', `btn--${variant}`, { 'btn--icon': iconOnly, 'is-loading': loading, 'is-disabled': disabled }]"
    :disabled="disabled || loading"
    @click="onClick"
  >
    <span v-if="loading" class="btn__spinner" aria-hidden="true"></span>
    <slot v-else />
  </component>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'link'
  type?: 'button' | 'submit' | 'reset'
  tag?: string
  iconOnly?: boolean
  loading?: boolean
  disabled?: boolean
}>(), {
  variant: 'secondary',
  type: 'button',
  tag: 'button',
  iconOnly: false,
  loading: false,
  disabled: false,
})

const emit = defineEmits<{ click: [e: MouseEvent] }>()
function onClick(e: MouseEvent) { emit('click', e) }
</script>

<style scoped>
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 36px;
  padding: 0 14px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  letter-spacing: 0;
  font-family: var(--font-sans);
  cursor: pointer;
  border: 1px solid transparent;
  background: transparent;
  color: var(--color-ink-base);
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out), border-color 150ms var(--ease-out);
  user-select: none;
  white-space: nowrap;
}
.btn:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 2px;
}
.btn.is-disabled, .btn:disabled { opacity: 0.5; cursor: not-allowed; }

.btn--primary {
  background: var(--color-accent);
  color: white;
  border-color: var(--color-accent);
}
.btn--primary:hover:not(:disabled) {
  background: var(--color-accent-ink);
  border-color: var(--color-accent-ink);
}

.btn--secondary {
  background: var(--color-surface-raised);
  color: var(--color-ink-base);
  border-color: var(--color-ink-faint);
}
.btn--secondary:hover:not(:disabled) {
  background: var(--color-surface-sunken);
  color: var(--color-ink-strong);
  border-color: var(--color-ink-soft);
}

.btn--danger {
  background: transparent;
  color: var(--color-status-crit);
  border-color: transparent;
}
.btn--danger:hover:not(:disabled) {
  background: var(--color-status-crit-soft);
}

.btn--ghost {
  background: transparent;
  color: var(--color-ink-muted);
  border-color: transparent;
}
.btn--ghost:hover:not(:disabled) {
  background: var(--color-surface-sunken);
  color: var(--color-ink-strong);
}

.btn--link {
  background: transparent;
  color: var(--color-accent-ink);
  border-color: transparent;
  height: auto;
  padding: 0;
}
.btn--link:hover:not(:disabled) {
  text-decoration: underline;
}

.btn--icon {
  width: 32px;
  height: 32px;
  padding: 0;
}

.btn__spinner {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid currentColor;
  border-right-color: transparent;
  animation: btn-spin 700ms linear infinite;
  opacity: 0.8;
}
@keyframes btn-spin { to { transform: rotate(360deg); } }
</style>
