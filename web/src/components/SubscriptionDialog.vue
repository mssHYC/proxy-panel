<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="订阅链接"
    width="560px"
    destroy-on-close
  >
    <div class="mb-4 text-sm text-gray-500">
      用户: <span class="font-medium text-gray-700">{{ user?.username }}</span>
    </div>

    <el-tabs v-model="activeTab">
      <el-tab-pane
        v-for="fmt in formats"
        :key="fmt.value"
        :label="fmt.label"
        :name="fmt.value"
      >
        <div class="flex flex-col gap-3 py-2">
          <el-input
            :model-value="getSubUrl(fmt.value)"
            readonly
            class="font-mono text-sm"
          >
            <template #append>
              <el-button @click="copyUrl(fmt.value)">
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </template>
          </el-input>

          <!-- 二维码 -->
          <div class="flex flex-col items-center gap-2">
            <canvas :ref="(el) => setCanvasRef(fmt.value, el as HTMLCanvasElement | null)" />
            <div class="text-xs text-gray-400">手机客户端扫码导入</div>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import QRCode from 'qrcode'

interface UserInfo {
  uuid: string
  username: string
}

const props = defineProps<{
  visible: boolean
  user: UserInfo | null
}>()

defineEmits<{
  'update:visible': [value: boolean]
}>()

const formats = [
  { label: 'Surge', value: 'surge' },
  { label: 'Clash', value: 'clash' },
  { label: 'V2Ray', value: 'v2ray' },
  { label: 'Shadowrocket', value: 'shadowrocket' },
  { label: 'Sing-box', value: 'singbox' },
]

const activeTab = ref('surge')

function getSubUrl(format: string): string {
  if (!props.user) return ''
  return `${window.location.origin}/api/sub/${props.user.uuid}?format=${format}`
}

async function copyUrl(format: string) {
  const url = getSubUrl(format)
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(url)
    } else {
      // HTTP 环境下 fallback: 使用临时 textarea
      const ta = document.createElement('textarea')
      ta.value = url
      ta.style.position = 'fixed'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败，请手动复制')
  }
}

const canvasRefs: Record<string, HTMLCanvasElement | null> = {}

function setCanvasRef(format: string, el: HTMLCanvasElement | null) {
  canvasRefs[format] = el
}

async function renderQR(canvas: HTMLCanvasElement, url: string) {
  try {
    await QRCode.toCanvas(canvas, url, { width: 220, margin: 2 })
  } catch (e) {
    console.error('QR 码生成失败', e)
  }
}

watch(
  () => [props.visible, props.user],
  async () => {
    if (!props.visible || !props.user) return
    await nextTick()
    // 等待 DOM 渲染完毕后绘制当前 tab
    setTimeout(() => {
      for (const fmt of formats) {
        const canvas = canvasRefs[fmt.value]
        if (canvas) {
          renderQR(canvas, getSubUrl(fmt.value))
        }
      }
    }, 200)
  },
  { immediate: true }
)

// 切换 tab 时也重绘
watch(activeTab, async () => {
  if (!props.visible || !props.user) return
  await nextTick()
  setTimeout(() => {
    const canvas = canvasRefs[activeTab.value]
    if (canvas) {
      renderQR(canvas, getSubUrl(activeTab.value))
    }
  }, 100)
})
</script>
