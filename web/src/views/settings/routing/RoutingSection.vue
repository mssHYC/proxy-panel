<template>
  <div class="routing-section">
    <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px">
      <div />
      <el-button :icon="QuestionFilled" @click="showHelp = true">帮助</el-button>
    </div>
    <el-tabs v-model="active">
      <el-tab-pane label="规则分类" name="categories">
        <CategoriesTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
      <el-tab-pane label="出站组" name="groups">
        <GroupsTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
      <el-tab-pane label="自定义规则" name="custom">
        <CustomRulesTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
      <el-tab-pane label="高级" name="advanced">
        <AdvancedTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
    </el-tabs>
    <RoutingHelpDrawer v-model="showHelp" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { QuestionFilled } from '@element-plus/icons-vue'
import { getRoutingConfig } from '../../../api/routing'
import type { RoutingConfig } from './types'
import CategoriesTab from './CategoriesTab.vue'
import GroupsTab from './GroupsTab.vue'
import CustomRulesTab from './CustomRulesTab.vue'
import AdvancedTab from './AdvancedTab.vue'
import RoutingHelpDrawer from './RoutingHelpDrawer.vue'

const active = ref('categories')
const config = ref<RoutingConfig | null>(null)
const showHelp = ref(false)

async function load() {
  const res = await getRoutingConfig()
  config.value = res.data
}
onMounted(load)
</script>
