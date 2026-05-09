<template>
  <div class="login">
    <aside class="login__brand">
      <div class="login__brand-inner">
        <span class="login__mark">P</span>
        <h1 class="login__title">ProxyPanel</h1>
        <p class="login__lede">
          为自己和小团队管理代理与流量。
          <br />
          一个克制的运维面板。
        </p>
        <ul class="login__bullets">
          <li><StatusDot state="ok" /> 多端订阅 · Surge / Clash / Sing-box</li>
          <li><StatusDot state="ok" /> 流量周期与服务器配额预警</li>
          <li><StatusDot state="ok" /> Telegram · 企业微信 告警</li>
        </ul>
      </div>
    </aside>

    <main class="login__form">
      <div class="login__form-inner">
        <p class="eyebrow">{{ phase === 'login' ? '登录管理面板' : '两步验证' }}</p>
        <h2 class="login__heading">{{ phase === 'login' ? '继续' : '请输入验证码' }}</h2>

        <form v-if="phase === 'login'" class="login__fields" @submit.prevent="handleLogin">
          <Field label="用户名" :error="errors.username">
            <template #default="{ id }">
              <Input
                :id="id"
                v-model="form.username"
                placeholder="admin"
                autocomplete="username"
              />
            </template>
          </Field>
          <Field label="密码" :error="errors.password">
            <template #default="{ id }">
              <Input
                :id="id"
                v-model="form.password"
                type="password"
                autocomplete="current-password"
                @keyup.enter="handleLogin"
              />
            </template>
          </Field>
          <Button type="submit" variant="primary" :loading="loading" class="login__submit">登录</Button>
        </form>

        <div v-else class="login__totp">
          <p class="login__hint">在你的验证器 App 中找到当前 6 位代码。</p>
          <Input
            v-model="totpCode"
            placeholder="000000"
            inputmode="numeric"
            :maxlength="6"
            class="login__totp-input"
            autocomplete="one-time-code"
            name="otp"
            @keyup.enter="handleVerify2FA"
          />
          <Button variant="primary" :loading="loading" class="login__submit" @click="handleVerify2FA">验证</Button>
          <button class="login__link" type="button" @click="backToLogin">← 返回登录</button>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { Button, Input, Field, StatusDot, toast } from '../ui'
import { login, verify2FA } from '../api/auth'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)

const phase = ref<'login' | 'totp'>('login')
const tempToken = ref('')
const totpCode = ref('')

const form = reactive({ username: '', password: '' })
const errors = reactive<{ username?: string; password?: string }>({})

function validate() {
  errors.username = form.username ? '' : '请输入用户名'
  errors.password = form.password ? '' : '请输入密码'
  return !errors.username && !errors.password
}

async function handleLogin() {
  if (!validate()) return
  loading.value = true
  try {
    const res = await login(form.username, form.password)
    if (res.data.require_2fa) {
      tempToken.value = res.data.temp_token
      phase.value = 'totp'
      totpCode.value = ''
    } else {
      auth.setToken(res.data.token)
      toast.success('登录成功')
      router.push('/')
    }
  } catch (err: any) {
    toast.error(err.response?.data?.error || '登录失败，请检查用户名和密码')
  } finally {
    loading.value = false
  }
}

async function handleVerify2FA() {
  if (totpCode.value.length !== 6) {
    toast.warn('请输入 6 位验证码')
    return
  }
  loading.value = true
  try {
    const res = await verify2FA(tempToken.value, totpCode.value)
    auth.setToken(res.data.token)
    toast.success('登录成功')
    router.push('/')
  } catch (err: any) {
    toast.error(err.response?.data?.error || '验证码错误，请重试')
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
.login {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 1fr 1fr;
  background: var(--color-surface-base);
}

.login__brand {
  background: var(--color-surface-raised);
  border-right: 1px solid var(--color-ink-faint);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: 64px;
}
.login__brand-inner { max-width: 420px; }
.login__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 10px;
  background: var(--color-accent);
  color: white;
  font-family: var(--font-serif);
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -0.02em;
  margin-bottom: 28px;
}
.login__title {
  font-family: var(--font-serif);
  font-size: 44px;
  line-height: 1.1;
  font-weight: 600;
  letter-spacing: -0.02em;
  color: var(--color-ink-strong);
  margin: 0 0 16px;
}
.login__lede {
  font-family: var(--font-serif);
  font-size: 18px;
  line-height: 1.6;
  color: var(--color-ink-base);
  margin: 0 0 28px;
  max-width: 30ch;
}
.login__bullets {
  list-style: none; margin: 0; padding: 0;
  display: flex; flex-direction: column; gap: 10px;
  font-size: 13px; color: var(--color-ink-muted);
}
.login__bullets li { display: flex; align-items: center; gap: 8px; }

.login__form {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  padding: 64px;
}
.login__form-inner { width: 100%; max-width: 360px; }
.login__heading {
  font-family: var(--font-serif);
  font-size: 32px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--color-ink-strong);
  margin: 6px 0 32px;
}

.login__fields { display: flex; flex-direction: column; gap: 16px; }
.login__submit {
  width: 100%;
  height: 44px;
  margin-top: 8px;
  font-size: 14px;
  letter-spacing: 0.04em;
}

.login__totp { display: flex; flex-direction: column; gap: 16px; }
.login__hint { font-size: 13px; color: var(--color-ink-muted); margin: 0; }
.login__totp-input.input { height: 56px; }
.login__totp-input :deep(.input__field) {
  text-align: center;
  font-family: var(--font-mono);
  font-size: 28px;
  letter-spacing: 12px;
}

.login__link {
  background: none; border: 0; padding: 0;
  color: var(--color-ink-muted);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  transition: color 150ms var(--ease-out);
}
.login__link:hover { color: var(--color-accent-ink); }

@media (max-width: 900px) {
  .login { grid-template-columns: 1fr; }
  .login__brand { display: none; }
  .login__form { padding: 40px 24px; justify-content: center; }
}
</style>
