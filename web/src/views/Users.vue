<template>
  <div class="p-6">
    <!-- 顶部栏 -->
    <div class="flex items-center justify-between mb-6">
      <h2 class="text-2xl font-bold">用户管理</h2>
      <el-button type="primary" @click="openCreate">
        <el-icon class="mr-1"><Plus /></el-icon>
        新增用户
      </el-button>
    </div>

    <!-- 用户表格 -->
    <el-table :data="users" v-loading="loading" stripe border class="w-full">
      <el-table-column prop="username" label="用户名" min-width="120" />

      <el-table-column label="节点" min-width="200">
        <template #default="{ row }">
          <template v-if="row.node_ids && row.node_ids.length">
            <el-tag
              v-for="nid in row.node_ids"
              :key="nid"
              size="small"
              class="mr-1 mb-1"
            >
              {{ nodeNameMap[nid] || `节点#${nid}` }}
            </el-tag>
          </template>
          <el-tag v-else size="small" type="info">全部节点</el-tag>
        </template>
      </el-table-column>

      <el-table-column label="流量" min-width="200">
        <template #default="{ row }">
          <div class="flex flex-col gap-1">
            <el-progress
              :percentage="trafficPercent(row)"
              :stroke-width="10"
              :color="trafficPercent(row) > 90 ? '#f56c6c' : '#409eff'"
            />
            <span class="text-xs text-gray-500">
              {{ formatBytes(row.traffic_used || 0) }} /
              {{ row.traffic_limit ? formatBytes(row.traffic_limit) : '无限制' }}
            </span>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="状态" width="90" align="center">
        <template #default="{ row }">
          <el-switch
            :model-value="row.enable"
            @change="(val: boolean) => handleToggle(row, val)"
          />
        </template>
      </el-table-column>

      <el-table-column label="到期时间" width="140">
        <template #default="{ row }">
          <span :class="{ 'text-orange-500 font-medium': isExpiringSoon(row.expire_at) }">
            {{ formatDate(row.expire_at) }}
          </span>
        </template>
      </el-table-column>

      <el-table-column label="操作" width="240" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="openSub(row)" title="订阅">
            <el-icon><Link /></el-icon>
          </el-button>
          <el-button size="small" @click="openEdit(row)" title="编辑">
            <el-icon><Edit /></el-icon>
          </el-button>
          <el-button size="small" @click="handleResetTraffic(row)" title="重置流量">
            <el-icon><Refresh /></el-icon>
          </el-button>
          <el-button size="small" type="danger" @click="handleDelete(row)" title="删除">
            <el-icon><Delete /></el-icon>
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增 / 编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑用户' : '新增用户'"
      width="520px"
      destroy-on-close
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="100px"
      >
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="请输入邮箱" />
        </el-form-item>
        <el-form-item label="节点">
          <el-select
            v-model="form.node_ids"
            multiple
            placeholder="请选择节点 (不选则可访问全部)"
            class="w-full"
            collapse-tags
            collapse-tags-tooltip
          >
            <el-option
              v-for="node in allNodes"
              :key="node.id"
              :label="node.name"
              :value="node.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="流量限额 GB">
          <el-input-number v-model="form.traffic_limit_gb" :min="0" :precision="1" controls-position="right" />
          <span class="ml-2 text-xs text-gray-400">0 = 无限制</span>
        </el-form-item>
        <el-form-item label="限速 Mbps">
          <el-input-number v-model="form.speed_limit" :min="0" controls-position="right" />
          <span class="ml-2 text-xs text-gray-400">0 = 无限制</span>
        </el-form-item>
        <el-form-item label="重置日">
          <el-input-number v-model="form.reset_day" :min="1" :max="31" controls-position="right" />
        </el-form-item>
        <el-form-item label="到期时间">
          <el-date-picker
            v-model="form.expire_at"
            type="date"
            placeholder="选择到期时间"
            value-format="YYYY-MM-DD"
            class="w-full"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>

    <!-- 订阅链接弹窗 -->
    <SubscriptionDialog v-model:visible="subVisible" :user="subUser" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getUsers, createUser, updateUser, deleteUser, resetTraffic } from '../api/user'
import { getNodes } from '../api/node'
import { formatBytes, formatDate } from '../utils/format'
import SubscriptionDialog from '../components/SubscriptionDialog.vue'

