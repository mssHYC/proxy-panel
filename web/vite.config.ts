import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// 体积大头：echarts ~1MB（仅 Traffic / Dashboard 用）、element-plus ~500kB、
// vue 生态 + vuedraggable ~250kB、qrcode ~30kB（仅 SubscriptionDialog 用）。
//
// 优化策略：
// 1. element-plus 改用 auto-import：unplugin-vue-components 在编译期识别
//    `<el-xxx>` 模板，只把用到的组件打进包；ElMessage/ElMessageBox 等命令式
//    API 由 unplugin-auto-import 接管。原 main.ts 的全量 `app.use(ElementPlus)`
//    被移除，1.1MB → ~300kB 量级。
// 2. echarts / qrcode / vue 生态分别拆 vendor chunk，命中浏览器缓存跨页复用。
// 3. Dashboard 的 TrafficChart 与 Users 的 SubscriptionDialog 改 defineAsyncComponent
//    懒加载，echarts / qrcode 只在用到时才下载。
function manualChunks(id: string) {
  if (!id.includes('node_modules')) return
  if (id.includes('echarts') || id.includes('zrender')) {
    return 'vendor-echarts'
  }
  if (id.includes('element-plus') || id.includes('@element-plus')) {
    return 'vendor-element'
  }
  if (id.includes('qrcode') || id.includes('dijkstrajs') || id.includes('pngjs')) {
    return 'vendor-qrcode'
  }
  if (
    id.includes('/vue/') ||
    id.includes('/vue-router/') ||
    id.includes('/@vue/') ||
    id.includes('/pinia/') ||
    id.includes('/vuedraggable/') ||
    id.includes('/sortablejs/')
  ) {
    return 'vendor-vue'
  }
}

export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    AutoImport({
      // ElMessage / ElMessageBox / ElNotification / ElLoading 等命令式 API 自动导入
      resolvers: [ElementPlusResolver()],
      dts: 'src/auto-imports.d.ts',
    }),
    Components({
      // <el-xxx> 模板组件按需注册
      resolvers: [ElementPlusResolver()],
      dts: 'src/components.d.ts',
    }),
  ],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
  build: {
    rollupOptions: {
      output: { manualChunks },
    },
    // 阈值放宽到 700kB：vendor-element / vendor-echarts 即便已按需还是接近这个量级，
    // 但属于稳定 vendor chunk，可被浏览器缓存跨页/跨版本复用，不是"关键业务 chunk"。
    chunkSizeWarningLimit: 700,
  },
})
