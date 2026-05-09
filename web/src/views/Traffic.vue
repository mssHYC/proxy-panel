<template>
  <div class="traffic" :class="{ 'is-loading-overlay': loading }">
    <section>
      <header class="section-head">
        <div>
          <p class="eyebrow">服务器</p>
          <h2 class="section-head__title">本周期总量</h2>
        </div>
        <Button @click="limitDialogVisible = true">设置限额</Button>
      </header>

      <dl class="traffic-summary">
        <div>
          <dt>上行</dt>
          <dd class="num">{{ formatBytes(traffic.total_up) }}</dd>
        </div>
        <div>
          <dt>下行</dt>
          <dd class="num">{{ formatBytes(traffic.total_down) }}</dd>
        </div>
        <div>
          <dt>限额</dt>
          <dd class="num">{{ traffic.limit_bytes > 0 ? formatBytes(traffic.limit_bytes) : '∞' }}</dd>
        </div>
      </dl>

      <div v-if="traffic.limit_bytes > 0" class="quota">
        <ProgressBar :percent="usagePercent" :thresholds="{ warn: 80, crit: 100 }" />
        <p class="quota__hint">
          <span class="num" :data-state="quotaState">{{ usagePercent }}%</span>
          已用 ·
          剩余 <span class="num">{{ formatBytes(Math.max(traffic.limit_bytes - traffic.total_up - traffic.total_down, 0)) }}</span>
        </p>
      </div>
    </section>

    <hr class="divider-h" />

    <section>
      <header class="section-head">
        <div>
          <p class="eyebrow">趋势</p>
          <h2 class="section-head__title">流量历史</h2>
        </div>
        <Tabs
          :tabs="rangeOptions.map(o => ({ label: o.label, value: String(o.value) }))"
          :model-value="String(days)"
          variant="pill"
          @update:model-value="(v) => setRange(Number(v))"
        />
      </header>
      <div ref="chartRef" class="chart-canvas"></div>
    </section>

    <hr class="divider-h" />

    <section>
      <header class="section-head">
        <div>
          <p class="eyebrow">节点</p>
          <h2 class="section-head__title">流量分布</h2>
        </div>
        <span class="section-head__hint">最近 <span class="num">{{ days }}</span> 天</span>
      </header>
      <p v-if="!nodeDist.length" class="empty">
        暂无节点维度数据。新版采集会写入真实 node_id；如长期为空请检查内核运行状态。
      </p>
      <div v-else ref="nodeChartRef" class="chart-canvas"></div>
    </section>

    <Modal v-model:open="limitDialogVisible" title="设置流量限额" :width="420">
      <Field label="限额" hint="GB · 0 为无限制" layout="row">
        <NumberInput v-model="limitGB" :min="0" :precision="1" :step="10" />
      </Field>
      <template #footer>
        <Button @click="limitDialogVisible = false">取消</Button>
        <Button variant="primary" :loading="settingLimit" @click="handleSetLimit">保存</Button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts'
import { Button, NumberInput, Modal, Field, ProgressBar, Tabs, toast } from '../ui'
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

const rangeOptions = [
  { label: '7d',  value: 7 },
  { label: '14d', value: 14 },
  { label: '30d', value: 30 },
  { label: '60d', value: 60 },
  { label: '90d', value: 90 },
]

const usagePercent = computed(() => {
  if (traffic.value.limit_bytes <= 0) return 0
  const used = traffic.value.total_up + traffic.value.total_down
  return Math.min(100, Math.round((used / traffic.value.limit_bytes) * 100))
})
const quotaState = computed(() => {
  const p = usagePercent.value
  if (p >= 100) return 'crit'
  if (p >= 80) return 'warn'
  return 'ok'
})

const cssVar = (name: string, fallback = '') => {
  if (typeof window === 'undefined') return fallback
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
}

function chartTheme() {
  return {
    ink:    cssVar('--color-ink-base',  '#3a3a3a'),
    muted:  cssVar('--color-ink-muted', '#7a7a7a'),
    faint:  cssVar('--color-ink-faint', '#e3e3e0'),
    accent: cssVar('--color-accent',    'oklch(0.48 0.13 28)'),
    soft:   cssVar('--color-ink-soft',  '#a8a8a4'),
    raised: cssVar('--color-surface-raised', '#fff'),
    font:   cssVar('--font-mono', 'JetBrains Mono, monospace'),
  }
}

