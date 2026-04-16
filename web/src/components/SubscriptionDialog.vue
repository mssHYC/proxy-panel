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

          <!-- 简易 QR 码展示区域 -->
          <div class="flex justify-center">
            <canvas :ref="(el) => setCanvasRef(fmt.value, el as HTMLCanvasElement | null)" />
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'

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

// ---- 简易 QR 码：使用 canvas 绘制 ----
const canvasRefs: Record<string, HTMLCanvasElement | null> = {}

function setCanvasRef(format: string, el: HTMLCanvasElement | null) {
  canvasRefs[format] = el
}

/**
 * 极简 QR 码生成器（基于数据矩阵可视化）
 * 由于项目没有 qrcode 库，此处将 URL 文本以像素化方式呈现，
 * 并提示用户直接复制链接使用。
 */
function drawUrlText(canvas: HTMLCanvasElement, url: string) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const padding = 16
  const fontSize = 11
  const lineHeight = 16
  const maxWidth = 320

  canvas.width = maxWidth + padding * 2
  ctx.font = `${fontSize}px monospace`

  // 自动换行
  const lines: string[] = []
  let current = ''
  for (const ch of url) {
    const test = current + ch
    if (ctx.measureText(test).width > maxWidth) {
      lines.push(current)
      current = ch
    } else {
      current = test
    }
  }
  if (current) lines.push(current)

  canvas.height = lines.length * lineHeight + padding * 2

  // 重绘背景
  ctx.fillStyle = '#f5f7fa'
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  ctx.font = `${fontSize}px monospace`
  ctx.fillStyle = '#303133'
  lines.forEach((line, i) => {
    ctx.fillText(line, padding, padding + (i + 1) * lineHeight)
  })
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
          drawUrlText(canvas, getSubUrl(fmt.value))
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
      drawUrlText(canvas, getSubUrl(activeTab.value))
    }
  }, 100)
})
</script>
