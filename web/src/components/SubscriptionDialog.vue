<template>
  <Modal :open="visible" :width="660" title="订阅管理" @update:open="(v) => emit('update:visible', v)">
    <p class="sub-user">
      <span class="eyebrow">用户</span>
      <span class="sub-user__name">{{ user?.username }}</span>
    </p>

    <Tabs
      :tabs="[{ label: '订阅链接', value: 'links' }, { label: 'Token 管理', value: 'manage' }]"
      :model-value="mainTab"
      variant="underline"
      @update:model-value="(v) => (mainTab = v as 'links' | 'manage')"
    />

    <!-- Tab: links -->
    <div v-if="mainTab === 'links'" class="sub-pane">
      <div v-if="tokens.length > 0" class="token-picker">
        <Select
          :model-value="selectedTokenId"
          :options="tokens.map(t => ({ label: tokenOptionLabel(t), value: t.id, disabled: !t.enabled || isExpired(t) }))"
          placeholder="选择 Token"
          @update:model-value="(v) => { selectedTokenId = (v as number); onTokenChange() }"
        />
      </div>

      <Alert v-if="tokens.length === 0" tone="info">
        该用户暂无 Token。可切到「Token 管理」新建。下方使用旧版 UUID 链接（将废弃）。
      </Alert>

      <Tabs
        :tabs="formats.map(f => ({ label: f.label, value: f.value }))"
        :model-value="activeFormat"
        variant="pill"
        class="sub-formats"
        @update:model-value="(v) => (activeFormat = v)"
      />

      <div v-for="fmt in formats" :key="fmt.value" v-show="activeFormat === fmt.value" class="format-pane">
        <div class="url-row">
          <Input :model-value="getSubUrl(fmt.value)" readonly class="url-input" />
          <Button @click="copyUrl(fmt.value)">
            <Copy :size="14" :stroke-width="1.6" /> 复制
          </Button>
        </div>
        <div class="qr">
          <canvas :ref="(el) => setCanvasRef(fmt.value, el as HTMLCanvasElement | null)" />
          <p class="qr__hint">手机客户端扫码导入</p>
        </div>
      </div>
    </div>

    <!-- Tab: manage -->
    <div v-else class="sub-pane">
      <div class="manage-bar">
        <Button variant="primary" @click="openCreateDialog">
          <Plus :size="14" :stroke-width="2" /> 新建 Token
        </Button>
      </div>

      <table class="dt dt--compact" v-if="tokens.length || tokensLoading">
        <thead>
          <tr>
            <th>名称</th>
            <th>启用</th>
            <th>过期</th>
            <th>IP 绑定</th>
            <th>最后使用</th>
            <th class="is-numeric">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in tokens" :key="row.id">
            <td>
              <span class="cell-name">{{ row.name }}</span>
              <button class="row-actions__btn inline-edit" @click="openEditDialog(row)" title="编辑">
                <Pencil :size="12" :stroke-width="1.6" />
              </button>
            </td>
            <td>
              <Switch :model-value="row.enabled" @update:model-value="(v) => toggleEnabled(row, v)" />
            </td>
            <td>
              <span class="num" :class="{ 'soon': isExpired(row) }">
                {{ row.expires_at ? formatDate(row.expires_at) : '永不' }}
              </span>
            </td>
            <td>
              <div class="ipbind">
                <Switch :model-value="row.ip_bind_enabled" @update:model-value="(v) => toggleIpBind(row, v)" />
                <span v-if="row.bound_ip" class="bound-ip mono">{{ row.bound_ip }}</span>
                <button v-if="row.bound_ip" class="link-btn" @click="resetBind(row)">清除</button>
              </div>
            </td>
            <td>
              <Tooltip v-if="row.last_used_at" :content="`IP: ${row.last_ip || '-'}\nUA: ${row.last_ua || '-'}\n时间: ${formatDate(row.last_used_at)}\n次数: ${row.use_count}`">
                <Tag :mono="true">{{ row.use_count }} 次</Tag>
              </Tooltip>
              <span v-else class="cell-none">从未</span>
            </td>
            <td class="is-numeric">
              <div class="row-actions">
                <button class="row-actions__btn" @click="handleRotate(row)" title="轮换 Token">
                  <RotateCcw :size="14" :stroke-width="1.6" />
                </button>
                <button class="row-actions__btn row-actions__btn--danger" @click="handleDelete(row)" title="删除">
                  <Trash2 :size="14" :stroke-width="1.6" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!tokensLoading && !tokens.length" class="empty-state">
        <p class="empty-state__title">还没有 Token</p>
        <p class="empty-state__hint">每个 Token 是一条独立可控的订阅链接，建议为不同设备创建独立 Token。</p>
        <Button variant="primary" @click="openCreateDialog">
          <Plus :size="14" :stroke-width="2" /> 创建第一个 Token
        </Button>
      </div>
    </div>
  </Modal>

  <!-- Create dialog -->
  <Modal v-model:open="createDialogVisible" title="新建 Token" :width="420">
    <Field label="名称" layout="row">
      <Input v-model="createForm.name" placeholder="例如：手机客户端" />
    </Field>
    <Field label="过期时间" hint="不填则永不过期" layout="row">
      <DateInput v-model="createForm.expires_at" enable-time format="yyyy-MM-dd HH:mm" model-type="yyyy-MM-ddTHH:mm:ss" />
    </Field>
    <Field label="IP 绑定" hint="首次使用后绑定 IP" layout="row">
      <Switch v-model="createForm.ip_bind_enabled" />
    </Field>
    <template #footer>
      <Button @click="createDialogVisible = false">取消</Button>
      <Button variant="primary" :loading="createSubmitting" @click="handleCreate">创建</Button>
    </template>
  </Modal>

  <!-- Edit dialog -->
  <Modal v-model:open="editDialogVisible" title="编辑 Token" :width="420">
    <Field label="名称" layout="row">
      <Input v-model="editForm.name" />
    </Field>
    <Field label="过期时间" hint="清空表示永不过期" layout="row">
      <DateInput v-model="editForm.expires_at" enable-time format="yyyy-MM-dd HH:mm" model-type="yyyy-MM-ddTHH:mm:ss" />
    </Field>
    <template #footer>
      <Button @click="editDialogVisible = false">取消</Button>
      <Button variant="primary" :loading="editSubmitting" @click="handleEdit">保存</Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { Plus, Pencil, Trash2, RotateCcw, Copy } from 'lucide-vue-next'
