<template>
  <div class="s-blocks">
    <!-- Username -->
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">用户名</h3>
        <p class="s-block__hint">需要再次输入当前密码以确认操作。</p>
      </div>
      <Field label="新用户名" layout="row">
        <Input v-model="usernameForm.newUsername" placeholder="新用户名" />
      </Field>
      <Field label="当前密码" layout="row">
        <Input v-model="usernameForm.password" type="password" />
      </Field>
      <div class="s-actions">
        <Button variant="primary" :loading="savingUsername" @click="handleChangeUsername">修改用户名</Button>
      </div>
    </section>

    <!-- Password -->
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">密码</h3>
        <p class="s-block__hint">建议使用至少 12 位的随机密码。</p>
      </div>
      <Field label="旧密码" layout="row"><Input v-model="passwordForm.oldPassword" type="password" /></Field>
      <Field label="新密码" layout="row"><Input v-model="passwordForm.newPassword" type="password" /></Field>
      <Field label="确认新密码" layout="row">
        <Input v-model="passwordForm.confirmPassword" type="password" @keyup.enter="handleChangePassword" />
      </Field>
      <div class="s-actions">
        <Button variant="primary" :loading="savingPassword" @click="handleChangePassword">修改密码</Button>
      </div>
    </section>

    <!-- 2FA -->
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">两步验证</h3>
        <p class="s-block__hint">启用后，每次登录需要输入验证器 App 中的 6 位动态验证码。</p>
      </div>
      <div v-if="totpLoading" class="totp-skel">加载中…</div>
      <div v-else class="totp-state">
        <StatusDot :state="totpEnabled ? 'ok' : 'off'">{{ totpEnabled ? '已启用' : '未启用' }}</StatusDot>
        <div class="s-actions">
          <Button v-if="totpEnabled" variant="danger" @click="handleDisable2FA">关闭两步验证</Button>
          <Button v-else variant="primary" @click="handleSetup2FA">启用两步验证</Button>
        </div>
      </div>
    </section>

    <!-- Pre-2FA password confirm -->
    <Modal v-model:open="setupAuthDialogVisible" title="验证当前密码" :width="420">
      <p class="dialog-lead">为防止令牌被盗后静默绑定，请再次输入当前密码。</p>
      <Input v-model="setupPassword" type="password" placeholder="当前密码" />
      <template #footer>
        <Button @click="setupAuthDialogVisible = false">取消</Button>
        <Button variant="primary" :loading="fetchingSetup" @click="confirmSetup2FA">下一步</Button>
      </template>
    </Modal>

    <!-- 2FA setup -->
    <Modal v-model:open="setupDialogVisible" title="设置两步验证" :width="480">
      <div class="totp-setup">
        <p class="dialog-lead">使用验证器 App（如 1Password、Authy、Google Authenticator）扫描下方二维码。</p>
        <img :src="qrImageUrl" alt="QR Code" class="totp-setup__qr" />
        <p class="totp-setup__caption">无法扫码？复制下方密钥手动添加。</p>
        <div class="totp-secret-row">
          <Input :model-value="totpSecret" readonly class="totp-secret" />
          <Button @click="copySecret">复制</Button>
        </div>
        <hr class="divider-h" />
        <p class="dialog-lead">输入 App 中显示的 6 位代码确认绑定。</p>
        <Input v-model="enablePassword" type="password" placeholder="再次输入当前密码" />
        <Input v-model="setupCode" placeholder="000000" inputmode="numeric" :maxlength="6" class="totp-code" />
      </div>
      <template #footer>
        <Button @click="setupDialogVisible = false">取消</Button>
        <Button variant="primary" :loading="enabling2FA" @click="handleEnable2FA">确认启用</Button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Button, Input, Modal, Field, StatusDot, toast, confirm } from '../../ui'
import {
  changePassword, changeUsername,
  get2FAStatus, setup2FA, enable2FA, disable2FA,
} from '../../api/auth'

const savingUsername = ref(false)
const savingPassword = ref(false)
const totpLoading = ref(false)
const totpEnabled = ref(false)
const setupDialogVisible = ref(false)
const setupAuthDialogVisible = ref(false)
const setupPassword = ref('')
const enablePassword = ref('')
const fetchingSetup = ref(false)
const totpSecret = ref('')
const qrImageUrl = ref('')
const setupCode = ref('')
const enabling2FA = ref(false)

const usernameForm = ref({ newUsername: '', password: '' })
const passwordForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })

