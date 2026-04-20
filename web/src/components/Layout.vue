<template>
  <el-container class="layout-container">
    <!-- 侧边栏 -->
    <el-aside width="220px" class="layout-aside">
      <div class="aside-title">ProxyPanel</div>
      <el-menu
        :default-active="activeMenu"
        background-color="#1d1e21"
        text-color="#bfcbd9"
        active-text-color="#409eff"
        router
      >
        <el-menu-item index="/">
          <el-icon><Odometer /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>
        <el-menu-item index="/users">
          <el-icon><User /></el-icon>
          <span>用户管理</span>
        </el-menu-item>
        <el-menu-item index="/nodes">
          <el-icon><Connection /></el-icon>
          <span>节点管理</span>
        </el-menu-item>
        <el-menu-item index="/traffic">
          <el-icon><DataLine /></el-icon>
          <span>流量统计</span>
        </el-menu-item>
        <el-menu-item index="/audit-logs">
          <el-icon><Document /></el-icon>
          <span>审计日志</span>
        </el-menu-item>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <span>系统设置</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <!-- 右侧主体 -->
    <el-container>
      <el-header class="layout-header">
        <span class="page-title">{{ pageTitle }}</span>
        <el-button type="danger" text @click="handleLogout">
          <el-icon class="mr-1"><SwitchButton /></el-icon>
          退出登录
        </el-button>
      </el-header>
      <el-main class="layout-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Odometer, User, Connection, DataLine, Setting, SwitchButton, Document } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const titleMap: Record<string, string> = {
  Dashboard: '仪表盘',
  Users: '用户管理',
  Nodes: '节点管理',
  Traffic: '流量统计',
  AuditLogs: '审计日志',
  Settings: '系统设置',
}

const activeMenu = computed(() => route.path)
const pageTitle = computed(() => titleMap[route.name as string] || 'ProxyPanel')

function handleLogout() {
  auth.logout()
  router.push('/login')
}
</script>

<style scoped>
.layout-container {
  min-height: 100vh;
}

.layout-aside {
  background-color: #1d1e21;
  overflow-y: auto;
}

.aside-title {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: 700;
  color: #fff;
  border-bottom: 1px solid #333;
}

.layout-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #e4e7ed;
  background: #fff;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
}

.layout-main {
  background-color: #f5f7fa;
}
</style>
