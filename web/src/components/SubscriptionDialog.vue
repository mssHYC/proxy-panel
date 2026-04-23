<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="订阅管理"
    width="640px"
    destroy-on-close
    @open="onOpen"
  >
    <div class="mb-4 text-sm text-gray-500">
      用户: <span class="font-medium text-gray-700">{{ user?.username }}</span>
    </div>

    <el-tabs v-model="mainTab">
      <!-- ===== Tab 1: 订阅链接 ===== -->
      <el-tab-pane label="订阅链接" name="links">
        <!-- Token 选择器 -->
        <div v-if="tokens.length > 0" class="mb-4">
          <el-select
            v-model="selectedTokenId"
            placeholder="选择 Token"
            class="w-full"
            @change="onTokenChange"
          >
            <el-option
              v-for="t in tokens"
              :key="t.id"
              :value="t.id"
              :label="tokenOptionLabel(t)"
            >
              <span :class="tokenOptionClass(t)">{{ t.name }}</span>
              <el-tag v-if="!t.enabled" size="small" type="danger" class="ml-2">已禁用</el-tag>
              <el-tag v-else-if="isExpired(t)" size="small" type="warning" class="ml-2">已过期</el-tag>
            </el-option>
          </el-select>
        </div>

        <!-- 无 Token 时的提示 -->
        <div v-if="tokens.length === 0" class="mb-3">
          <el-alert
            title="该用户暂无 Token，可切到『Token 管理』新建"
            type="info"
            :closable="false"
            show-icon
          />
          <div class="mt-2 text-xs text-orange-500 font-medium">旧版链接（将废弃）</div>
        </div>

        <!-- 格式 Tabs -->
        <el-tabs v-model="activeFormat">
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

              <div class="flex flex-col items-center gap-2">
                <canvas :ref="(el) => setCanvasRef(fmt.value, el as HTMLCanvasElement | null)" />
                <div class="text-xs text-gray-400">手机客户端扫码导入</div>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </el-tab-pane>

      <!-- ===== Tab 2: Token 管理 ===== -->
      <el-tab-pane label="Token 管理" name="manage">
        <div class="flex justify-end mb-3">
          <el-button type="primary" size="small" @click="openCreateDialog">
            新建 Token
          </el-button>
        </div>

        <el-table :data="tokens" v-loading="tokensLoading" border size="small">
          <!-- 名称 -->
          <el-table-column label="名称" min-width="120">
            <template #default="{ row }">
              <span>{{ row.name }}</span>
              <el-button
                link
                size="small"
                class="ml-1"
                @click="openEditDialog(row)"
                title="编辑"
              >
                <el-icon><Edit /></el-icon>
              </el-button>
            </template>
          </el-table-column>

          <!-- 启用 -->
          <el-table-column label="启用" width="70" align="center">
            <template #default="{ row }">
              <el-switch
                :model-value="row.enabled"
                @change="(val: boolean) => toggleEnabled(row, val)"
              />
            </template>
          </el-table-column>

          <!-- 过期时间 -->
          <el-table-column label="过期" width="130">
            <template #default="{ row }">
              <span :class="{ 'text-orange-500': isExpired(row) }">
                {{ row.expires_at ? formatDate(row.expires_at) : '永不' }}
              </span>
            </template>
          </el-table-column>

          <!-- IP 绑定 -->
          <el-table-column label="IP 绑定" min-width="140">
            <template #default="{ row }">
              <div class="flex flex-col gap-1">
                <el-switch
                  :model-value="row.ip_bind_enabled"
                  @change="(val: boolean) => toggleIpBind(row, val)"
                />
                <span v-if="row.bound_ip" class="text-xs text-gray-500 font-mono">
                  {{ row.bound_ip }}
                  <el-button link size="small" @click="resetBind(row)" class="ml-1 text-orange-500">
                    清除
                  </el-button>
                </span>
              </div>
            </template>
          </el-table-column>

          <!-- 最后使用 -->
          <el-table-column label="最后使用" width="100" align="center">
            <template #default="{ row }">
              <el-tooltip
                v-if="row.last_used_at"
                placement="top"
                :content="`IP: ${row.last_ip || '-'}\nUA: ${row.last_ua || '-'}\n时间: ${formatDate(row.last_used_at)}\n次数: ${row.use_count}`"
              >
                <el-tag size="small" type="info">{{ row.use_count }}次</el-tag>
              </el-tooltip>
              <span v-else class="text-gray-400 text-xs">从未</span>
            </template>
          </el-table-column>

          <!-- 操作 -->
          <el-table-column label="操作" width="120" align="center">
            <template #default="{ row }">
              <el-button
                link
                size="small"
                type="warning"
                @click="handleRotate(row)"
                title="轮换 Token"
              >轮换</el-button>
              <el-button
                link
                size="small"
                type="danger"
                @click="handleDelete(row)"
                title="删除"
              >删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 新建 Token 对话框 -->
    <el-dialog
      v-model="createDialogVisible"
      title="新建 Token"
      width="420px"
      append-to-body
    >
      <el-form :model="createForm" label-width="90px">
        <el-form-item label="名称" required>
          <el-input v-model="createForm.name" placeholder="例如：手机客户端" />
        </el-form-item>
        <el-form-item label="过期时间">
          <el-date-picker
            v-model="createForm.expires_at"
            type="datetime"
            placeholder="不填则永不过期"
            value-format="YYYY-MM-DDTHH:mm:ss"
            class="w-full"
          />
        </el-form-item>
        <el-form-item label="IP 绑定">
          <el-switch v-model="createForm.ip_bind_enabled" />
          <span class="ml-2 text-xs text-gray-400">首次使用后绑定 IP</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="createSubmitting" @click="handleCreate">确定</el-button>
      </template>
    </el-dialog>

    <!-- 编辑 Token 对话框 -->
    <el-dialog
      v-model="editDialogVisible"
      title="编辑 Token"
      width="420px"
      append-to-body
    >
      <el-form :model="editForm" label-width="90px">
        <el-form-item label="名称" required>
          <el-input v-model="editForm.name" />
        </el-form-item>
        <el-form-item label="过期时间">
          <div class="flex gap-2 items-center">
            <el-date-picker
              v-model="editForm.expires_at"
              type="datetime"
              placeholder="不填则永不过期"
              value-format="YYYY-MM-DDTHH:mm:ss"
              class="flex-1"
            />
            <el-button size="small" @click="editForm.expires_at = null">清除</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="editSubmitting" @click="handleEdit">确定</el-button>
      </template>
    </el-dialog>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import QRCode from 'qrcode'
