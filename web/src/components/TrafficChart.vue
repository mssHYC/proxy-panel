<template>
  <div ref="chartRef" class="w-full" style="height: 350px"></div>
</template>

<script setup lang="ts">
import * as echarts from 'echarts'
import { onMounted, onUnmounted, ref } from 'vue'
import { getTrafficHistory } from '../api/traffic'
import { formatBytes } from '../utils/format'

const chartRef = ref<HTMLDivElement>()
let chart: echarts.ECharts

const fetchAndRender = async () => {
  try {
    const { data } = await getTrafficHistory(30)
    const history: { date: string; upload: number; download: number }[] = data.history || []

    const dates = history.map((item) => item.date)
    const uploads = history.map((item) => item.upload)
    const downloads = history.map((item) => item.download)

    chart.setOption({
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
      legend: {
        data: ['上行', '下行'],
        bottom: 0,
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: '12%',
        top: '8%',
        containLabel: true,
      },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: {
          rotate: 30,
          fontSize: 11,
        },
      },
      yAxis: {
        type: 'value',
        axisLabel: {
          formatter: (val: number) => formatBytes(val),
        },
      },
      series: [
        {
          name: '上行',
          type: 'bar',
          stack: 'traffic',
          data: uploads,
          itemStyle: { color: '#409EFF' },
        },
        {
          name: '下行',
          type: 'bar',
          stack: 'traffic',
          data: downloads,
          itemStyle: { color: '#67C23A' },
        },
      ],
    })
  } catch (e) {
    console.error('获取流量历史失败', e)
  }
}

const handleResize = () => {
  chart?.resize()
}

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