import QRCode from 'qrcode'
import {
  Modal, Tabs, Input, Button, Switch, Field, Tag, Tooltip, Alert, DateInput, Select,
  toast, confirm,
} from '../ui'
import {
  listSubTokens, createSubToken, updateSubToken, rotateSubToken, deleteSubToken,
  type SubscriptionToken,
} from '../api/sub-tokens'

interface UserInfo { id: number; uuid: string; username: string }

const props = defineProps<{ visible: boolean; user: UserInfo | null }>()
const emit = defineEmits<{ 'update:visible': [v: boolean] }>()

const formats = [
  { label: 'Surge', value: 'surge' },
  { label: 'Clash', value: 'clash' },
  { label: 'V2Ray', value: 'v2ray' },
  { label: 'Shadowrocket', value: 'shadowrocket' },
  { label: 'Sing-box', value: 'singbox' },
]

const mainTab = ref<'links' | 'manage'>('links')
const activeFormat = ref('surge')
const tokens = ref<SubscriptionToken[]>([])
const tokensLoading = ref(false)
const selectedTokenId = ref<number | null>(null)

const createDialogVisible = ref(false)
const createSubmitting = ref(false)
const createForm = ref({ name: '', expires_at: null as string | null, ip_bind_enabled: true })

const editDialogVisible = ref(false)
const editSubmitting = ref(false)
const editingToken = ref<SubscriptionToken | null>(null)
const editForm = ref({ name: '', expires_at: null as string | null })

