<template>
  <router-view />
  <Toaster
    position="top-right"
    :duration="3000"
    :toast-options="{
      class: 'app-toast',
    }"
  />
</template>

<script setup lang="ts">
import { Toaster } from 'vue-sonner'
import 'vue-sonner/style.css'
</script>

<style>
.app-toast {
  --normal-bg: var(--color-surface-raised);
  --normal-text: var(--color-ink-strong);
  --normal-border: var(--color-ink-faint);
  font-family: var(--font-sans);
  font-size: 13px;
  border-radius: 8px !important;
  box-shadow: var(--shadow-raised) !important;
}
</style>
