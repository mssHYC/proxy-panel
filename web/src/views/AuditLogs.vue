<template>
  <el-card>
    <template #header>
      <div class="header">
        <span>审计日志</span>
        <div class="filters">
          <el-input v-model="filter.actor" placeholder="操作人" clearable style="width:120px" @change="reload" />
          <el-input v-model="filter.action" placeholder="动作 (如 POST /api/users)" clearable style="width:240px" @change="reload" />
          <el-input v-model="filter.target_type" placeholder="资源类型 (如 users)" clearable style="width:160px" @change="reload" />
          <el-input v-model="filter.target_id" placeholder="目标 ID" clearable style="width:120px" @change="reload" />
          <el-date-picker
            v-model="dateRange"
            type="datetimerange"
            range-separator="~"
            start-placeholder="起始时间"
            end-placeholder="结束时间"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            style="width:360px"
            @change="reload"
          />
          <el-button type="primary" @click="reload">查询</el-button>
          <el-button :loading="exporting" @click="onExport">导出 CSV</el-button>
        </div>
      </div>
    </template>

    <el-table :data="items" v-loading="loading" size="small">
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="actor" label="操作人" width="120" />
      <el-table-column prop="action" label="动作" min-width="240" />
      <el-table-column prop="target_type" label="资源" width="100" />
      <el-table-column prop="target_id" label="目标 ID" width="100" />
      <el-table-column prop="ip" label="IP" width="140" />
      <el-table-column prop="detail" label="详情" show-overflow-tooltip />
    </el-table>

    <el-pagination
      style="margin-top: 12px; justify-content: flex-end"
      :current-page="page"
      :page-size="size"
      :total="total"
      :page-sizes="[20, 50, 100]"
      layout="total, sizes, prev, pager, next"
      @current-change="(p: number) => { page = p; fetch() }"
      @size-change="(s: number) => { size = s; page = 1; fetch() }"
    />
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getAuditLogs, exportAuditLogs } from '../api/audit'

const loading = ref(false)
const exporting = ref(false)
const items = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const size = ref(50)
const filter = reactive({ actor: '', action: '', target_type: '', target_id: '' })
const dateRange = ref<[string, string] | null>(null)

function buildParams() {
  const params: Record<string, any> = {
    actor: filter.actor || undefined,
    action: filter.action || undefined,
    target_type: filter.target_type || undefined,
    target_id: filter.target_id || undefined,
  }
  if (dateRange.value && dateRange.value[0]) params.from = dateRange.value[0]
  if (dateRange.value && dateRange.value[1]) params.to = dateRange.value[1]
  return params
}

async function fetch() {
  loading.value = true
  try {
    const { data } = await getAuditLogs({
      page: page.value,
      size: size.value,
      ...buildParams(),
    })
    items.value = data.items || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  fetch()
}

async function onExport() {
  exporting.value = true
  try {
    const res = await exportAuditLogs(buildParams())
    const blob = new Blob([res.data], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `audit-logs-${new Date().toISOString().replace(/[:.]/g, '-')}.csv`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    if (res.headers?.['x-export-truncated'] === '1') {
      ElMessage.warning(`结果已达上限 ${res.headers['x-export-limit']} 条，请缩小筛选范围`)
    } else {
      ElMessage.success('已导出 CSV')
    }
  } catch (err: any) {
    // responseType: 'blob' 时后端 JSON 错误体也是 Blob，需要先读出来
    const data = err?.response?.data
    if (data instanceof Blob) {
      try {
        const text = await data.text()
        const parsed = JSON.parse(text)
        ElMessage.error(parsed.error || parsed.message || '导出失败')
      } catch {
        ElMessage.error('导出失败')
      }
    } else {
      ElMessage.error(err?.message || '导出失败')
    }
  } finally {
    exporting.value = false
  }
}

function formatTime(t: string) { return t ? new Date(t).toLocaleString() : '-' }

onMounted(fetch)
</script>

<style scoped>
.header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; }
.filters { display: flex; gap: 8px; flex-wrap: wrap; }
</style>