function isExpired(t: SubscriptionToken): boolean {
  if (!t.expires_at) return false
  return new Date(t.expires_at).getTime() < Date.now()
}

function toRFC3339(local: string | null): string | null {
  if (!local) return null
  const d = new Date(local)
  return isNaN(d.getTime()) ? null : d.toISOString()
}

function fromRFC3339(iso: string | null): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (isNaN(d.getTime())) return null
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function formatDate(d: string | null): string {
  if (!d) return '-'
  return new Date(d).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

function tokenOptionLabel(t: SubscriptionToken): string {
  if (!t.enabled) return `${t.name}（已禁用）`
  if (isExpired(t)) return `${t.name}（已过期）`
  return t.name
}

async function loadTokens() {
  if (!props.user) return
  tokensLoading.value = true
  try {
    const res = await listSubTokens(props.user.id)
    tokens.value = (res.data as any)?.tokens ?? res.data ?? []
    const good = tokens.value.find(t => t.enabled && !isExpired(t))
    selectedTokenId.value = good?.id ?? tokens.value[0]?.id ?? null
  } catch { toast.error('加载 Token 列表失败') }
  finally { tokensLoading.value = false }
}

watch(() => props.visible, (v) => {
  if (v) {
    mainTab.value = 'links'
    activeFormat.value = 'surge'
    loadTokens()
  }
})

function getSelectedToken(): SubscriptionToken | null {
  if (!selectedTokenId.value) return null
  return tokens.value.find(t => t.id === selectedTokenId.value) ?? null
}

function getSubUrl(format: string): string {
  if (!props.user) return ''
  const tok = getSelectedToken()
  if (tok) return `${window.location.origin}/api/sub/t/${tok.token}?format=${format}`
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
    toast.success('已复制')
  } catch { toast.error('复制失败，请手动复制') }
}

const canvasRefs: Record<string, HTMLCanvasElement | null> = {}
function setCanvasRef(format: string, el: HTMLCanvasElement | null) {
  canvasRefs[format] = el
}

async function renderQR(canvas: HTMLCanvasElement, url: string) {
  try { await QRCode.toCanvas(canvas, url, { width: 200, margin: 2 }) }
  catch (e) { console.error('QR 码生成失败', e) }
}
function renderAllQR() {
  for (const fmt of formats) {
    const canvas = canvasRefs[fmt.value]
    if (canvas) renderQR(canvas, getSubUrl(fmt.value))
  }
}

watch(
  () => [props.visible, selectedTokenId.value, mainTab.value],
  async () => {
    if (!props.visible || mainTab.value !== 'links') return
    await nextTick()
    setTimeout(() => renderAllQR(), 200)
  },
)

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

async function toggleEnabled(row: SubscriptionToken, val: boolean) {
  try { await updateSubToken(row.id, { enabled: val }); row.enabled = val; toast.success(val ? '已启用' : '已禁用') }
  catch { toast.error('操作失败') }
}
async function toggleIpBind(row: SubscriptionToken, val: boolean) {
  try { await updateSubToken(row.id, { ip_bind_enabled: val }); row.ip_bind_enabled = val }
  catch { toast.error('操作失败') }
}
async function resetBind(row: SubscriptionToken) {
  try {
    const updated = await updateSubToken(row.id, { reset_bind: true })
    const data: any = updated.data
    row.bound_ip = (data?.token ?? data)?.bound_ip ?? ''
    toast.success('IP 绑定已清除')
  } catch { toast.error('操作失败') }
}

async function handleRotate(row: SubscriptionToken) {
  try {
    await confirm({
      title: '轮换 Token',
      message: `确认轮换「${row.name}」的 Token？轮换后旧订阅链接将失效。`,
      tone: 'danger',
      confirmText: '轮换',
    })
    const res = await rotateSubToken(row.id)
    const data: any = res.data
    const updated: SubscriptionToken = data?.token ?? data
    const idx = tokens.value.findIndex(t => t.id === row.id)
    if (idx !== -1) tokens.value[idx] = updated
    toast.success('Token 已轮换')
  } catch (e) { if (e === 'cancel') return }
}

