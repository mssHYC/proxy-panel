<template>
  <div v-loading="loading" class="space-y-4">
    <el-card shadow="hover">
      <template #header><span class="font-bold">Telegram 配置</span></template>
      <el-form label-width="120px">
        <el-form-item label="Bot Token">
          <el-input v-model="form.tg_bot_token" type="password" show-password placeholder="请输入 Telegram Bot Token" />
        </el-form-item>
        <el-form-item label="Chat ID">
          <el-input v-model="form.tg_chat_id" placeholder="请输入 Chat ID" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="testingTg" @click="handleTest('telegram')">测试连接</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="hover">
      <template #header><span class="font-bold">企业微信配置</span></template>
      <el-form label-width="120px">
        <el-form-item label="Webhook URL">
          <el-input v-model="form.wechat_webhook" placeholder="请输入企业微信 Webhook URL" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="testingWechat" @click="handleTest('wechat')">测试连接</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <div class="flex justify-end">
      <el-button type="primary" size="large" :loading="saving" @click="handleSave">
        保存通知设置
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getSettings, updateSettings } from '../../api/setting'
import { testNotify } from '../../api/notify'

const loading = ref(false)
const saving = ref(false)
const testingTg = ref(false)
const testingWechat = ref(false)

const form = ref({ tg_bot_token: '', tg_chat_id: '', wechat_webhook: '' })

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
    form.value.tg_bot_token = map.tg_bot_token || ''
    form.value.tg_chat_id = map.tg_chat_id || ''
    form.value.wechat_webhook = map.wechat_webhook || ''
  } catch (e) {
    console.error('加载通知设置失败', e)
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    await updateSettings({
      tg_bot_token: form.value.tg_bot_token,
      tg_chat_id: form.value.tg_chat_id,
      wechat_webhook: form.value.wechat_webhook,
    })
    ElMessage.success('通知设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleTest(channel: 'telegram' | 'wechat') {
  if (channel === 'telegram') testingTg.value = true
  else testingWechat.value = true
  try {
    await testNotify(channel)
    ElMessage.success(`${channel === 'telegram' ? 'Telegram' : '企业微信'} 测试消息发送成功`)
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '测试失败，请先点击"保存通知设置"再测试')
  } finally {
    testingTg.value = false
    testingWechat.value = false
  }
}

onMounted(fetchState)
</script>
