<template>
  <el-card>
    <template #header>
      <div class="header">
        <span>审计日志</span>
        <div class="filters">
          <el-input v-model="filter.actor" placeholder="操作人" clearable style="width:140px" @change="fetch" />
          <el-input v-model="filter.action" placeholder="动作 (方法 路径)" clearable style="width:220px" @change="fetch" />
          <el-button type="primary" @click="fetch">查询</el-button>
        </div>
      </div>
    </template>

    <el-table :data="items" v-loading="loading" size="small">
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="actor" label="操作人" width="140" />
      <el-table-column prop="action" label="动作" min-width="260" />
      <el-table-column prop="target_id" label="目标" width="120" />
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
import { getAuditLogs } from '../api/audit'

const loading = ref(false)
const items = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const size = ref(50)
const filter = reactive({ actor: '', action: '' })

async function fetch() {
  loading.value = true
  try {
    const { data } = await getAuditLogs({
      page: page.value,
      size: size.value,
      actor: filter.actor || undefined,
      action: filter.action || undefined,
    })
    items.value = data.items || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function formatTime(t: string) { return t ? new Date(t).toLocaleString() : '-' }

onMounted(fetch)
</script>

<style scoped>
.header { display: flex; justify-content: space-between; align-items: center; }
.filters { display: flex; gap: 8px; }
</style>