// ---- 类型 ----
interface User {
  id: number
  uuid: string
  username: string
  email: string
  protocol: string
  traffic_used: number
  traffic_limit: number
  speed_limit: number
  reset_day: number
  enable: boolean
  expire_at: string | null
  node_ids: number[]
}

interface NodeItem {
  id: number
  name: string
}

// ---- 状态 ----
const loading = ref(false)
const users = ref<User[]>([])
const allNodes = ref<NodeItem[]>([])
const nodeNameMap = ref<Record<number, string>>({})
const dialogVisible = ref(false)
const isEdit = ref(false)
const editingId = ref<number | null>(null)
const submitting = ref(false)
const formRef = ref<FormInstance>()

const GB = 1024 ** 3

const defaultForm = () => ({
  username: '',
  email: '',
  protocol: 'vless',
  node_ids: [] as number[],
  traffic_limit_gb: 0,
  speed_limit: 0,
  reset_day: 1,
  expire_at: null as string | null,
})
const form = reactive(defaultForm())

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
}

// 订阅弹窗
const subVisible = ref(false)
const subUser = ref<{ uuid: string; username: string } | null>(null)

// ---- 工具函数 ----
function trafficPercent(row: User): number {
  if (!row.traffic_limit || row.traffic_limit === 0) return 0
  return Math.min(100, Math.round((row.traffic_used / row.traffic_limit) * 100))
}

function isExpiringSoon(date: string | null): boolean {
  if (!date) return false
  const diff = new Date(date).getTime() - Date.now()
  return diff > 0 && diff < 7 * 24 * 3600 * 1000 // 7 天内
}

// ---- 数据加载 ----
async function fetchNodes() {
  try {
    const res = await getNodes()
    const list = res.data?.nodes ?? []
    allNodes.value = list.map((n: any) => ({ id: n.id, name: n.name }))
    const map: Record<number, string> = {}
    for (const n of list) {
      map[n.id] = n.name
    }
    nodeNameMap.value = map
  } catch {
    // 静默失败
  }
}

async function fetchUsers() {
  loading.value = true
  try {
    const res = await getUsers()
    users.value = res.data?.users ?? []
  } catch {
    ElMessage.error('加载用户列表失败')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await fetchNodes()
  await fetchUsers()
})

// ---- 新增 / 编辑 ----
function resetForm() {
  Object.assign(form, defaultForm())
}

function openCreate() {
  isEdit.value = false
  editingId.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: User) {
  isEdit.value = true
  editingId.value = row.id
  Object.assign(form, {
    username: row.username,
    email: row.email || '',
    protocol: row.protocol,
    node_ids: row.node_ids ? [...row.node_ids] : [],
    traffic_limit_gb: row.traffic_limit ? +(row.traffic_limit / GB).toFixed(1) : 0,
    speed_limit: row.speed_limit || 0,
    reset_day: row.reset_day || 1,
    expire_at: row.expire_at,
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
    expire_at: form.expire_at || undefined,
  }
}

async function handleSubmit() {
  if (!formRef.value) return
  await formRef.value.validate()
  submitting.value = true
  try {
    const payload = buildPayload()
    if (isEdit.value && editingId.value !== null) {
      await updateUser(editingId.value, payload)
      ElMessage.success('更新成功')
    } else {
      await createUser(payload)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    await fetchUsers()
  } catch {
    ElMessage.error(isEdit.value ? '更新失败' : '创建失败')
  } finally {
    submitting.value = false
  }
}

// ---- 开关 ----
async function handleToggle(row: User, val: boolean) {
  try {
    await updateUser(row.id, { enable: val })
    row.enable = val
    ElMessage.success(val ? '已启用' : '已禁用')
  } catch {
    ElMessage.error('操作失败')
  }
}

// ---- 删除 ----
async function handleDelete(row: User) {
  try {
    await ElMessageBox.confirm(`确认删除用户「${row.username}」？`, '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await deleteUser(row.id)
    ElMessage.success('已删除')
    await fetchUsers()
  } catch {
    // 用户取消或删除失败
  }
}

// ---- 重置流量 ----
async function handleResetTraffic(row: User) {
  try {
    await ElMessageBox.confirm(`确认重置用户「${row.username}」的流量？`, '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await resetTraffic(row.id)
    ElMessage.success('流量已重置')
    await fetchUsers()
  } catch {
    // 用户取消或重置失败
  }
}

// ---- 订阅 ----
function openSub(row: User) {
  subUser.value = { uuid: row.uuid, username: row.username }
  subVisible.value = true
}
</script>
