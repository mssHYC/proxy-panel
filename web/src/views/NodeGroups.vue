<template>
  <div v-loading="loading" class="p-4 space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-bold">节点分组</h2>
      <el-button type="primary" @click="openDialog()">
        <el-icon class="mr-1"><Plus /></el-icon>新增分组
      </el-button>
    </div>

    <el-card shadow="hover">
      <el-table :data="groups" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column label="包含节点" min-width="280">
          <template #default="{ row }">
            <el-tag v-for="nid in row.node_ids" :key="nid" size="small" class="mr-1 mb-1">
              {{ nodeName(nid) }}
            </el-tag>
            <span v-if="!row.node_ids?.length" class="text-gray-400">—</span>
          </template>
        </el-table-column>
        <el-table-column prop="sort_order" label="排序" width="80" />
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDialog(row)">编辑</el-button>
            <el-popconfirm title="确认删除该分组？" @confirm="handleDelete(row.id)">
              <template #reference>
                <el-button link type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑分组' : '新增分组'" width="520px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" />
        </el-form-item>
        <el-form-item label="节点">
          <el-select v-model="form.node_ids" multiple filterable style="width:100%" placeholder="选择该分组包含的节点">
            <el-option v-for="n in allNodes" :key="n.id" :label="n.name" :value="n.id" />
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
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { getNodeGroups, createNodeGroup, updateNodeGroup, deleteNodeGroup } from '../api/plan'
import { getNodes } from '../api/node'

const loading = ref(false)
const groups = ref<any[]>([])
const allNodes = ref<{ id: number; name: string }[]>([])
const dialogVisible = ref(false)
const isEdit = ref(false)
const editingId = ref<number | null>(null)
const form = ref<{ name: string; sort_order: number; node_ids: number[] }>({
  name: '', sort_order: 0, node_ids: [],
})

const nodeName = computed(() => (id: number) => allNodes.value.find(n => n.id === id)?.name || `#${id}`)

async function fetchAll() {
  loading.value = true
  try {
    const [g, n] = await Promise.all([getNodeGroups(), getNodes()])
    groups.value = (g.data as any).groups || []
    allNodes.value = ((n.data as any).nodes || []).map((x: any) => ({ id: x.id, name: x.name }))
  } finally {
    loading.value = false
  }
}

function openDialog(row?: any) {
  isEdit.value = !!row
  editingId.value = row?.id ?? null
  form.value = {
    name: row?.name || '',
    sort_order: row?.sort_order || 0,
    node_ids: [...(row?.node_ids || [])],
  }
  dialogVisible.value = true
}

async function handleSubmit() {
  if (!form.value.name.trim()) {
    ElMessage.warning('请输入分组名称')
    return
  }
  try {
    if (isEdit.value && editingId.value) {
      await updateNodeGroup(editingId.value, form.value)
    } else {
      await createNodeGroup(form.value)
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
    await deleteNodeGroup(id)
    ElMessage.success('已删除')
    await fetchAll()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '删除失败')
  }
}

onMounted(fetchAll)
</script>
