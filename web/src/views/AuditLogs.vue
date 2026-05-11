<template>
  <div class="logs" :class="{ 'is-loading-overlay': loading }">
    <div class="filters">
      <Input v-model="filter.actor" placeholder="操作人" class="filters__field" @blur="reload" />
      <Input v-model="filter.action" placeholder="动作（如 POST /api/users）" class="filters__field filters__field--wide" @blur="reload" />
      <Input v-model="filter.target_type" placeholder="资源类型" class="filters__field" @blur="reload" />
      <Input v-model="filter.target_id" placeholder="目标 ID" class="filters__field filters__field--narrow" @blur="reload" />
      <DateInput
        v-model="dateRange"
        range
        enable-time
        format="yyyy-MM-dd HH:mm"
        model-type="iso"
        placeholder="时间范围"
        class="filters__date"
        @update:model-value="reload"
      />
      <div class="filters__actions">
        <Button variant="primary" @click="reload">查询</Button>
        <Button :loading="exporting" @click="onExport">导出 CSV</Button>
      </div>
    </div>

    <table v-if="items.length || loading" class="dt dt--compact dt--responsive">
      <thead>
        <tr>
          <th>时间</th>
          <th>操作人</th>
          <th>动作</th>
          <th>资源</th>
          <th>目标 ID</th>
          <th>IP</th>
          <th>详情</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in items" :key="row.id || row.created_at + row.action">
          <td><span class="ts">{{ formatTime(row.created_at) }}</span></td>
          <td data-label="操作人"><span class="actor">{{ row.actor || '—' }}</span></td>
          <td data-label="动作"><span class="mono action">{{ row.action }}</span></td>
          <td data-label="资源"><span class="mono cell-meta">{{ row.target_type || '—' }}</span></td>
          <td data-label="目标 ID"><span class="num">{{ row.target_id || '—' }}</span></td>
          <td data-label="IP"><span class="mono">{{ row.ip || '—' }}</span></td>
          <td data-label="详情"><span class="detail" :title="row.detail">{{ row.detail || '—' }}</span></td>
        </tr>
      </tbody>
    </table>

    <div v-if="!loading && !items.length" class="empty-state">
      <p class="empty-state__title">暂无审计日志</p>
      <p class="empty-state__hint">操作面板时会自动记录这里。修改筛选条件试试。</p>
    </div>

    <Pagination
      v-if="total > 0"
      :total="total"
      :page="page"
      :size="size"
      :page-sizes="[20, 50, 100]"
      @update:page="(p) => { page = p; fetchData() }"
      @update:size="(s) => { size = s; page = 1; fetchData() }"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Button, Input, DateInput, Pagination, toast } from '../ui'
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

async function fetchData() {
  loading.value = true
  try {
    const { data } = await getAuditLogs({ page: page.value, size: size.value, ...buildParams() })
    items.value = data.items || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function reload() { page.value = 1; fetchData() }

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
      toast.warn(`结果已达上限 ${res.headers['x-export-limit']} 条，请缩小筛选范围`)
    } else {
      toast.success('已导出 CSV')
    }
  } catch (err: any) {
    const data = err?.response?.data
    if (data instanceof Blob) {
      try {
        const text = await data.text()
        const parsed = JSON.parse(text)
        toast.error(parsed.error || parsed.message || '导出失败')
      } catch { toast.error('导出失败') }
    } else {
      toast.error(err?.message || '导出失败')
    }
  } finally {
    exporting.value = false
  }
}

function formatTime(t: string) {
  if (!t) return '—'
  const d = new Date(t)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

onMounted(fetchData)
</script>

<style scoped>
.logs { display: flex; flex-direction: column; gap: 20px; }

.filters {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr)) auto auto;
  gap: 8px;
  align-items: center;
}
.filters__field--wide { grid-column: span 2; }
.filters__date { grid-column: span 2; }
.filters__actions { display: inline-flex; gap: 8px; grid-column: span 6 / -1; justify-content: flex-end; }

.ts { font-family: var(--font-mono); font-size: 12px; color: var(--color-ink-base); }
.actor { font-weight: 600; color: var(--color-ink-strong); }
.action { font-size: 12px; color: var(--color-ink-base); }
.cell-meta { font-size: 12px; color: var(--color-ink-muted); }
.detail {
  font-size: 12px;
  color: var(--color-ink-muted);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
  max-width: 300px;
}

@media (max-width: 1100px) {
  .filters { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .filters__field--wide, .filters__date { grid-column: span 2; }
  .filters__actions { grid-column: span 2; }
}
</style>
