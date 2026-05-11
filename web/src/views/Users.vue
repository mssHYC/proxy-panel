<template>
  <div class="users" :class="{ 'is-loading-overlay': loading }">
    <div class="toolbar">
      <p class="toolbar__hint">
        共 <span class="num">{{ users.length }}</span> 位用户，其中
        <span class="num">{{ enabledCount }}</span> 启用。
      </p>
      <Button variant="primary" @click="openCreate">
        <Plus :size="14" :stroke-width="2" /> 新增用户
      </Button>
    </div>

    <table v-if="users.length || loading" class="dt dt--responsive">
      <thead>
        <tr>
          <th>用户</th>
          <th>节点访问</th>
          <th>流量</th>
          <th>状态</th>
          <th>到期</th>
          <th>套餐</th>
          <th class="is-numeric">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in users" :key="row.id">
          <td>
            <div class="cell-user">
              <span class="cell-user__name">{{ row.username }}</span>
              <span v-if="row.email" class="cell-user__email">{{ row.email }}</span>
            </div>
          </td>
          <td data-label="节点">
            <template v-if="row.node_ids && row.node_ids.length">
              <Tag v-for="nid in row.node_ids" :key="nid" :mono="false" class="mr-1">
                {{ nodeNameMap[nid] || `节点#${nid}` }}
              </Tag>
            </template>
            <span v-else class="cell-allnodes">全部节点</span>
          </td>
          <td data-label="流量">
            <div class="cell-traffic">
              <div class="cell-traffic__row">
                <span class="num">{{ formatBytes(row.traffic_used || 0) }}</span>
                <span class="cell-traffic__sep">/</span>
                <span class="num cell-traffic__limit">
                  {{ row.traffic_limit ? formatBytes(row.traffic_limit) : '∞' }}
                </span>
              </div>
              <ProgressBar
                v-if="row.traffic_limit"
                :percent="trafficPercent(row)"
                :thresholds="{ warn: 80, crit: 95 }"
              />
            </div>
          </td>
          <td data-label="启用">
            <Switch :model-value="row.enable" @update:model-value="(v) => handleToggle(row, v)" />
          </td>
          <td data-label="到期">
            <span class="cell-expire" :data-soon="isExpiringSoon(row.expires_at) ? '1' : null">
              <span class="num">{{ formatDate(row.expires_at) || '—' }}</span>
            </span>
          </td>
          <td data-label="套餐">
            <span v-if="planNameMap[row.plan_id ?? -1]">{{ planNameMap[row.plan_id ?? -1] }}</span>
            <span v-else class="cell-none">—</span>
          </td>
          <td class="is-numeric dt-actions">
            <div class="row-actions">
              <button class="row-actions__btn" @click="openSub(row)" title="订阅链接">
                <Link :size="14" :stroke-width="1.6" />
              </button>
              <button class="row-actions__btn" @click="openEdit(row)" title="编辑">
                <Pencil :size="14" :stroke-width="1.6" />
              </button>
              <button class="row-actions__btn" @click="openAssignPlan(row)" title="分配套餐">
                <Package :size="14" :stroke-width="1.6" />
              </button>
              <button class="row-actions__btn" @click="handleResetTraffic(row)" title="重置流量">
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

    <div v-if="!loading && !users.length" class="empty-state">
      <p class="empty-state__title">还没有用户</p>
      <p class="empty-state__hint">添加第一个用户后，可以为 ta 生成订阅链接，分发到 Surge / Clash / Sing-box 等客户端。</p>
      <Button variant="primary" @click="openCreate">
        <Plus :size="14" :stroke-width="2" /> 添加第一个用户
      </Button>
    </div>

    <Modal v-model:open="planDialogVisible" title="分配套餐" :width="480">
      <Field label="用户" layout="row">
        <span class="plan-user">{{ planTarget?.username }}</span>
      </Field>
      <Field label="套餐" layout="row">
        <Select
          :model-value="planForm.plan_id"
          :options="plans.map(p => ({ label: p.name, value: p.id }))"
          placeholder="留空可解除套餐"
          @update:model-value="(v) => (planForm.plan_id = v as number | null)"
        />
      </Field>
      <Field label="重置流量" hint="按套餐重新计算可用流量" layout="row">
        <Switch v-model="planForm.reset_traffic" :disabled="!planForm.plan_id" />
      </Field>
      <Field label="设置过期" hint="按套餐有效期推算到期时间" layout="row">
        <Switch v-model="planForm.set_expires_at" :disabled="!planForm.plan_id" />
      </Field>
      <template #footer>
        <Button @click="planDialogVisible = false">取消</Button>
        <Button variant="primary" @click="handleAssignPlan">保存</Button>
      </template>
    </Modal>

    <Modal v-model:open="dialogVisible" :title="isEdit ? '编辑用户' : '新增用户'" :width="540">
      <Field label="用户名" :error="errors.username" layout="row">
        <Input v-model="form.username" placeholder="例如 alice" />
      </Field>
      <Field label="邮箱" hint="可选，用于通知" layout="row">
        <Input v-model="form.email" />
      </Field>
      <Field label="节点" layout="row">
        <MultiSelect
          v-model="form.node_ids"
          :options="allNodes.map(n => ({ label: n.name, value: n.id }))"
          placeholder="不选则可访问全部节点"
        />
      </Field>
      <Field label="流量限额" hint="GB · 0 为无限制" layout="row">
        <NumberInput v-model="form.traffic_limit_gb" :min="0" :precision="1" />
      </Field>
      <Field label="限速" hint="Mbps · 仅 hy2 单用户场景" layout="row">
        <NumberInput v-model="form.speed_limit" :min="0" />
      </Field>
      <Field label="重置日" hint="每月此日重置流量" layout="row">
        <NumberInput v-model="form.reset_day" :min="1" :max="31" />
      </Field>
      <Field label="到期时间" layout="row">
        <DateInput v-model="form.expires_at" placeholder="选择日期" />
      </Field>
      <template #footer>
        <Button @click="dialogVisible = false">取消</Button>
        <Button variant="primary" :loading="submitting" @click="handleSubmit">
          {{ isEdit ? '保存' : '创建' }}
        </Button>
      </template>
    </Modal>

    <SubscriptionDialog v-model:visible="subVisible" :user="subUser" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, defineAsyncComponent, onMounted } from 'vue'