async function handleDelete(row: SubscriptionToken) {
  try {
    await confirm({ title: '删除 Token', message: `确认删除 Token「${row.name}」？`, tone: 'danger', confirmText: '删除' })
    await deleteSubToken(row.id)
    toast.success('已删除')
    if (selectedTokenId.value === row.id) selectedTokenId.value = null
    await loadTokens()
  } catch (e) { if (e === 'cancel') return }
}

function openCreateDialog() {
  createForm.value = { name: '', expires_at: null, ip_bind_enabled: true }
  createDialogVisible.value = true
}

async function handleCreate() {
  if (!props.user || !createForm.value.name.trim()) { toast.warn('请输入名称'); return }
  createSubmitting.value = true
  try {
    await createSubToken(props.user.id, {
      name: createForm.value.name.trim(),
      expires_at: toRFC3339(createForm.value.expires_at),
      ip_bind_enabled: createForm.value.ip_bind_enabled,
    })
    createDialogVisible.value = false
    toast.success('Token 已创建')
    await loadTokens()
  } catch { toast.error('创建失败') }
  finally { createSubmitting.value = false }
}

function openEditDialog(row: SubscriptionToken) {
  editingToken.value = row
  editForm.value = { name: row.name, expires_at: fromRFC3339(row.expires_at) }
  editDialogVisible.value = true
}

async function handleEdit() {
  if (!editingToken.value || !editForm.value.name.trim()) { toast.warn('请输入名称'); return }
  editSubmitting.value = true
  try {
    const payload: any = { name: editForm.value.name.trim() }
    const iso = toRFC3339(editForm.value.expires_at)
    if (iso) payload.expires_at = iso
    else payload.expires_at_null = true
    const res = await updateSubToken(editingToken.value.id, payload)
    const data: any = res.data
    const updated: SubscriptionToken = data?.token ?? data
    const idx = tokens.value.findIndex(t => t.id === editingToken.value!.id)
    if (idx !== -1) tokens.value[idx] = updated
    editDialogVisible.value = false
    toast.success('已更新')
  } catch { toast.error('更新失败') }
  finally { editSubmitting.value = false }
}
</script>

<style scoped>
.sub-user {
  display: flex; align-items: baseline; gap: 10px;
  margin: 0 0 16px;
}
.sub-user__name {
  font-family: var(--font-serif); font-size: 18px; font-weight: 600;
  color: var(--color-ink-strong);
}

.sub-pane { padding-top: 16px; display: flex; flex-direction: column; gap: 16px; }
.token-picker { max-width: 320px; }
.sub-formats { margin-top: 4px; }

.format-pane { display: flex; flex-direction: column; gap: 16px; padding-top: 8px; }
.url-row { display: flex; gap: 8px; align-items: center; }
.url-input :deep(.input__field) { font-family: var(--font-mono); font-size: 12px; }

.qr {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.qr canvas {
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  padding: 8px;
  background: white;
}
.qr__hint { font-size: 12px; color: var(--color-ink-muted); margin: 0; }

.manage-bar { display: flex; justify-content: flex-end; }

.cell-name { font-weight: 600; color: var(--color-ink-strong); }
.cell-none { color: var(--color-ink-soft); font-size: 12px; }
.inline-edit { margin-left: 4px; vertical-align: middle; }
.soon { color: var(--color-status-warn); font-weight: 600; }

.ipbind { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.bound-ip { font-size: 12px; color: var(--color-ink-muted); }
.link-btn {
  background: transparent; border: 0; padding: 0;
  color: var(--color-status-warn);
  font-size: 12px; cursor: pointer;
}
.link-btn:hover { text-decoration: underline; }
</style>
