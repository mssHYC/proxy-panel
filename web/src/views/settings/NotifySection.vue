<template>
  <div :class="['s-blocks', { 'is-loading-overlay': loading }]">
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">Telegram</h3>
        <p class="s-block__hint">通过官方 Bot API 推送告警。先填 Token / Chat ID，再点测试。</p>
      </div>
      <Field label="Bot Token" hint="从 @BotFather 获取" layout="row">
        <Input v-model="form.tg_bot_token" type="password" placeholder="1234567:ABC..." />
      </Field>
      <Field label="Chat ID" hint="用户或群组 ID" layout="row">
        <Input v-model="form.tg_chat_id" placeholder="例如 123456789 或 -100..." />
      </Field>
      <div class="s-actions">
        <Button :loading="testingTg" @click="handleTest('telegram')">测试连接</Button>
      </div>
    </section>

    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">企业微信</h3>
        <p class="s-block__hint">在群机器人中创建 Webhook，将完整 URL 粘贴在下方。</p>
      </div>
      <Field label="Webhook URL" layout="row">
        <Input v-model="form.wechat_webhook" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
      </Field>
      <div class="s-actions">
        <Button :loading="testingWechat" @click="handleTest('wechat')">测试连接</Button>
      </div>
    </section>

    <div class="s-actions">
      <Button variant="primary" :loading="saving" @click="handleSave">保存通知设置</Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Button, Input, Field, toast } from '../../ui'
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
    if (Array.isArray(data)) data.forEach((item: any) => { map[item.key] = item.value })
    else if (data.settings) {
      if (Array.isArray(data.settings)) data.settings.forEach((item: any) => { map[item.key] = item.value })
      else Object.assign(map, data.settings)
    } else { Object.assign(map, data) }
    form.value.tg_bot_token = map.tg_bot_token || ''
    form.value.tg_chat_id = map.tg_chat_id || ''
    form.value.wechat_webhook = map.wechat_webhook || ''
  } catch (e) { console.error('加载通知设置失败', e) }
  finally { loading.value = false }
}

async function handleSave() {
  saving.value = true
  try {
    await updateSettings({
      tg_bot_token: form.value.tg_bot_token,
      tg_chat_id: form.value.tg_chat_id,
      wechat_webhook: form.value.wechat_webhook,
    })
    toast.success('通知设置已保存')
  } catch (e: any) {
    toast.error(e.response?.data?.error || '保存失败')
  } finally { saving.value = false }
}

async function handleTest(channel: 'telegram' | 'wechat') {
  if (channel === 'telegram') testingTg.value = true
  else testingWechat.value = true
  try {
    await testNotify(channel)
    toast.success(`${channel === 'telegram' ? 'Telegram' : '企业微信'} 测试消息发送成功`)
  } catch (e: any) {
    toast.error(e.response?.data?.error || '测试失败，请先点击「保存通知设置」再测试')
  } finally {
    testingTg.value = false
    testingWechat.value = false
  }
}

onMounted(fetchState)
</script>
