<template>
  <div>
    <el-button @click="onAdd">+ 新增规则</el-button>
    <el-table :data="config.customRules" border style="margin-top: 12px">
      <el-table-column prop="Name" label="名称" width="200" />
      <el-table-column label="Site">
        <template #default="{ row }">
          <el-tag v-for="t in row.SiteTags" :key="t" size="small" style="margin-right: 4px">{{ t }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="IP">
        <template #default="{ row }">
          <el-tag v-for="t in row.IPTags" :key="t" size="small" type="warning" style="margin-right: 4px">{{ t }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Domain Suffix">
        <template #default="{ row }">{{ row.DomainSuffix.join(', ') }}</template>
      </el-table-column>
      <el-table-column label="IP CIDR">
        <template #default="{ row }">{{ row.IPCIDR.join(', ') }}</template>
      </el-table-column>
      <el-table-column label="出站" width="160">
        <template #default="{ row }">
          {{ row.OutboundLiteral || groupName(row.OutboundGroupID) }}
        </template>
      </el-table-column>
      <el-table-column prop="SortOrder" label="排序" width="80" />
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button size="small" @click="onEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <CustomRuleEditDialog v-model="editing" :groups="config.groups" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createCustomRule, updateCustomRule, deleteCustomRule } from '../../../api/routing'
import type { RoutingConfig, CustomRule } from './types'
import CustomRuleEditDialog from './CustomRuleEditDialog.vue'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()
const editing = ref<CustomRule | null>(null)

function groupName(id: number | null) {
  return props.config.groups.find(g => g.ID === id)?.DisplayName || '-'
}

function onAdd() {
  editing.value = {
    ID: 0,
    Name: '',
    SiteTags: [],
    IPTags: [],
    DomainSuffix: [],
    DomainKeyword: [],
    IPCIDR: [],
    SrcIPCIDR: [],
    Protocol: '',
    Port: '',
    OutboundGroupID: props.config.groups[0]?.ID ?? null,
    OutboundLiteral: '',
    SortOrder: 100,
  }
}

function onEdit(row: CustomRule) {
  editing.value = JSON.parse(JSON.stringify(row))
}

async function onSave(row: CustomRule) {
  if (row.ID === 0) await createCustomRule(row)
  else await updateCustomRule(row.ID, row)
  editing.value = null
  ElMessage.success('已保存')
  emit('refresh')
}

async function onDelete(row: CustomRule) {
  try {
    await ElMessageBox.confirm(`删除规则 ${row.Name}？`, '确认', { type: 'warning' })
  } catch { return }
  await deleteCustomRule(row.ID)
  emit('refresh')
}
</script>
