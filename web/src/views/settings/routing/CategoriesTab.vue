<template>
  <div>
    <div class="preset-bar" style="display: flex; gap: 12px; align-items: center; margin-bottom: 12px">
      <span>应用预设方案：</span>
      <el-select v-model="presetCode" placeholder="选择预设" style="width: 200px">
        <el-option v-for="p in config.presets" :key="p.Code" :label="p.DisplayName" :value="p.Code" />
      </el-select>
      <el-button type="primary" :disabled="!presetCode" @click="onApplyPreset">应用（覆盖启用分类）</el-button>
      <el-button @click="onAddCustom">+ 新增自定义分类</el-button>
    </div>

    <el-table :key="tableKey" :data="config.categories" border>
      <el-table-column prop="DisplayName" label="名称" width="180" />
      <el-table-column label="类型" width="80">
        <template #default="{ row }">
          <el-tag :type="row.Kind === 'system' ? 'info' : 'success'" size="small">
            {{ row.Kind === 'system' ? '系统' : '自定义' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Site Tags">
        <template #default="{ row }">
          <el-tag v-for="t in row.SiteTags" :key="t" size="small" style="margin-right: 4px">{{ t }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="IP Tags">
        <template #default="{ row }">
          <el-tag v-for="t in row.IPTags" :key="t" size="small" type="warning" style="margin-right: 4px">{{ t }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="默认出站组" width="200">
        <template #default="{ row }">
          <el-select v-model="row.DefaultGroupID" size="small" @change="onUpdate(row)">
            <el-option v-for="g in config.groups" :key="g.ID" :label="g.DisplayName" :value="g.ID" />
          </el-select>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="80">
        <template #default="{ row }">
          <el-switch v-model="row.Enabled" @change="onUpdate(row)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button size="small" @click="onEdit(row)">编辑</el-button>
          <el-button v-if="row.Kind === 'custom'" size="small" type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <CategoryEditDialog v-model="editing" :readonly="editingIsSystem" :groups="config.groups" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { applyPreset, createCategory, updateCategory, deleteCategory } from '../../../api/routing'
import type { RoutingConfig, Category } from './types'
import CategoryEditDialog from './CategoryEditDialog.vue'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()

const presetCode = ref('')
const editing = ref<Category | null>(null)
const editingIsSystem = ref(false)

// Force el-table re-mount when categories change so el-switch reflects updated Enabled state.
const tableKey = ref(0)
watch(() => props.config.categories, () => { tableKey.value++ })

async function onApplyPreset() {
  try {
    await ElMessageBox.confirm('将覆盖当前启用的分类，确定？', '应用预设', { type: 'warning' })
  } catch { return }
  await applyPreset(presetCode.value)
  ElMessage.success('已应用')
  emit('refresh')
}

async function onUpdate(row: Category) {
  await updateCategory(row.ID, row)
  ElMessage.success('已保存')
  emit('refresh')
}

function onEdit(row: Category) {
  editingIsSystem.value = row.Kind === 'system'
  editing.value = JSON.parse(JSON.stringify(row))
}

function onAddCustom() {
  editingIsSystem.value = false
  editing.value = {
    ID: 0,
    Code: '',
    DisplayName: '',
    Kind: 'custom',
    SiteTags: [],
    IPTags: [],
    InlineDomainSuffix: [],
    InlineDomainKeyword: [],
    InlineIPCIDR: [],
    Protocol: '',
    DefaultGroupID: props.config.groups[0]?.ID ?? null,
    Enabled: true,
    SortOrder: 500,
  }
}

async function onSave(row: Category) {
  if (row.ID === 0) {
    await createCategory(row)
  } else {
    await updateCategory(row.ID, row)
  }
  editing.value = null
  ElMessage.success('已保存')
  emit('refresh')
}

async function onDelete(row: Category) {
  try {
    await ElMessageBox.confirm(`删除自定义分类 ${row.DisplayName}？`, '确认', { type: 'warning' })
  } catch { return }
  await deleteCategory(row.ID)
  emit('refresh')
}
</script>
