<template>
  <div class="p-4">
    <h2 class="text-xl font-bold mb-4">系统设置</h2>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="账号" name="account">
        <AccountSection v-if="activeTab === 'account'" />
      </el-tab-pane>
      <el-tab-pane label="通知" name="notify">
        <NotifySection v-if="activeTab === 'notify'" />
      </el-tab-pane>
      <el-tab-pane label="告警" name="alert">
        <AlertSection v-if="activeTab === 'alert'" />
      </el-tab-pane>
      <el-tab-pane label="分流规则" name="rules">
        <RulesSection v-if="activeTab === 'rules'" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AccountSection from './settings/AccountSection.vue'
import NotifySection from './settings/NotifySection.vue'
import AlertSection from './settings/AlertSection.vue'
import RulesSection from './settings/RulesSection.vue'

const activeTab = ref<'account' | 'notify' | 'alert' | 'rules'>('account')
</script>
