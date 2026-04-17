<template>
  <div class="space-y-4">
    <!-- 修改用户名 -->
    <el-card shadow="hover">
      <template #header><span class="font-bold">修改用户名</span></template>
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

    <!-- 修改密码 -->
    <el-card shadow="hover">
      <template #header><span class="font-bold">修改密码</span></template>
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

    <!-- 二次验证 -->
    <el-card shadow="hover">
      <template #header><span class="font-bold">二次验证</span></template>
      <div v-if="totpLoading" v-loading="true" style="min-height: 80px" />
      <div v-else>
        <div v-if="totpEnabled">
          <el-tag type="success" size="large" style="margin-bottom: 16px">已启用</el-tag>
          <p style="color: #606266; margin-bottom: 16px">二次验证已开启，每次登录需要输入动态验证码。</p>
          <el-button type="danger" @click="handleDisable2FA">关闭二次验证</el-button>
        </div>
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
        <el-input v-model="setupCode" placeholder="000000" maxlength="6"
          style="width: 200px; margin-bottom: 16px" class="totp-input" />
      </div>
      <template #footer>
        <el-button @click="setupDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="enabling2FA" @click="handleEnable2FA">确认启用</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  changePassword, changeUsername,
  get2FAStatus, setup2FA, enable2FA, disable2FA,
} from '../../api/auth'

const savingUsername = ref(false)
const savingPassword = ref(false)
const totpLoading = ref(false)
const totpEnabled = ref(false)
const setupDialogVisible = ref(false)
const totpSecret = ref('')
const qrImageUrl = ref('')
const setupCode = ref('')
const enabling2FA = ref(false)

const usernameForm = ref({ newUsername: '', password: '' })
const passwordForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })

async function handleChangeUsername() {
  if (!usernameForm.value.newUsername) { ElMessage.warning('请输入新用户名'); return }
  if (!usernameForm.value.password) { ElMessage.warning('请输入当前密码'); return }
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

async function handleChangePassword() {
  if (!passwordForm.value.oldPassword) { ElMessage.warning('请输入旧密码'); return }
  if (!passwordForm.value.newPassword) { ElMessage.warning('请输入新密码'); return }
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    ElMessage.warning('两次输入的密码不一致'); return
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

async function fetch2FAStatus() {
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

async function handleSetup2FA() {
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

async function handleEnable2FA() {
  if (setupCode.value.length !== 6) { ElMessage.warning('请输入 6 位验证码'); return }
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

async function handleDisable2FA() {
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
    if (e === 'cancel' || e?.toString?.().includes('cancel')) return
    ElMessage.error(e.response?.data?.error || '关闭失败')
  }
}

function copySecret() {
  const text = totpSecret.value
  try {
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => {
        ElMessage.success('密钥已复制到剪贴板')
      })
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
      ElMessage.success('密钥已复制到剪贴板')
    }
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

onMounted(fetch2FAStatus)
</script>

<style scoped>
.totp-input :deep(.el-input__inner) {
  text-align: center;
  font-size: 20px;
  letter-spacing: 6px;
}
</style>