import { Plus, Pencil, Trash2, Link, RotateCcw, Package } from 'lucide-vue-next'
import {
  Button, Input, NumberInput, Select, MultiSelect, Switch, Modal, Field, Tag, ProgressBar, DateInput,
  toast, confirm,
} from '../ui'
import { getUsers, createUser, updateUser, deleteUser, resetTraffic } from '../api/user'
import { getNodes } from '../api/node'
import { getPlans, assignPlanToUser } from '../api/plan'
import { formatBytes, formatDate } from '../utils/format'

const SubscriptionDialog = defineAsyncComponent(() => import('../components/SubscriptionDialog.vue'))

interface User {
  id: number; uuid: string; username: string; email: string; protocol: string
  traffic_used: number; traffic_limit: number; speed_limit: number; reset_day: number
  enable: boolean; expires_at: string | null; node_ids: number[]; plan_id?: number | null
}

const loading = ref(false)
const users = ref<User[]>([])
const allNodes = ref<{ id: number; name: string }[]>([])
const nodeNameMap = ref<Record<number, string>>({})
const dialogVisible = ref(false)
const isEdit = ref(false)
const editingId = ref<number | null>(null)
const submitting = ref(false)
const errors = reactive<{ username?: string }>({})

const enabledCount = computed(() => users.value.filter(u => u.enable).length)

const GB = 1024 ** 3
const defaultForm = () => ({
  username: '', email: '', protocol: 'vless',
  node_ids: [] as number[],
  traffic_limit_gb: 0, speed_limit: 0, reset_day: 1,
  expires_at: null as string | null,
})
const form = reactive(defaultForm())

const subVisible = ref(false)
const subUser = ref<{ id: number; uuid: string; username: string } | null>(null)

const plans = ref<{ id: number; name: string }[]>([])
const planNameMap = ref<Record<number, string>>({})
const planDialogVisible = ref(false)
const planTarget = ref<User | null>(null)
const planForm = reactive<{ plan_id: number | null; reset_traffic: boolean; set_expires_at: boolean }>({
  plan_id: null, reset_traffic: true, set_expires_at: true,
})

function trafficPercent(row: User): number {
  if (!row.traffic_limit) return 0
  return Math.min(100, Math.round((row.traffic_used / row.traffic_limit) * 100))
}
function isExpiringSoon(date: string | null): boolean {
  if (!date) return false
  const diff = new Date(date).getTime() - Date.now()
  return diff > 0 && diff < 7 * 24 * 3600 * 1000
}

async function fetchNodes() {
  try {
    const res = await getNodes()
    const list = res.data?.nodes ?? []
    allNodes.value = list.map((n: any) => ({ id: n.id, name: n.name }))
    const map: Record<number, string> = {}
    for (const n of list) map[n.id] = n.name
    nodeNameMap.value = map
  } catch {/* silent */}
}

async function fetchUsers() {
  loading.value = true
  try {
    const res = await getUsers()
    users.value = res.data?.users ?? []
  } catch { toast.error('加载用户列表失败') }
  finally { loading.value = false }
}

async function fetchPlans() {
  try {
    const res = await getPlans()
    const list = res.data?.plans ?? []
    plans.value = list.map((p: any) => ({ id: p.id, name: p.name }))
    const map: Record<number, string> = {}
    for (const p of list) map[p.id] = p.name
    planNameMap.value = map
  } catch {/* silent */}
}

onMounted(async () => {
  await Promise.all([fetchNodes(), fetchPlans()])
  await fetchUsers()
})

