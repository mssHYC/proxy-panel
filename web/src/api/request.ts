import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import router from '../router'

const request = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

request.interceptors.request.use((config) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  // 允许调用方显式关闭默认的全局错误提示：config.silent = true
  return config
})

// 全局错误提示：任何非 2xx 响应默认弹 ElMessage.error
// 响应体约定 { error: string, code?: string }；优先展示 error 字段
request.interceptors.response.use(
  (response) => response,
  (error) => {
    const cfg = error.config as any
    const status = error.response?.status

    if (status === 401) {
      const auth = useAuthStore()
      auth.logout()
      router.push('/login')
      // 401 页面会自己处理，不弹错
      return Promise.reject(error)
    }

    if (!cfg?.silent) {
      const data = error.response?.data
      const msg = (data && (data.error || data.message)) ||
        (status === 429 ? '请求过于频繁' : null) ||
        error.message ||
        '请求失败'
      ElMessage.error(msg)
    }
    return Promise.reject(error)
  }
)

export default request