import {
  listSubTokens,
  createSubToken,
  updateSubToken,
  rotateSubToken,
  deleteSubToken,
  type SubscriptionToken,
} from '../api/sub-tokens'

interface UserInfo {
  id: number
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

// ---- State ----
const mainTab = ref('links')
const activeFormat = ref('surge')
const tokens = ref<SubscriptionToken[]>([])
const tokensLoading = ref(false)
const selectedTokenId = ref<number | null>(null)

// Create dialog
const createDialogVisible = ref(false)
const createSubmitting = ref(false)
const createForm = ref({ name: '', expires_at: null as string | null, ip_bind_enabled: true })

// Edit dialog
const editDialogVisible = ref(false)
const editSubmitting = ref(false)
const editingToken = ref<SubscriptionToken | null>(null)
const editForm = ref({ name: '', expires_at: null as string | null })

// ---- Helpers ----
function isExpired(t: SubscriptionToken): boolean {
  if (!t.expires_at) return false
  return new Date(t.expires_at).getTime() < Date.now()
}

function formatDate(d: string | null): string {
  if (!d) return '-'
  return new Date(d).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

function tokenOptionLabel(t: SubscriptionToken): string {
  return t.name
}

function tokenOptionClass(t: SubscriptionToken): string {
  if (!t.enabled || isExpired(t)) return 'text-gray-400'
  return ''
}

// ---- Load tokens ----
async function loadTokens() {
  if (!props.user) return
  tokensLoading.value = true
  try {
    const res = await listSubTokens(props.user.id)
    tokens.value = (res.data as any)?.tokens ?? res.data ?? []
    // Auto-select first enabled/non-expired token
    const good = tokens.value.find(t => t.enabled && !isExpired(t))
    selectedTokenId.value = good?.id ?? tokens.value[0]?.id ?? null
  } catch {
    ElMessage.error('加载 Token 列表失败')
  } finally {
    tokensLoading.value = false
  }
}

function onOpen() {
  mainTab.value = 'links'
  activeFormat.value = 'surge'
  loadTokens()
}

// ---- Subscription URL ----
function getSelectedToken(): SubscriptionToken | null {
  if (!selectedTokenId.value) return null
  return tokens.value.find(t => t.id === selectedTokenId.value) ?? null
}

function getSubUrl(format: string): string {
  if (!props.user) return ''
  const tok = getSelectedToken()
  if (tok) {
    return `${window.location.origin}/api/sub/t/${tok.token}?format=${format}`
  }
  // Legacy fallback
  return `${window.location.origin}/api/sub/${props.user.uuid}?format=${format}`
}

async function copyUrl(format: string) {
  const url = getSubUrl(format)
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(url)
    } else {
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

// ---- QR codes ----
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

function renderAllQR() {
  for (const fmt of formats) {
    const canvas = canvasRefs[fmt.value]
    if (canvas) {
      renderQR(canvas, getSubUrl(fmt.value))
    }
  }
}

// Re-render when dialog opens or tokens/selection changes
watch(
  () => [props.visible, selectedTokenId.value],
  async () => {
    if (!props.visible) return
    await nextTick()
    setTimeout(() => renderAllQR(), 200)
  }
)

// Re-render on format tab switch
watch(activeFormat, async () => {
  if (!props.visible) return
  await nextTick()
  setTimeout(() => {
    const canvas = canvasRefs[activeFormat.value]
    if (canvas) renderQR(canvas, getSubUrl(activeFormat.value))
  }, 100)
})

function onTokenChange() {
  nextTick(() => setTimeout(() => renderAllQR(), 100))
}

// ---- Token mutations ----
async function toggleEnabled(row: SubscriptionToken, val: boolean) {
  try {
    await updateSubToken(row.id, { enabled: val })
    row.enabled = val
    ElMessage.success(val ? '已启用' : '已禁用')
  } catch {
    ElMessage.error('操作失败')
  }
}

async function toggleIpBind(row: SubscriptionToken, val: boolean) {
  try {
    await updateSubToken(row.id, { ip_bind_enabled: val })
    row.ip_bind_enabled = val
  } catch {
    ElMessage.error('操作失败')
  }
}

async function resetBind(row: SubscriptionToken) {
  try {
    const updated = await updateSubToken(row.id, { reset_bind: true })
    const data: any = updated.data
    row.bound_ip = (data?.token ?? data)?.bound_ip ?? ''
    ElMessage.success('IP 绑定已清除')
  } catch {
    ElMessage.error('操作失败')
  }
}

async function handleRotate(row: SubscriptionToken) {
  try {
    await ElMessageBox.confirm(`确认轮换「${row.name}」的 Token？轮换后旧订阅链接将失效。`, '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    const res = await rotateSubToken(row.id)
    const data: any = res.data
    const updated: SubscriptionToken = data?.token ?? data
    const idx = tokens.value.findIndex(t => t.id === row.id)
    if (idx !== -1) tokens.value[idx] = updated
    ElMessage.success('Token 已轮换')
  } catch {
    // cancelled or failed
  }
}

async function handleDelete(row: SubscriptionToken) {
  try {
    await ElMessageBox.confirm(`确认删除 Token「${row.name}」？`, '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await deleteSubToken(row.id)
    ElMessage.success('已删除')
    if (selectedTokenId.value === row.id) selectedTokenId.value = null
    await loadTokens()
  } catch {
    // cancelled or failed
  }
}

// ---- Create ----
function openCreateDialog() {
  createForm.value = { name: '', expires_at: null, ip_bind_enabled: true }
  createDialogVisible.value = true
}

async function handleCreate() {
  if (!props.user || !createForm.value.name.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  createSubmitting.value = true
  try {
    await createSubToken(props.user.id, {
      name: createForm.value.name.trim(),
      expires_at: createForm.value.expires_at || null,
      ip_bind_enabled: createForm.value.ip_bind_enabled,
    })
    createDialogVisible.value = false
    ElMessage.success('Token 已创建')
    await loadTokens()
  } catch {
    ElMessage.error('创建失败')
  } finally {
    createSubmitting.value = false
  }
}

// ---- Edit ----
function openEditDialog(row: SubscriptionToken) {
  editingToken.value = row
  editForm.value = { name: row.name, expires_at: row.expires_at }
  editDialogVisible.value = true
}

async function handleEdit() {
  if (!editingToken.value || !editForm.value.name.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  editSubmitting.value = true
  try {
    const payload: any = { name: editForm.value.name.trim() }
    if (editForm.value.expires_at) {
      payload.expires_at = editForm.value.expires_at
    } else {
      payload.expires_at_null = true
    }
    const res = await updateSubToken(editingToken.value.id, payload)
    const data: any = res.data
    const updated: SubscriptionToken = data?.token ?? data
    const idx = tokens.value.findIndex(t => t.id === editingToken.value!.id)
    if (idx !== -1) tokens.value[idx] = updated
    editDialogVisible.value = false
    ElMessage.success('已更新')
  } catch {
    ElMessage.error('更新失败')
  } finally {
    editSubmitting.value = false
  }
}
</script>