function resetForm() { Object.assign(form, defaultForm()) }

function openCreate() {
  isEdit.value = false; editingId.value = null
  errors.username = ''
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: User) {
  isEdit.value = true; editingId.value = row.id
  errors.username = ''
  Object.assign(form, {
    username: row.username,
    email: row.email || '',
    protocol: row.protocol,
    node_ids: row.node_ids ? [...row.node_ids] : [],
    traffic_limit_gb: row.traffic_limit ? +(row.traffic_limit / GB).toFixed(1) : 0,
    speed_limit: row.speed_limit || 0,
    reset_day: row.reset_day || 1,
    expires_at: row.expires_at,
  })
  dialogVisible.value = true
}

function buildPayload() {
  return {
    username: form.username,
    email: form.email || undefined,
    protocol: form.protocol,
    node_ids: form.node_ids.length > 0 ? form.node_ids : [],
    traffic_limit: form.traffic_limit_gb ? Math.round(form.traffic_limit_gb * GB) : 0,
    speed_limit: form.speed_limit,
    reset_day: form.reset_day,
    expires_at: form.expires_at || undefined,
  }
}

async function handleSubmit() {
  if (!form.username.trim()) { errors.username = '请输入用户名'; return }
  errors.username = ''
  submitting.value = true
  try {
    const payload = buildPayload()
    if (isEdit.value && editingId.value !== null) await updateUser(editingId.value, payload)
    else await createUser(payload)
    toast.success(isEdit.value ? '更新成功' : '创建成功')
    dialogVisible.value = false
    await fetchUsers()
  } catch { toast.error(isEdit.value ? '更新失败' : '创建失败') }
  finally { submitting.value = false }
}

async function handleToggle(row: User, val: boolean) {
  try {
    await updateUser(row.id, { enable: val })
    row.enable = val
    toast.success(val ? '已启用' : '已禁用')
  } catch { toast.error('操作失败') }
}

async function handleDelete(row: User) {
  try {
    await confirm({
      title: '删除用户',
      message: `确认删除用户「${row.username}」？`,
      tone: 'danger',
      confirmText: '删除',
    })
    await deleteUser(row.id)
    toast.success('已删除')
    await fetchUsers()
  } catch (e) { if (e === 'cancel') return }
}

async function handleResetTraffic(row: User) {
  try {
    await confirm({
      title: '重置流量',
      message: `确认重置用户「${row.username}」的流量？`,
      confirmText: '重置',
      tone: 'danger',
    })
    await resetTraffic(row.id)
    toast.success('流量已重置')
    await fetchUsers()
  } catch (e) { if (e === 'cancel') return }
}

function openSub(row: User) {
  subUser.value = { id: row.id, uuid: row.uuid, username: row.username }
  subVisible.value = true
}

function openAssignPlan(row: User) {
  planTarget.value = row
  planForm.plan_id = row.plan_id ?? null
  planForm.reset_traffic = true
  planForm.set_expires_at = true
  planDialogVisible.value = true
}

async function handleAssignPlan() {
  if (!planTarget.value) return
  try {
    await assignPlanToUser(planTarget.value.id, {
      plan_id: planForm.plan_id || null,
      reset_traffic: planForm.reset_traffic,
      set_expires_at: planForm.set_expires_at,
    })
    toast.success('已应用套餐')
    planDialogVisible.value = false
    await fetchUsers()
  } catch (e: any) {
    toast.error(e?.response?.data?.error || '操作失败')
  }
}
</script>

<style scoped>
.users { display: flex; flex-direction: column; gap: 24px; }

.cell-user { display: flex; flex-direction: column; gap: 2px; }
.cell-user__name { color: var(--color-ink-strong); font-weight: 600; }
.cell-user__email { font-family: var(--font-mono); font-size: 12px; color: var(--color-ink-muted); }

.cell-allnodes { font-size: 12px; color: var(--color-ink-muted); font-style: italic; }
.cell-none { color: var(--color-ink-soft); }

.cell-traffic { display: flex; flex-direction: column; gap: 6px; min-width: 180px; }
@media (max-width: 1023px) {
  /* Responsive table puts each td value in grid column 2; let cell-traffic
     stretch to fill that column so the progress bar can span its width. */
  .dt--responsive tbody td > .cell-traffic { justify-self: stretch; align-items: stretch; }
  .cell-traffic__row { justify-content: flex-end; }
}
.cell-traffic__row { display: flex; align-items: baseline; gap: 6px; font-size: 13px; color: var(--color-ink-strong); }
.cell-traffic__sep { color: var(--color-ink-soft); }
.cell-traffic__limit { color: var(--color-ink-muted); }

.cell-expire .num { color: var(--color-ink-base); }
.cell-expire[data-soon="1"] .num { color: var(--color-status-warn); font-weight: 600; }

.plan-user { font-family: var(--font-serif); font-weight: 600; color: var(--color-ink-strong); }
.mr-1 { margin-right: 4px; }
</style>
