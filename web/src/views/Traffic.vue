<template>
  <div v-loading="loading" class="p-4 space-y-4">
    <!-- 服务器流量卡片 -->
    <el-card shadow="hover">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">服务器流量</span>
          <el-button size="small" @click="limitDialogVisible = true">设置限额</el-button>
        </div>
      </template>
      <el-row :gutter="24">
        <el-col :xs="24" :sm="8">
          <div class="text-sm text-gray-500 mb-1">上行流量</div>
          <div class="text-xl font-bold text-blue-500">{{ formatBytes(traffic.total_up) }}</div>
        </el-col>
        <el-col :xs="24" :sm="8">
          <div class="text-sm text-gray-500 mb-1">下行流量</div>
          <div class="text-xl font-bold text-green-500">{{ formatBytes(traffic.total_down) }}</div>
        </el-col>
        <el-col :xs="24" :sm="8">
          <div class="text-sm text-gray-500 mb-1">流量限额</div>
          <div class="text-xl font-bold">
            {{ traffic.limit_bytes > 0 ? formatBytes(traffic.limit_bytes) : '无限制' }}
          </div>
        </el-col>
      </el-row>
      <div v-if="traffic.limit_bytes > 0" class="mt-4">
        <el-progress
          :percentage="usagePercent"
          :color="usagePercent > 80 ? '#F56C6C' : '#409EFF'"
          :stroke-width="18"
          :format="() => usagePercent + '%'"
        />
      </div>
    </el-card>

    <!-- 流量历史图表 -->
    <el-card shadow="hover">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">流量趋势</span>
          <el-select v-model="days" size="small" style="width: 120px" @change="onDaysChange">
            <el-option :value="7" label="最近 7 天" />
            <el-option :value="14" label="最近 14 天" />
            <el-option :value="30" label="最近 30 天" />
            <el-option :value="60" label="最近 60 天" />
            <el-option :value="90" label="最近 90 天" />
          </el-select>
        </div>
      </template>
      <div ref="chartRef" class="w-full" style="height: 350px"></div>
    </el-card>

    <!-- 节点维度分布 -->
    <el-card shadow="hover">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">节点流量分布</span>
          <span class="text-xs text-gray-400">最近 {{ days }} 天 · 按 traffic_logs.node_id 聚合</span>
        </div>
      </template>
      <div v-if="!nodeDist.length" class="text-sm text-gray-400 py-6 text-center">
        暂无节点维度数据。新版采集会写入真实 node_id；如长期为空请检查内核运行状态。
      </div>
      <div v-else ref="nodeChartRef" class="w-full" style="height: 320px"></div>
    </el-card>

    <!-- 设置限额对话框 -->
    <el-dialog v-model="limitDialogVisible" title="设置流量限额" width="400px">
      <el-form label-width="80px">
        <el-form-item label="限额 (GB)">
          <el-input-number v-model="limitGB" :min="0" :precision="1" :step="10" style="width: 100%" />
        </el-form-item>
        <div class="text-xs text-gray-400">设为 0 表示无限制</div>
      </el-form>
      <template #footer>
        <el-button @click="limitDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="settingLimit" @click="handleSetLimit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts'
import { ElMessage } from 'element-plus'
import { getServerTraffic, setServerLimit, getTrafficHistory, getTrafficByNode } from '../api/traffic'
import { formatBytes } from '../utils/format'

const loading = ref(false)
const traffic = ref({ total_up: 0, total_down: 0, limit_bytes: 0 })
const days = ref(30)
const limitDialogVisible = ref(false)
const limitGB = ref(0)
const settingLimit = ref(false)

const chartRef = ref<HTMLDivElement>()
let chart: echarts.ECharts
const nodeChartRef = ref<HTMLDivElement>()
let nodeChart: echarts.ECharts | null = null
const nodeDist = ref<{ node_id: number; node_name: string; upload: number; download: number }[]>([])

const usagePercent = computed(() => {
  if (traffic.value.limit_bytes <= 0) return 0
  const used = traffic.value.total_up + traffic.value.total_down
  return Math.min(100, Math.round((used / traffic.value.limit_bytes) * 100))
})

