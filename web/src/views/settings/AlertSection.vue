<template>
  <div v-loading="loading" class="space-y-4">
    <el-card shadow="hover">
      <template #header><span class="font-bold">告警设置</span></template>
      <el-form label-width="180px">
        <el-form-item label="流量预警阈值 (%)">
          <el-input-number v-model="warnPercent" :min="1" :max="100" :step="5" style="width: 200px" />
        </el-form-item>
        <el-form-item label="流量采集间隔 (秒)">
          <el-input-number v-model="collectInterval" :min="10" :step="10" style="width: 200px" />
        </el-form-item>
        <el-form-item label="服务器总流量重置">
          <div class="flex items-center gap-2 flex-wrap">
            <el-select v-model="resetPreset" size="default" style="width: 180px" @change="onPresetChange">
              <el-option value="monthly" label="每月 1 号 00:00" />
              <el-option value="weekly" label="每周一 00:00" />
              <el-option value="daily" label="每日 00:00" />
              <el-option value="custom" label="自定义" />
            </el-select>
            <el-input v-model="resetCron" :disabled="resetPreset !== 'custom'" style="width: 200px" placeholder="0 0 1 * *" />
            <span class="text-xs text-gray-400">5 段 cron (分 时 日 月 周)</span>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <div class="flex justify-end">
      <el-button type="primary" size="large" :loading="saving" @click="handleSave">
        保存告警设置
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getSettings, updateSettings } from '../../api/setting'

const loading = ref(false)
const saving = ref(false)
const warnPercent = ref(80)
const collectInterval = ref(60)
const resetCron = ref('0 0 1 * *')
const resetPreset = ref<'monthly' | 'weekly' | 'daily' | 'custom'>('monthly')

const PRESET_MAP: Record<string, string> = {
  monthly: '0 0 1 * *',
  weekly: '0 0 * * 1',
  daily: '0 0 * * *',
}

function cronToPreset(expr: string): 'monthly' | 'weekly' | 'daily' | 'custom' {
  const trimmed = expr.trim().replace(/\s+/g, ' ')
  for (const [k, v] of Object.entries(PRESET_MAP)) {
    if (v === trimmed) return k as 'monthly' | 'weekly' | 'daily'
  }
  return 'custom'
}

function onPresetChange(val: string) {
  if (val !== 'custom' && PRESET_MAP[val]) {
    resetCron.value = PRESET_MAP[val]
  }
}

async function fetchState() {
  loading.value = true
  try {
    const { data } = await getSettings()
    const map: Record<string, string> = {}
    if (Array.isArray(data)) {
      data.forEach((item: any) => { map[item.key] = item.value })
    } else if (data.settings) {
      if (Array.isArray(data.settings)) {
        data.settings.forEach((item: any) => { map[item.key] = item.value })
      } else { Object.assign(map, data.settings) }
    } else { Object.assign(map, data) }
    warnPercent.value = parseInt(map.warn_percent) || 80
    collectInterval.value = parseInt(map.collect_interval) || 60
    resetCron.value = map.reset_cron || '0 0 1 * *'
    resetPreset.value = cronToPreset(resetCron.value)
  } catch (e) {
    console.error('加载告警设置失败', e)
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  const cron = resetCron.value.trim()
  if (cron.split(/\s+/).length !== 5) {
    ElMessage.error('reset_cron 必须为 5 段格式，例如 0 0 1 * *')
    return
  }
  saving.value = true
  try {
    await updateSettings({
      warn_percent: String(warnPercent.value),
      collect_interval: String(collectInterval.value),
      reset_cron: cron,
    })
    ElMessage.success('告警设置已保存，调度已热生效')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(fetchState)
</script>
