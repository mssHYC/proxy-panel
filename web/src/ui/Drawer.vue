<template>
  <DialogRoot :open="open" @update:open="(v: boolean) => emit('update:open', v)">
    <DialogPortal>
      <DialogOverlay class="drawer__overlay" />
      <DialogContent
        :class="['drawer__content', `drawer__content--${side}`]"
        :style="{ width: side === 'left' || side === 'right' ? width + 'px' : undefined, height: side === 'top' || side === 'bottom' ? width + 'px' : undefined }"
        @open-auto-focus="(e: Event) => e.preventDefault()"
        @pointer-down-outside="onOutside"
        @interact-outside="onOutside"
        @focus-outside="onOutside"
      >
        <header v-if="title || $slots.title" class="drawer__head">
          <DialogTitle class="drawer__title">
            <slot name="title">{{ title }}</slot>
          </DialogTitle>
          <DialogClose class="drawer__close" aria-label="关闭">×</DialogClose>
        </header>
        <div class="drawer__body">
          <slot />
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import {
  DialogRoot, DialogPortal, DialogOverlay, DialogContent, DialogTitle, DialogClose,
} from 'reka-ui'

withDefaults(defineProps<{
  open?: boolean
  title?: string
  side?: 'left' | 'right' | 'top' | 'bottom'
  width?: number
}>(), { side: 'right', width: 520 })

const emit = defineEmits<{ 'update:open': [v: boolean] }>()

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
.drawer__overlay {
  position: fixed; inset: 0;
  background: oklch(0.20 0.01 80 / 0.35);
  z-index: 50;
  animation: drawer-fade 160ms var(--ease-out);
}
.drawer__content {
  position: fixed;
  background: var(--color-surface-raised);
  z-index: 51;
  display: flex;
  flex-direction: column;
  outline: none;
  box-shadow: var(--shadow-raised);
}
.drawer__content--right { top: 0; bottom: 0; right: 0; max-width: 100vw; animation: drawer-slide-r 240ms var(--ease-out); }
.drawer__content--left  { top: 0; bottom: 0; left: 0;  max-width: 100vw; animation: drawer-slide-l 240ms var(--ease-out); }
.drawer__content--top   { top: 0; left: 0; right: 0; max-height: 100vh; animation: drawer-slide-t 240ms var(--ease-out); }
.drawer__content--bottom { bottom: 0; left: 0; right: 0; max-height: 100vh; animation: drawer-slide-b 240ms var(--ease-out); }

@keyframes drawer-fade { from { opacity: 0; } }
@keyframes drawer-slide-r { from { transform: translateX(100%); } }
@keyframes drawer-slide-l { from { transform: translateX(-100%); } }
@keyframes drawer-slide-t { from { transform: translateY(-100%); } }
@keyframes drawer-slide-b { from { transform: translateY(100%); } }

.drawer__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--color-ink-faint);
}
.drawer__title {
  font-family: var(--font-serif);
  font-size: 18px;
  font-weight: 600;
  color: var(--color-ink-strong);
  margin: 0;
}
.drawer__close {
  background: transparent; border: 0;
  width: 28px; height: 28px;
  border-radius: 6px;
  font-size: 22px;
  color: var(--color-ink-muted);
  cursor: pointer;
}
.drawer__close:hover { background: var(--color-surface-sunken); color: var(--color-ink-strong); }

.drawer__body {
  flex: 1;
  overflow-y: auto;
  padding: 20px 22px;
}
</style>
