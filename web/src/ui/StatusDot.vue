<template>
  <span class="sd">
    <span class="status-dot" :class="{ 'status-dot--pulse': pulse }" :data-state="state" />
    <span v-if="$slots.default" class="sd__label"><slot /></span>
  </span>
</template>

<script setup lang="ts">
defineProps<{
  state?: 'ok' | 'warn' | 'crit' | 'info' | 'off'
  pulse?: boolean
}>()
</script>

<style scoped>
.sd { display: inline-flex; align-items: center; }
.sd__label { font-size: 13px; color: var(--color-ink-base); }
</style>