function buildOpts(categories: string[], up: number[], down: number[]) {
  const t = chartTheme()
  return {
    textStyle: { color: t.ink, fontFamily: t.font },
    tooltip: {
      trigger: 'axis',
      backgroundColor: t.raised, borderColor: t.faint,
      textStyle: { color: t.ink, fontFamily: t.font },
      formatter: (params: any) => {
        const name = params[0].axisValue
        let html = `<div style="font-weight:600;margin-bottom:4px">${name}</div>`
        params.forEach((p: any) => {
          html += `<div>${p.marker} ${p.seriesName}: <b>${formatBytes(p.value)}</b></div>`
        })
        return html
      },
    },
    legend: { data: ['上行', '下行'], bottom: 0, textStyle: { color: t.muted, fontFamily: t.font }, itemWidth: 10, itemHeight: 10 },
    grid: { left: 8, right: 12, bottom: 32, top: 12, containLabel: true },
    xAxis: {
      type: 'category', data: categories,
      axisLine: { lineStyle: { color: t.faint } },
      axisTick: { show: false },
      axisLabel: { color: t.muted, fontSize: 11, rotate: 25, fontFamily: t.font },
    },
    yAxis: {
      type: 'value',
      axisLine: { show: false }, axisTick: { show: false },
      splitLine: { lineStyle: { color: t.faint, type: 'dashed' } },
      axisLabel: { color: t.muted, fontSize: 11, fontFamily: t.font, formatter: (v: number) => formatBytes(v) },
    },
    series: [
      { name: '上行', type: 'bar', stack: 'n', data: up, itemStyle: { color: t.accent, borderRadius: [2, 2, 0, 0] }, barWidth: '55%' },
      { name: '下行', type: 'bar', stack: 'n', data: down, itemStyle: { color: t.soft, borderRadius: [2, 2, 0, 0] }, barWidth: '55%' },
    ],
  }
}

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
    chart?.setOption(buildOpts(
      history.map((i) => i.date),
      history.map((i) => i.upload),
      history.map((i) => i.download),
    ))
  } catch (e) { console.error('获取流量历史失败', e) }
}

function setRange(v: number) {
  days.value = v
  onDaysChange()
}

const onDaysChange = async () => {
  await fetchHistory()
  await fetchNodeDistribution()
}

const fetchNodeDistribution = async () => {
  try {
    const { data } = await getTrafficByNode(days.value)
    nodeDist.value = data.distribution || []
    if (!nodeDist.value.length) { nodeChart?.dispose(); nodeChart = null; return }
    await nextTick()
    if (!nodeChart && nodeChartRef.value) nodeChart = echarts.init(nodeChartRef.value)
    nodeChart?.setOption(buildOpts(
      nodeDist.value.map((d) => d.node_name),
      nodeDist.value.map((d) => d.upload),
      nodeDist.value.map((d) => d.download),
    ))
  } catch (e) { console.error('获取节点流量分布失败', e) }
}

const handleSetLimit = async () => {
  settingLimit.value = true
  try {
    await setServerLimit(limitGB.value)
    toast.success('限额设置成功')
    limitDialogVisible.value = false
    await fetchTraffic()
  } catch (e: any) { toast.error(e.response?.data?.message || '设置失败') }
  finally { settingLimit.value = false }
}

const handleResize = () => { chart?.resize(); nodeChart?.resize() }

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

<style scoped>
.traffic { display: flex; flex-direction: column; gap: 40px; }

.traffic-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 200px));
  gap: 32px;
  margin: 0;
}
.traffic-summary div { display: flex; flex-direction: column; gap: 4px; }
.traffic-summary dt { font-size: 11px; font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; color: var(--color-ink-muted); margin: 0; }
.traffic-summary dd { margin: 0; font-family: var(--font-mono); font-size: 22px; font-weight: 600; color: var(--color-ink-strong); }

.quota { margin-top: 24px; max-width: 720px; display: flex; flex-direction: column; gap: 8px; }
.quota__hint { margin: 0; font-size: 13px; color: var(--color-ink-muted); }
.quota__hint .num { color: var(--color-ink-strong); font-weight: 600; }
.quota__hint .num[data-state="warn"] { color: var(--color-status-warn); }
.quota__hint .num[data-state="crit"] { color: var(--color-status-crit); }

.chart-canvas { width: 100%; height: 340px; }
.empty { font-size: 13px; color: var(--color-ink-muted); padding: 24px 0; }
</style>