async function handleChangeUsername() {
  if (!usernameForm.value.newUsername) { toast.warn('请输入新用户名'); return }
  if (!usernameForm.value.password) { toast.warn('请输入当前密码'); return }
  savingUsername.value = true
  try {
    await changeUsername(usernameForm.value.password, usernameForm.value.newUsername)
    toast.success('用户名修改成功')
    usernameForm.value.newUsername = ''
    usernameForm.value.password = ''
  } catch (e: any) {
    toast.error(e.response?.data?.error || '修改用户名失败')
  } finally {
    savingUsername.value = false
  }
}

async function handleChangePassword() {
  if (!passwordForm.value.oldPassword) { toast.warn('请输入旧密码'); return }
  if (!passwordForm.value.newPassword) { toast.warn('请输入新密码'); return }
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    toast.warn('两次输入的密码不一致'); return
  }
  savingPassword.value = true
  try {
    await changePassword(passwordForm.value.oldPassword, passwordForm.value.newPassword)
    toast.success('密码修改成功')
    passwordForm.value.oldPassword = ''
    passwordForm.value.newPassword = ''
    passwordForm.value.confirmPassword = ''
  } catch (e: any) {
    toast.error(e.response?.data?.error || '修改密码失败')
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
    console.error('获取两步验证状态失败', e)
  } finally {
    totpLoading.value = false
  }
}

function handleSetup2FA() {
  setupPassword.value = ''
  setupAuthDialogVisible.value = true
}

async function confirmSetup2FA() {
  if (!setupPassword.value) { toast.warn('请输入当前密码'); return }
  fetchingSetup.value = true
  try {
    const { data } = await setup2FA(setupPassword.value)
    totpSecret.value = data.secret
    qrImageUrl.value = `https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(data.qr_url)}`
    setupCode.value = ''
    enablePassword.value = setupPassword.value
    setupAuthDialogVisible.value = false
    setupDialogVisible.value = true
  } catch (e: any) {
    toast.error(e.response?.data?.error || '获取两步验证配置失败')
  } finally {
    fetchingSetup.value = false
  }
}

async function handleEnable2FA() {
  if (!enablePassword.value) { toast.warn('请输入当前密码'); return }
  if (setupCode.value.length !== 6) { toast.warn('请输入 6 位验证码'); return }
  enabling2FA.value = true
  try {
    await enable2FA(enablePassword.value, setupCode.value)
    toast.success('两步验证已启用')
    setupDialogVisible.value = false
    totpEnabled.value = true
  } catch (e: any) {
    toast.error(e.response?.data?.error || '启用失败，请检查密码或验证码')
  } finally {
    enabling2FA.value = false
  }
}

async function handleDisable2FA() {
  try {
    const password = await confirm({
      title: '关闭两步验证',
      message: '请输入当前密码以关闭。',
      prompt: true,
      inputType: 'password',
      inputPlaceholder: '当前密码',
      tone: 'danger',
      confirmText: '关闭',
    })
    if (typeof password !== 'string' || !password) return
    await disable2FA(password)
    toast.success('两步验证已关闭')
    totpEnabled.value = false
  } catch (e: any) {
    if (e === 'cancel') return
    toast.error(e.response?.data?.error || '关闭失败')
  }
}

function copySecret() {
  const text = totpSecret.value
  try {
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => toast.success('密钥已复制'))
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
      toast.success('密钥已复制')
    }
  } catch { toast.warn('复制失败，请手动复制') }
}

onMounted(fetch2FAStatus)
</script>

<style scoped>
.totp-skel { font-size: 13px; color: var(--color-ink-muted); padding: 8px 0; }
.totp-state { display: flex; flex-direction: column; gap: 16px; align-items: flex-start; }
.dialog-lead { font-size: 13px; color: var(--color-ink-muted); margin: 0 0 12px; }

.totp-setup { display: flex; flex-direction: column; gap: 12px; }
.totp-setup__qr {
  width: 200px; height: 200px;
  align-self: center;
  border: 1px solid var(--color-ink-faint);
  border-radius: 8px;
  padding: 8px;
  background: white;
}
.totp-setup__caption { font-size: 12px; color: var(--color-ink-muted); margin: 0; text-align: center; }

.totp-secret-row { display: flex; gap: 8px; align-items: center; }
.totp-secret :deep(.input__field) { font-family: var(--font-mono); font-size: 13px; }

.totp-code :deep(.input__field) {
  text-align: center;
  font-family: var(--font-mono);
  font-size: 22px;
  letter-spacing: 8px;
}
.totp-code.input { height: 48px; }
</style>
