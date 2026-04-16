<template>
  <div class="login-wrapper">
    <el-card class="login-card" shadow="always">
      <div class="login-header">
        <h1 class="login-title">ProxyPanel</h1>
        <p class="login-subtitle">{{ phase === 'login' ? '管理面板登录' : '请输入验证码' }}</p>
      </div>

      <!-- 阶段1: 用户名 + 密码 -->
      <el-form
        v-if="phase === 'login'"
        ref="formRef"
        :model="form"
        :rules="rules"
        @submit.prevent="handleLogin"
      >
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            placeholder="用户名"
            size="large"
            :prefix-icon="User"
          />
        </el-form-item>
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            size="large"
            show-password
            :prefix-icon="Lock"
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="loading"
            class="login-btn"
            @click="handleLogin"
          >
            登 录
          </el-button>
        </el-form-item>
      </el-form>

      <!-- 阶段2: TOTP 验证码 -->
      <div v-else class="totp-phase">
        <p class="totp-hint">请输入验证器 App 中的 6 位动态验证码</p>
        <el-input
          v-model="totpCode"
          placeholder="000000"
          size="large"
          maxlength="6"
          class="totp-input"
          @keyup.enter="handleVerify2FA"
        />
        <el-button
          type="primary"
          size="large"
          :loading="loading"
          class="login-btn"
          style="margin-top: 16px"
          @click="handleVerify2FA"
        >
          验 证
        </el-button>
        <div class="totp-back">
          <el-link type="primary" @click="backToLogin">返回登录</el-link>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { login, verify2FA } from '../api/auth'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const formRef = ref<FormInstance>()
const loading = ref(false)

// 登录阶段: 'login' | 'totp'
const phase = ref<'login' | 'totp'>('login')
const tempToken = ref('')
const totpCode = ref('')

const form = reactive({
  username: '',
  password: '',
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function handleLogin() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    const res = await login(form.username, form.password)
    if (res.data.require_2fa) {
      // 需要二次验证
      tempToken.value = res.data.temp_token
      phase.value = 'totp'
      totpCode.value = ''
    } else {
      // 直接登录成功
      auth.setToken(res.data.token)
      ElMessage.success('登录成功')
      router.push('/')
    }
  } catch (err: any) {
    const msg = err.response?.data?.error || '登录失败，请检查用户名和密码'
    ElMessage.error(msg)
  } finally {
    loading.value = false
  }
}

async function handleVerify2FA() {
  if (totpCode.value.length !== 6) {
    ElMessage.warning('请输入 6 位验证码')
    return
  }
  loading.value = true
  try {
    const res = await verify2FA(tempToken.value, totpCode.value)
    auth.setToken(res.data.token)
    ElMessage.success('登录成功')
    router.push('/')
  } catch (err: any) {
    const msg = err.response?.data?.error || '验证码错误，请重试'
    ElMessage.error(msg)
  } finally {
    loading.value = false
  }
}

function backToLogin() {
  phase.value = 'login'
  tempToken.value = ''
  totpCode.value = ''
}
</script>

<style scoped>
.login-wrapper {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 400px;
  padding: 20px;
  border-radius: 12px;
}

.login-header {
  text-align: center;
  margin-bottom: 30px;
}

.login-title {
  font-size: 28px;
  font-weight: 700;
  color: #303133;
  margin: 0 0 8px 0;
}

.login-subtitle {
  font-size: 14px;
  color: #909399;
  margin: 0;
}

.login-btn {
  width: 100%;
}

.totp-phase {
  text-align: center;
}

.totp-hint {
  font-size: 14px;
  color: #606266;
  margin-bottom: 20px;
}

.totp-input :deep(.el-input__inner) {
  text-align: center;
  font-size: 24px;
  letter-spacing: 8px;
}

.totp-back {
  margin-top: 16px;
}
</style>
