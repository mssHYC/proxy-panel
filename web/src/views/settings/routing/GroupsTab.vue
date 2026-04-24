<template>
  <div>
    <el-button @click="onAdd">+ 新增自定义组</el-button>
    <el-table :data="config.groups" border style="margin-top: 12px" row-key="ID">
      <el-table-column prop="DisplayName" label="显示名" width="200" />
      <el-table-column prop="Code" label="Code" width="160" />
      <el-table-column prop="Type" label="类型" width="100" />
      <el-table-column label="成员">
        <template #default="{ row }">
          <el-tag v-for="m in row.Members" :key="m" size="small" style="margin-right: 4px">{{ m }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="80">
        <template #default="{ row }">
          <el-tag :type="row.Kind === 'system' ? 'info' : 'success'" size="small">
            {{ row.Kind === 'system' ? '系统' : '自定义' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="160">
        <template #default="{ row }">
          <el-button size="small" @click="onEdit(row)">编辑</el-button>
          <el-button v-if="row.Kind === 'custom'" size="small" type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <GroupEditDialog v-model="editing" :groups="config.groups" :readonly-code="editingIsSystem" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createGroup, updateGroup, deleteGroup } from '../../../api/routing'
import type { RoutingConfig, Group } from './types'
import GroupEditDialog from './GroupEditDialog.vue'

defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()
const editing = ref<Group | null>(null)
const editingIsSystem = ref(false)

function onAdd() {
  editingIsSystem.value = false
  editing.value = {
    ID: 0,
    Code: '',
    DisplayName: '',
    Type: 'selector',
    Members: [],
    Kind: 'custom',
    SortOrder: 500,
  }
}

function onEdit(row: Group) {
  editingIsSystem.value = row.Kind === 'system'
  editing.value = { ...row, Members: [...row.Members] }
}

async function onSave(row: Group) {
  if (row.ID === 0) await createGroup(row)
  else await updateGroup(row.ID, row)
  editing.value = null
  ElMessage.success('已保存')
  emit('refresh')
}

async function onDelete(row: Group) {
  try {
    await ElMessageBox.confirm(`删除出站组 ${row.DisplayName}？`, '确认', { type: 'warning' })
  } catch { return }
  try {
    await deleteGroup(row.ID)
    emit('refresh')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '删除失败')
  }
}
</script>
