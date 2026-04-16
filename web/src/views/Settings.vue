<template>
  <div v-loading="loading" class="p-4 space-y-4">
    <h2 class="text-xl font-bold">系统设置</h2>

    <!-- 账号管理: 修改用户名 -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">修改用户名</span>
      </template>
      <el-form label-width="120px">
        <el-form-item label="新用户名">
          <el-input v-model="usernameForm.newUsername" placeholder="请输入新用户名" />
        </el-form-item>
        <el-form-item label="当前密码">
          <el-input v-model="usernameForm.password" type="password" show-password placeholder="请输入当前密码" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="savingUsername" @click="handleChangeUsername">
            确认修改
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 账号管理: 修改密码 -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">修改密码</span>
      </template>
      <el-form label-width="120px">
        <el-form-item label="旧密码">
          <el-input v-model="passwordForm.oldPassword" type="password" show-password placeholder="请输入旧密码" />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="passwordForm.newPassword" type="password" show-password placeholder="请输入新密码" />
        </el-form-item>
        <el-form-item label="确认密码">
          <el-input v-model="passwordForm.confirmPassword" type="password" show-password placeholder="请再次输入新密码" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="savingPassword" @click="handleChangePassword">
            确认修改
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 账号管理: 二次验证 (TOTP) -->
    <el-card shadow="hover">
      <template #header>
        <span class="font-bold">二次验证</span>
      </template>
      <div v-if="totpLoading" v-loading="true" style="min-height: 80px" />
      <div v-else>
        <!-- 已启用 -->
        <div v-if="totpEnabled">
          <el-tag type="success" size="large" style="margin-bottom: 16px">已启用</el-tag>
          <p style="color: #606266; margin-bottom: 16px">二次验证已开启，每次登录需要输入动态验证码。</p>
          <el-button type="danger" @click="handleDisable2FA">关闭二次验证</el-button>
        </div>
        <!-- 未启用 -->
        <div v-else>
          <p style="color: #606266; margin-bottom: 16px">启用二次验证后，每次登录需要输入验证器 App 中的动态验证码</p>
          <el-button type="primary" @click="handleSetup2FA">启用二次验证</el-button>
        </div>
      </div>
    </el-card>

    <!-- 二次验证设置对话框 -->
    <el-dialog v-model="setupDialogVisible" title="设置二次验证" width="460px" :close-on-click-modal="false">
      <div style="text-align: center">
        <p style="margin-bottom: 16px; color: #606266">请使用验证器 App (如 Google Authenticator) 扫描下方二维码</p>
        <img :src="qrImageUrl" alt="QR Code" style="width: 200px; height: 200px; margin: 0 auto 16px" />
        <p style="margin-bottom: 8px; color: #909399; font-size: 13px">无法扫码？手动输入密钥：</p>
        <el-input :model-value="totpSecret" readonly style="margin-bottom: 20px">
          <template #append>
            <el-button @click="copySecret">复制</el-button>
          </template>
        </el-input>
        <el-divider />
        <p style="margin-bottom: 12px; color: #606266">输入验证器中显示的 6 位验证码以确认启用</p>
        <el-input
          v-model="setupCode"
          placeholder="000000"
          maxlength="6"
          style="width: 200px; margin-bottom: 16px"
          class="totp-input"
        />
      </div>
      <template #footer>
        <el-button @click="setupDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="enabling2FA" @click="handleEnable2FA">确认启用</el-button>
      </template>
    </el-dialog>

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

    <!-- 自定义规则 -->
    <el-card shadow="hover">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">自定义分流规则</span>
          <el-tag type="info" size="small">优先于默认规则执行</el-tag>
        </div>
      </template>
      <el-alert
        type="info"
        :closable="false"
        show-icon
        class="mb-4"
        description="每行一条规则，支持 Clash 格式。自定义规则会插入到默认规则之前，优先匹配。Surge 和 Sing-box 订阅也会同步应用。"
      />
      <el-input
        v-model="customRulesText"
        type="textarea"
        :rows="8"
        placeholder="示例:
DOMAIN-SUFFIX,example.com,全球代理
DOMAIN-KEYWORD,openai,OpenAI
IP-CIDR,1.2.3.0/24,本地直连,no-resolve
GEOSITE,category-porn,REJECT"
        style="font-family: monospace"
      />
      <div class="mt-2 text-xs text-gray-400">
        可用策略组：手动切换 / 自动选择 / 全球代理 / 流媒体 / Telegram / Google / YouTube / Netflix / Spotify / HBO / Bing / OpenAI / ClaudeAI / Disney / GitHub / 国内媒体 / 本地直连 / 漏网之鱼 / DIRECT / REJECT
      </div>
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
import { ElMessage, ElMessageBox } from 'element-plus'
import { getSettings, updateSettings } from '../api/setting'
import { testNotify } from '../api/notify'
import { changePassword, changeUsername, get2FAStatus, setup2FA, enable2FA, disable2FA } from '../api/auth'

