<template>
  <div v-loading="loading" class="p-4 space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-bold">套餐管理</h2>
      <el-button type="primary" @click="openDialog()">
        <el-icon class="mr-1"><Plus /></el-icon>新增套餐
      </el-button>
    </div>

    <el-card shadow="hover">
      <el-table :data="plans" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column label="流量上限" width="140">
          <template #default="{ row }">{{ formatBytes(row.traffic_limit) }}</template>
        </el-table-column>
        <el-table-column prop="duration_days" label="有效期(天)" width="120" />
        <el-table-column label="节点分组" min-width="240">
          <template #default="{ row }">
            <el-tag v-for="gid in row.node_group_ids" :key="gid" size="small" class="mr-1 mb-1">
              {{ groupName(gid) }}
            </el-tag>
            <span v-if="!row.node_group_ids?.length" class="text-gray-400">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
              {{ row.enabled ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDialog(row)">编辑</el-button>
            <el-popconfirm title="确认删除该套餐？" @confirm="handleDelete(row.id)">
              <template #reference>
                <el-button link type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑套餐' : '新增套餐'" width="560px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="流量上限(GB)">
          <el-input-number v-model="trafficGB" :min="0" :precision="2" :step="1" />
          <span class="ml-2 text-gray-400 text-xs">0 表示不限</span>
        </el-form-item>
        <el-form-item label="有效期(天)">
          <el-input-number v-model="form.duration_days" :min="0" :step="1" />
          <span class="ml-2 text-gray-400 text-xs">0 表示不限</span>
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="节点分组">
          <el-select v-model="form.node_group_ids" multiple filterable style="width:100%" placeholder="选择该套餐授权的节点分组">
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { getPlans, createPlan, updatePlan, deletePlan, getNodeGroups } from '../api/plan'

const loading = ref(false)
const plans = ref<any[]>([])
const groups = ref<any[]>([])
const dialogVisible = ref(false)
const isEdit = ref(false)
const editingId = ref<number | null>(null)
const form = ref({
  name: '', traffic_limit: 0, duration_days: 30, sort_order: 0, enabled: true, node_group_ids: [] as number[],
})
const trafficGB = ref(0)

watch(trafficGB, (v) => {
  form.value.traffic_limit = Math.round((v || 0) * 1024 * 1024 * 1024)
})

const groupName = computed(() => (id: number) => groups.value.find(g => g.id === id)?.name || `#${id}`)

function formatBytes(b: number): string {
  if (!b) return '不限'
  const gb = b / 1024 / 1024 / 1024
  if (gb >= 1) return gb.toFixed(2) + ' GB'
  return (b / 1024 / 1024).toFixed(0) + ' MB'
}

async function fetchAll() {
  loading.value = true
  try {
    const [p, g] = await Promise.all([getPlans(), getNodeGroups()])
    plans.value = (p.data as any).plans || []
    groups.value = (g.data as any).groups || []
  } finally {
    loading.value = false
  }
}

function openDialog(row?: any) {
  isEdit.value = !!row
  editingId.value = row?.id ?? null
  form.value = {
    name: row?.name || '',
    traffic_limit: row?.traffic_limit || 0,
    duration_days: row?.duration_days ?? 30,
    sort_order: row?.sort_order || 0,
    enabled: row?.enabled ?? true,
    node_group_ids: [...(row?.node_group_ids || [])],
  }
  trafficGB.value = (row?.traffic_limit || 0) / 1024 / 1024 / 1024
  dialogVisible.value = true
}

async function handleSubmit() {
  if (!form.value.name.trim()) {
    ElMessage.warning('请输入套餐名称')
    return
  }
  try {
    if (isEdit.value && editingId.value) {
      await updatePlan(editingId.value, form.value)
    } else {
      await createPlan(form.value)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await fetchAll()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  }
}

async function handleDelete(id: number) {
  try {
    await deletePlan(id)
    ElMessage.success('已删除')
    await fetchAll()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '删除失败')
  }
}

onMounted(fetchAll)
</script>
