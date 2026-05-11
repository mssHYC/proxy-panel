<template>
  <div class="routing">
    <div class="routing__head">
      <Tabs
        :tabs="tabs"
        :model-value="active"
        variant="pill"
        @update:model-value="(v) => (active = v as TabName)"
      />
      <button class="routing__help" @click="showHelp = true">
        <HelpCircle :size="14" :stroke-width="1.6" />
        <span>帮助</span>
      </button>
    </div>

    <div class="routing__pane">
      <CategoriesTab  v-if="active === 'categories' && config" :config="config" @refresh="load" />
      <GroupsTab      v-else-if="active === 'groups'    && config" :config="config" @refresh="load" />
      <CustomRulesTab v-else-if="active === 'custom'    && config" :config="config" @refresh="load" />
      <AdvancedTab    v-else-if="active === 'advanced'  && config" :config="config" @refresh="load" />
    </div>

    <RoutingHelpDrawer v-model="showHelp" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { HelpCircle } from 'lucide-vue-next'
import { Tabs } from '../../../ui'
import { getRoutingConfig } from '../../../api/routing'
import type { RoutingConfig } from './types'
import CategoriesTab from './CategoriesTab.vue'
import GroupsTab from './GroupsTab.vue'
import CustomRulesTab from './CustomRulesTab.vue'
import AdvancedTab from './AdvancedTab.vue'
import RoutingHelpDrawer from './RoutingHelpDrawer.vue'

type TabName = 'categories' | 'groups' | 'custom' | 'advanced'

const tabs = [
  { label: '规则分类',     value: 'categories' },
  { label: '出站组',       value: 'groups' },
  { label: '自定义规则',   value: 'custom' },
  { label: '高级',         value: 'advanced' },
]

const active = ref<TabName>('categories')
const config = ref<RoutingConfig | null>(null)
const showHelp = ref(false)

async function load() {
  const res = await getRoutingConfig()
  config.value = res.data
}
onMounted(load)
</script>

<style scoped>
.routing { display: flex; flex-direction: column; gap: 24px; }

.routing__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.routing__help {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: transparent;
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  font-size: 13px;
  color: var(--color-ink-muted);
  cursor: pointer;
  transition: background 150ms var(--ease-out), color 150ms var(--ease-out);
  font-family: inherit;
  flex-shrink: 0;
}
.routing__help:hover { background: var(--color-surface-sunken); color: var(--color-ink-strong); }

.routing__pane { min-height: 200px; }

@media (max-width: 1023px) {
  /* Let the pill tab strip scroll horizontally so all 4 tabs stay reachable
     even on narrow phones, and keep the help button visible to the right. */
  .routing__head { gap: 12px; flex-wrap: nowrap; }
  .routing__head :deep(.tabs) {
    min-width: 0;
    flex: 1 1 auto;
  }
  .routing__head :deep(.tabs__list) {
    width: 100%;
    overflow-x: auto;
    max-width: 100%;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
  }
  .routing__head :deep(.tabs__list)::-webkit-scrollbar { display: none; }
  .routing__head :deep(.tabs--pill .tabs__tab) {
    white-space: nowrap;
    flex: 0 0 auto;
    min-height: 32px;
  }
}
</style>