const loading = ref(false)
const saving = ref(false)
const testingTg = ref(false)
const testingWechat = ref(false)

// 账号管理相关
const savingUsername = ref(false)
const savingPassword = ref(false)
const totpLoading = ref(false)
const totpEnabled = ref(false)
const setupDialogVisible = ref(false)
const totpSecret = ref('')
const qrImageUrl = ref('')
const setupCode = ref('')
const enabling2FA = ref(false)

const usernameForm = ref({
  newUsername: '',
  password: '',
})

const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const settings = ref({
  tg_bot_token: '',
  tg_chat_id: '',
  wechat_webhook: '',
})

const warnPercent = ref(80)
const collectInterval = ref(60)
const customRulesText = ref('')

// 修改用户名
const handleChangeUsername = async () => {
  if (!usernameForm.value.newUsername) {
    ElMessage.warning('请输入新用户名')
    return
  }
  if (!usernameForm.value.password) {
    ElMessage.warning('请输入当前密码')
    return
  }
  savingUsername.value = true
  try {
    await changeUsername(usernameForm.value.password, usernameForm.value.newUsername)
    ElMessage.success('用户名修改成功')
    usernameForm.value.newUsername = ''
    usernameForm.value.password = ''
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '修改用户名失败')
  } finally {
    savingUsername.value = false
  }
}

// 修改密码
const handleChangePassword = async () => {
  if (!passwordForm.value.oldPassword) {
    ElMessage.warning('请输入旧密码')
    return
  }
  if (!passwordForm.value.newPassword) {
    ElMessage.warning('请输入新密码')
    return
  }
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    ElMessage.warning('两次输入的密码不一致')
    return
  }
  savingPassword.value = true
  try {
    await changePassword(passwordForm.value.oldPassword, passwordForm.value.newPassword)
    ElMessage.success('密码修改成功')
    passwordForm.value.oldPassword = ''
    passwordForm.value.newPassword = ''
    passwordForm.value.confirmPassword = ''
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '修改密码失败')
  } finally {
    savingPassword.value = false
  }
}

// 获取二次验证状态
const fetch2FAStatus = async () => {
  totpLoading.value = true
  try {
    const { data } = await get2FAStatus()
    totpEnabled.value = data.enabled
  } catch (e) {
    console.error('获取二次验证状态失败', e)
  } finally {
    totpLoading.value = false
  }
}

// 设置二次验证
const handleSetup2FA = async () => {
  try {
    const { data } = await setup2FA()
    totpSecret.value = data.secret
    qrImageUrl.value = `https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(data.qr_url)}`
    setupCode.value = ''
    setupDialogVisible.value = true
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '获取二次验证配置失败')
  }
}

// 启用二次验证
const handleEnable2FA = async () => {
  if (setupCode.value.length !== 6) {
    ElMessage.warning('请输入 6 位验证码')
    return
  }
  enabling2FA.value = true
  try {
    await enable2FA(setupCode.value)
    ElMessage.success('二次验证已启用')
    setupDialogVisible.value = false
    totpEnabled.value = true
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '启用失败，请检查验证码')
  } finally {
    enabling2FA.value = false
  }
}

// 关闭二次验证
const handleDisable2FA = async () => {
  try {
    const { value: password } = await ElMessageBox.prompt('请输入密码以关闭二次验证', '关闭二次验证', {
      inputType: 'password',
      inputPlaceholder: '请输入当前密码',
      confirmButtonText: '确认关闭',
      cancelButtonText: '取消',
    })
    if (!password) return
    await disable2FA(password)
    ElMessage.success('二次验证已关闭')
    totpEnabled.value = false
  } catch (e: any) {
    // 用户取消不提示错误
    if (e === 'cancel' || e?.toString?.().includes('cancel')) return
    ElMessage.error(e.response?.data?.error || '关闭失败')
  }
}

// 复制密钥
const copySecret = () => {
  navigator.clipboard.writeText(totpSecret.value).then(() => {
    ElMessage.success('密钥已复制到剪贴板')
  }).catch(() => {
    ElMessage.warning('复制失败，请手动复制')
  })
}

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
    customRulesText.value = map.custom_rules || ''
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
      custom_rules: customRulesText.value,
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
  fetch2FAStatus()
})
</script>

<style scoped>
.totp-input :deep(.el-input__inner) {
  text-align: center;
  font-size: 20px;
  letter-spacing: 6px;
}
</style>
