<template>
  <div ref="chartRef" class="traffic-chart"></div>
</template>

<script setup lang="ts">
import * as echarts from 'echarts'
import { onMounted, onUnmounted, ref } from 'vue'
import { getTrafficHistory } from '../api/traffic'
import { formatBytes } from '../utils/format'

const chartRef = ref<HTMLDivElement>()
let chart: echarts.ECharts

// Read design tokens from the live stylesheet so the chart stays in sync.
const cssVar = (name: string, fallback = '') => {
  if (typeof window === 'undefined') return fallback
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
}

const fetchAndRender = async () => {
  try {
    const { data } = await getTrafficHistory(30)
    const history: { date: string; upload: number; download: number }[] = data.history || []

    const dates = history.map((item) => item.date)
    const uploads = history.map((item) => item.upload)
    const downloads = history.map((item) => item.download)

    const ink     = cssVar('--color-ink-base',   '#3a3a3a')
    const muted   = cssVar('--color-ink-muted',  '#7a7a7a')
    const faint   = cssVar('--color-ink-faint',  '#e3e3e0')
    const accent  = cssVar('--color-accent',     'oklch(0.48 0.13 28)')
    const neutral = cssVar('--color-ink-soft',   '#a8a8a4')
    const fontFamily = cssVar('--font-mono', 'JetBrains Mono, monospace')

    chart.setOption({
      textStyle: { color: ink, fontFamily },
      tooltip: {
        trigger: 'axis',
        backgroundColor: cssVar('--color-surface-raised', '#fff'),
        borderColor: faint,
        textStyle: { color: ink, fontFamily },
        formatter: (params: any) => {
          const date = params[0].axisValue
          let html = `<div style="font-weight:600;margin-bottom:4px">${date}</div>`
          params.forEach((p: any) => {
            html += `<div>${p.marker} ${p.seriesName}: <b>${formatBytes(p.value)}</b></div>`
          })
          return html
        },
      },
      legend: {
        data: ['上行', '下行'],
        bottom: 0,
        textStyle: { color: muted, fontFamily },
        itemWidth: 10,
        itemHeight: 10,
      },
      grid: { left: 8, right: 12, bottom: 32, top: 12, containLabel: true },
      xAxis: {
        type: 'category',
        data: dates,
        axisLine: { lineStyle: { color: faint } },
        axisTick: { show: false },
        axisLabel: { color: muted, fontSize: 11, rotate: 30, fontFamily },
      },
      yAxis: {
        type: 'value',
        axisLine: { show: false },
        axisTick: { show: false },
        splitLine: { lineStyle: { color: faint, type: 'dashed' } },
        axisLabel: {
          color: muted,
          fontSize: 11,
          fontFamily,
          formatter: (val: number) => formatBytes(val),
        },
      },
      series: [
        { name: '上行', type: 'bar', stack: 'traffic', data: uploads,
          itemStyle: { color: accent, borderRadius: [2, 2, 0, 0] }, barWidth: '55%' },
        { name: '下行', type: 'bar', stack: 'traffic', data: downloads,
          itemStyle: { color: neutral, borderRadius: [2, 2, 0, 0] }, barWidth: '55%' },
      ],
    })
  } catch (e) {
    console.error('获取流量历史失败', e)
  }
}

const handleResize = () => chart?.resize()

onMounted(() => {
  chart = echarts.init(chartRef.value!)
  fetchAndRender()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chart?.dispose()
})
</script>

<style scoped>
.traffic-chart {
  width: 100%;
  height: 320px;
}
</style>
