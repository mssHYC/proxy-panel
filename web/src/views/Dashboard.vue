<template>
  <div v-loading="loading" class="p-4 space-y-4">
    <!-- 统计卡片 -->
    <el-row :gutter="16">
      <!-- 用户统计 -->
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover">
          <div class="flex items-center">
            <div class="flex items-center justify-center w-12 h-12 rounded-lg bg-blue-100 text-blue-500">
              <el-icon :size="24"><User /></el-icon>
            </div>
            <div class="ml-4">
              <div class="text-sm text-gray-500">用户统计</div>
              <div class="text-xl font-bold">{{ dashboard.users.enabled }}/{{ dashboard.users.total }} <span class="text-sm font-normal text-gray-400">活跃</span></div>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 节点统计 -->
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover">
          <div class="flex items-center">
            <div class="flex items-center justify-center w-12 h-12 rounded-lg bg-green-100 text-green-500">
              <el-icon :size="24"><Connection /></el-icon>
            </div>
            <div class="ml-4">
              <div class="text-sm text-gray-500">节点统计</div>
              <div class="text-xl font-bold">{{ dashboard.nodes.enabled }}/{{ dashboard.nodes.total }} <span class="text-sm font-normal text-gray-400">在线</span></div>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 今日流量 -->
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover">
          <div class="flex items-center">
            <div class="flex items-center justify-center w-12 h-12 rounded-lg bg-orange-100 text-orange-500">
              <el-icon :size="24"><Upload /></el-icon>
            </div>
            <div class="ml-4">
              <div class="text-sm text-gray-500">今日流量</div>
              <div class="text-xl font-bold">{{ formatBytes(dashboard.today_traffic.upload + dashboard.today_traffic.download) }}</div>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 服务器流量 -->
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card shadow="hover">
          <div class="flex items-center">
            <div class="flex items-center justify-center w-12 h-12 rounded-lg" :class="usagePercent > 80 ? 'bg-red-100 text-red-500' : 'bg-green-100 text-green-500'">
              <el-icon :size="24"><Odometer /></el-icon>
            </div>
            <div class="ml-4">
              <div class="text-sm text-gray-500">服务器流量</div>
              <div class="text-xl font-bold">{{ formatBytes(serverUsed) }}</div>
              <div class="text-xs text-gray-400" v-if="dashboard.server_traffic.limit_bytes > 0">
                / {{ formatBytes(dashboard.server_traffic.limit_bytes) }} ({{ usagePercent }}%)
              </div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 内核状态 -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">内核状态</span>
      </template>
      <div class="space-y-4">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <span class="font-medium w-20">Xray</span>
            <el-tag :type="dashboard.kernel_status.xray ? 'success' : 'danger'">
              {{ dashboard.kernel_status.xray ? '运行中' : '已停止' }}
            </el-tag>
          </div>
          <el-button size="small" @click="restartKernel('xray')" :loading="restartingKernel === 'xray'">
            重启
          </el-button>
        </div>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <span class="font-medium w-20">Sing-box</span>
            <el-tag :type="dashboard.kernel_status['sing-box'] ? 'success' : 'danger'">
              {{ dashboard.kernel_status['sing-box'] ? '运行中' : '已停止' }}
            </el-tag>
          </div>
          <el-button size="small" @click="restartKernel('sing-box')" :loading="restartingKernel === 'sing-box'">
            重启
          </el-button>
        </div>
      </div>
    </el-card>

    <!-- 流量图表 -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">近30天流量趋势</span>
      </template>
      <TrafficChart />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getDashboard } from '../api/dashboard'
import request from '../api/request'
import { formatBytes } from '../utils/format'
import TrafficChart from '../components/TrafficChart.vue'

const loading = ref(false)
const restartingKernel = ref('')

const dashboard = ref({
  users: { total: 0, enabled: 0 },
  nodes: { total: 0, enabled: 0 },
  server_traffic: { total_up: 0, total_down: 0, limit_bytes: 0 },
  today_traffic: { upload: 0, download: 0, total: 0 },
  kernel_status: { xray: false, 'sing-box': false },
})

const serverUsed = computed(() => {
  const t = dashboard.value.server_traffic
  return t.total_up + t.total_down
})

const usagePercent = computed(() => {
  const limit = dashboard.value.server_traffic.limit_bytes
  if (limit <= 0) return 0
  return Math.round((serverUsed.value / limit) * 100)
})

const fetchDashboard = async () => {
  loading.value = true
  try {
    const { data } = await getDashboard()
    dashboard.value = data
  } catch (e) {
    console.error('获取仪表盘数据失败', e)
  } finally {
    loading.value = false
  }
}

const restartKernel = async (name: string) => {
  restartingKernel.value = name
  try {
    await request.post('/kernel/restart', { name })
    ElMessage.success(`${name} 重启成功`)
    // 刷新状态
    await fetchDashboard()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || `${name} 重启失败`)
  } finally {
    restartingKernel.value = ''
  }
}

onMounted(() => {
  fetchDashboard()
})
</script>
