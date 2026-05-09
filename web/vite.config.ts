import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// vendor 拆分：echarts ~1MB（仅 Traffic / Dashboard 用）、reka-ui + vue-datepicker ~150kB、
// vue 生态 + vuedraggable ~250kB、qrcode ~30kB（仅 SubscriptionDialog 用）。
function manualChunks(id: string) {
  if (!id.includes('node_modules')) return
  if (id.includes('echarts') || id.includes('zrender')) return 'vendor-echarts'
  if (id.includes('reka-ui') || id.includes('@vuepic') || id.includes('vue-sonner')) return 'vendor-ui'
  if (id.includes('lucide-vue-next')) return 'vendor-icons'
  if (id.includes('qrcode') || id.includes('dijkstrajs') || id.includes('pngjs')) return 'vendor-qrcode'
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
  plugins: [vue(), tailwindcss()],
  server: { proxy: { '/api': 'http://localhost:8080' } },
  build: {
    rollupOptions: { output: { manualChunks } },
    chunkSizeWarningLimit: 700,
  },
})
