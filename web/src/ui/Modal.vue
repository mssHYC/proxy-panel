<template>
  <DialogRoot :open="open" @update:open="(v: boolean) => emit('update:open', v)">
    <DialogPortal>
      <DialogOverlay class="modal__overlay" />
      <DialogContent
        class="modal__content"
        :style="{ width: width + 'px' }"
        @open-auto-focus="(e: Event) => e.preventDefault()"
        @pointer-down-outside="onOutside"
        @interact-outside="onOutside"
        @focus-outside="onOutside"
      >
        <header v-if="title || $slots.title" class="modal__head">
          <DialogTitle class="modal__title">
            <slot name="title">{{ title }}</slot>
          </DialogTitle>
          <DialogClose class="modal__close" aria-label="关闭">×</DialogClose>
        </header>
        <DialogDescription v-if="description" class="modal__desc">{{ description }}</DialogDescription>
        <div class="modal__body">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="modal__foot">
          <slot name="footer" />
        </footer>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import {
  DialogRoot, DialogPortal, DialogOverlay, DialogContent,
  DialogTitle, DialogClose, DialogDescription,
} from 'reka-ui'

withDefaults(defineProps<{
  open?: boolean
  title?: string
  description?: string
  width?: number
}>(), { width: 520 })

const emit = defineEmits<{ 'update:open': [v: boolean] }>()

// Prevent Dialog auto-close when click lands inside a portaled popover
// (Reka Select / Combobox / Tooltip, vue-datepicker menu, sonner toast, nested Modal).
function onOutside(e: any) {
  const t = (e?.detail?.originalEvent?.target ?? e?.target) as HTMLElement | null
  if (!t || !t.closest) return
  const inPortal = t.closest(
    '[role="listbox"], [role="menu"], [role="dialog"], [role="tooltip"], ' +
    '[class*="dp__"], .dp-menu, .sel__pop, .ms__pop, ' +
    '[data-sonner-toaster], [data-reka-popper-content-wrapper]',
  )
  if (inPortal) e.preventDefault()
}
</script>

<style>
.modal__overlay {
  position: fixed; inset: 0;
  background: oklch(0.20 0.01 80 / 0.35);
  backdrop-filter: blur(2px);
  z-index: 50;
  animation: modal-fade 160ms var(--ease-out);
}
.modal__content {
  position: fixed;
  top: 50%; left: 50%;
  transform: translate(-50%, -50%);
  max-width: calc(100vw - 32px);
  max-height: calc(100vh - 64px);
  display: flex;
  flex-direction: column;
  background: var(--color-surface-raised);
  border-radius: 12px;
  box-shadow: var(--shadow-raised);
  z-index: 51;
  outline: none;
  animation: modal-rise 200ms var(--ease-out);
}
@keyframes modal-fade { from { opacity: 0; } }
@keyframes modal-rise {
  from { opacity: 0; transform: translate(-50%, calc(-50% + 6px)); }
}

.modal__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 22px 24px 0;
  gap: 12px;
}
.modal__title {
  font-family: var(--font-serif);
  font-size: 20px;
  font-weight: 600;
  color: var(--color-ink-strong);
  letter-spacing: -0.005em;
  margin: 0;
}
.modal__close {
  background: transparent;
  border: 0;
  width: 28px; height: 28px;
  border-radius: 6px;
  font-size: 22px;
  line-height: 1;
  color: var(--color-ink-muted);
  cursor: pointer;
  margin: -4px -4px 0 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.modal__close:hover { background: var(--color-surface-sunken); color: var(--color-ink-strong); }

.modal__desc {
  margin: 6px 24px 0;
  font-size: 13px;
  color: var(--color-ink-muted);
}

.modal__body {
  padding: 16px 24px 8px;
  overflow-y: auto;
  flex: 1;
}

.modal__foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 16px 24px 22px;
}

/* Bottom sheet on mobile — full-width, anchored to bottom,
   sticky footer; height capped so users can swipe-scroll body */
@media (max-width: 639px) {
  .modal__content {
    inset: auto 0 0 0;
    top: auto; left: 0;
    transform: none;
    width: 100% !important;
    max-width: 100%;
    max-height: 92dvh;
    border-radius: 16px 16px 0 0;
    animation: sheet-rise 220ms var(--ease-out);
    padding-bottom: env(safe-area-inset-bottom, 0);
  }
  @keyframes sheet-rise {
    from { opacity: 0; transform: translateY(12px); }
  }
  .modal__head { padding: 18px 20px 0; }
  .modal__title { font-size: 18px; }
  .modal__body { padding: 14px 20px 8px; }
  .modal__foot {
    padding: 14px 20px;
    border-top: 1px solid var(--color-ink-faint);
    background: var(--color-surface-raised);
    position: sticky;
    bottom: 0;
  }
  .modal__foot > * { flex: 1; min-height: 44px; }
  .modal__close { width: 36px; height: 36px; }

  /* Subtle drag affordance */
  .modal__content::before {
    content: '';
    display: block;
    width: 36px;
    height: 4px;
    border-radius: 2px;
    background: var(--color-ink-faint);
    margin: 8px auto 0;
  }
}
</style>