const fetchTraffic = async () => {
  loading.value = true
  try {
    const { data } = await getServerTraffic()
    traffic.value = data
    limitGB.value = Math.round((data.limit_bytes || 0) / 1073741824 * 10) / 10
  } catch (e) {
    console.error('获取服务器流量失败', e)
  } finally {
    loading.value = false
  }
}

const fetchHistory = async () => {
  try {
    const { data } = await getTrafficHistory(days.value)
    const history: { date: string; upload: number; download: number }[] = data.history || []

    const dates = history.map((item) => item.date)
    const uploads = history.map((item) => item.upload)
    const downloads = history.map((item) => item.download)

    chart?.setOption({
      tooltip: {
        trigger: 'axis',
        formatter: (params: any) => {
          const date = params[0].axisValue
          let html = `<div style="font-weight:600">${date}</div>`
          params.forEach((p: any) => {
            html += `<div>${p.marker} ${p.seriesName}: ${formatBytes(p.value)}</div>`
          })
          return html
        },
      },
      legend: { data: ['上行', '下行'], bottom: 0 },
      grid: { left: '3%', right: '4%', bottom: '12%', top: '8%', containLabel: true },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: { rotate: 30, fontSize: 11 },
      },
      yAxis: {
        type: 'value',
        axisLabel: { formatter: (val: number) => formatBytes(val) },
      },
      series: [
        { name: '上行', type: 'bar', stack: 'traffic', data: uploads, itemStyle: { color: '#409EFF' } },
        { name: '下行', type: 'bar', stack: 'traffic', data: downloads, itemStyle: { color: '#67C23A' } },
      ],
    })
  } catch (e) {
    console.error('获取流量历史失败', e)
  }
}

const onDaysChange = async () => {
  await fetchHistory()
  await fetchNodeDistribution()
}

const fetchNodeDistribution = async () => {
  try {
    const { data } = await getTrafficByNode(days.value)
    nodeDist.value = data.distribution || []
    if (!nodeDist.value.length) {
      nodeChart?.dispose()
      nodeChart = null
      return
    }
    // 等待 v-if 渲染出 DOM 后再 init
    await nextTick()
    if (!nodeChart && nodeChartRef.value) {
      nodeChart = echarts.init(nodeChartRef.value)
    }
    nodeChart?.setOption({
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: any) => {
          const name = params[0].axisValue
          let html = `<div style="font-weight:600">${name}</div>`
          params.forEach((p: any) => {
            html += `<div>${p.marker} ${p.seriesName}: ${formatBytes(p.value)}</div>`
          })
          return html
        },
      },
      legend: { data: ['上行', '下行'], bottom: 0 },
      grid: { left: '3%', right: '4%', bottom: '12%', top: '8%', containLabel: true },
      xAxis: {
        type: 'category',
        data: nodeDist.value.map((d) => d.node_name),
        axisLabel: { rotate: 25, fontSize: 11 },
      },
      yAxis: {
        type: 'value',
        axisLabel: { formatter: (val: number) => formatBytes(val) },
      },
      series: [
        { name: '上行', type: 'bar', stack: 'n', data: nodeDist.value.map((d) => d.upload), itemStyle: { color: '#409EFF' } },
        { name: '下行', type: 'bar', stack: 'n', data: nodeDist.value.map((d) => d.download), itemStyle: { color: '#67C23A' } },
      ],
    })
  } catch (e) {
    console.error('获取节点流量分布失败', e)
  }
}

const handleSetLimit = async () => {
  settingLimit.value = true
  try {
    await setServerLimit(limitGB.value)
    ElMessage.success('限额设置成功')
    limitDialogVisible.value = false
    await fetchTraffic()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '设置失败')
  } finally {
    settingLimit.value = false
  }
}

const handleResize = () => {
  chart?.resize()
  nodeChart?.resize()
}

onMounted(async () => {
  await fetchTraffic()
  chart = echarts.init(chartRef.value!)
  await fetchHistory()
  await fetchNodeDistribution()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chart?.dispose()
  nodeChart?.dispose()
})
</script>
