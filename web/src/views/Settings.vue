<template>
  <div v-loading="loading" class="p-4 space-y-4">
    <h2 class="text-xl font-bold">系统设置</h2>

    <!-- Telegram 配置 -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">Telegram 配置</span>
      </template>
      <el-form label-width="120px">
        <el-form-item label="Bot Token">
          <el-input v-model="settings.tg_bot_token" type="password" show-password placeholder="请输入 Telegram Bot Token" />
        </el-form-item>
        <el-form-item label="Chat ID">
          <el-input v-model="settings.tg_chat_id" placeholder="请输入 Chat ID" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="testingTg" @click="handleTest('telegram')">
            测试连接
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 企业微信配置 -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">企业微信配置</span>
      </template>
      <el-form label-width="120px">
        <el-form-item label="Webhook URL">
          <el-input v-model="settings.wechat_webhook" placeholder="请输入企业微信 Webhook URL" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="testingWechat" @click="handleTest('wechat')">
            测试连接
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 告警设置 -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">告警设置</span>
      </template>
      <el-form label-width="140px">
        <el-form-item label="流量预警阈值 (%)">
          <el-input-number v-model="warnPercent" :min="0" :max="100" :step="5" style="width: 200px" />
        </el-form-item>
        <el-form-item label="流量采集间隔 (秒)">
          <el-input-number v-model="collectInterval" :min="10" :step="10" style="width: 200px" />
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 保存按钮 -->
    <div class="flex justify-end">
      <el-button type="primary" size="large" :loading="saving" @click="handleSave">
        保存设置
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getSettings, updateSettings } from '../api/setting'
import { testNotify } from '../api/notify'

const loading = ref(false)
const saving = ref(false)
const testingTg = ref(false)
const testingWechat = ref(false)

const settings = ref({
  tg_bot_token: '',
  tg_chat_id: '',
  wechat_webhook: '',
})

const warnPercent = ref(80)
const collectInterval = ref(60)

const fetchSettings = async () => {
  loading.value = true
  try {
    const { data } = await getSettings()
    // data 是 key-value 对象或数组
    const map: Record<string, string> = {}
    if (Array.isArray(data)) {
      data.forEach((item: any) => { map[item.key] = item.value })
    } else if (data.settings) {
      // 如果返回 { settings: [...] } 格式
      if (Array.isArray(data.settings)) {
        data.settings.forEach((item: any) => { map[item.key] = item.value })
      } else {
        Object.assign(map, data.settings)
      }
    } else {
      Object.assign(map, data)
    }

    settings.value.tg_bot_token = map.tg_bot_token || ''
    settings.value.tg_chat_id = map.tg_chat_id || ''
    settings.value.wechat_webhook = map.wechat_webhook || ''
    warnPercent.value = parseInt(map.warn_percent) || 80
    collectInterval.value = parseInt(map.collect_interval) || 60
  } catch (e) {
    console.error('获取设置失败', e)
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    const data: Record<string, string> = {
      tg_bot_token: settings.value.tg_bot_token,
      tg_chat_id: settings.value.tg_chat_id,
      wechat_webhook: settings.value.wechat_webhook,
      warn_percent: String(warnPercent.value),
      collect_interval: String(collectInterval.value),
    }
    await updateSettings(data)
    ElMessage.success('设置保存成功')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleTest = async (channel: string) => {
  if (channel === 'telegram') testingTg.value = true
  else testingWechat.value = true

  try {
    // 先保存当前设置，确保测试使用最新配置
    await handleSave()
    await testNotify(channel)
    ElMessage.success(`${channel === 'telegram' ? 'Telegram' : '企业微信'} 测试消息发送成功`)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '测试失败，请检查配置')
  } finally {
    testingTg.value = false
    testingWechat.value = false
  }
}

onMounted(() => {
  fetchSettings()
})
</script>
