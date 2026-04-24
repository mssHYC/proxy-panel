<template>
  <div>TODO: GroupsTab</div>
</template>

<script setup lang="ts">
import type { RoutingConfig } from './types'
defineProps<{ config: RoutingConfig }>()
defineEmits<{ (e: 'refresh'): void }>()
</script>
