<template>
  <TooltipProvider :delay-duration="200">
    <TooltipRoot>
      <TooltipTrigger as-child>
        <slot />
      </TooltipTrigger>
      <TooltipPortal>
        <TooltipContent class="tt" :side="side" :side-offset="6">
          {{ content }}
          <slot name="content" />
        </TooltipContent>
      </TooltipPortal>
    </TooltipRoot>
  </TooltipProvider>
</template>

<script setup lang="ts">
import { TooltipProvider, TooltipRoot, TooltipTrigger, TooltipPortal, TooltipContent } from 'reka-ui'

withDefaults(defineProps<{
  content?: string
  side?: 'top' | 'right' | 'bottom' | 'left'
}>(), { side: 'top' })
</script>

<style>
.tt {
  background: var(--color-ink-strong);
  color: var(--color-surface-base);
  padding: 6px 10px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.5;
  max-width: 280px;
  z-index: 200;
  box-shadow: 0 4px 12px oklch(0.2 0.01 80 / 0.18);
  white-space: pre-wrap;
}
</style>
