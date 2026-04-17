<template>
  <div v-loading="loading" class="space-y-4">
    <el-card shadow="hover">
      <template #header><span class="font-bold">告警设置</span></template>
      <el-form label-width="140px">
        <el-form-item label="流量预警阈值 (%)">
          <el-input-number v-model="warnPercent" :min="0" :max="100" :step="5" style="width: 200px" />
        </el-form-item>
        <el-form-item label="流量采集间隔 (秒)">
          <el-input-number v-model="collectInterval" :min="10" :step="10" style="width: 200px" />
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
  } catch (e) {
    console.error('加载告警设置失败', e)
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    await updateSettings({
      warn_percent: String(warnPercent.value),
      collect_interval: String(collectInterval.value),
    })
    ElMessage.success('告警设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(fetchState)
</script>
