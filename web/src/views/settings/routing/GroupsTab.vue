<template>
  <div class="grps">
    <div class="grps__bar">
      <p class="grps__hint">出站组定义一组节点的选择策略，是流量的"出口"。</p>
      <Button variant="primary" @click="onAdd">
        <Plus :size="14" :stroke-width="2" /> 新增自定义组
      </Button>
    </div>

    <table v-if="config.groups.length" class="dt">
      <thead>
        <tr>
          <th>显示名</th>
          <th>Code</th>
          <th>类型</th>
          <th>成员</th>
          <th>种类</th>
          <th class="is-numeric">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in config.groups" :key="row.ID">
          <td><span class="cell-name">{{ row.DisplayName }}</span></td>
          <td><span class="mono cell-code">{{ row.Code }}</span></td>
          <td><span class="mono cell-meta">{{ row.Type }}</span></td>
          <td>
            <Tag v-for="m in row.Members" :key="m" class="mr-1">{{ m }}</Tag>
          </td>
          <td><span class="kind-tag" :data-kind="row.Kind">{{ row.Kind === 'system' ? '系统' : '自定义' }}</span></td>
          <td class="is-numeric">
            <div class="row-actions">
              <button class="row-actions__btn" @click="onEdit(row)" title="编辑">
                <Pencil :size="14" :stroke-width="1.6" />
              </button>
              <button v-if="row.Kind === 'custom'" class="row-actions__btn row-actions__btn--danger" @click="onDelete(row)" title="删除">
                <Trash2 :size="14" :stroke-width="1.6" />
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>

    <GroupEditDialog v-model="editing" :groups="config.groups" :readonly-code="editingIsSystem" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Pencil, Trash2 } from 'lucide-vue-next'
import { Button, Tag, toast, confirm } from '../../../ui'
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
    ID: 0, Code: '', DisplayName: '', Type: 'selector',
    Members: [], Kind: 'custom', SortOrder: 500,
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
  toast.success('已保存')
  emit('refresh')
}

async function onDelete(row: Group) {
  try {
    await confirm({ message: `删除出站组 ${row.DisplayName}？`, tone: 'danger', confirmText: '删除' })
  } catch { return }
  try {
    await deleteGroup(row.ID)
    emit('refresh')
  } catch (e: any) { toast.error(e?.response?.data?.error || '删除失败') }
}
</script>

<style scoped>
.grps { display: flex; flex-direction: column; gap: 16px; }
.grps__bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.grps__hint { margin: 0; font-size: 13px; color: var(--color-ink-muted); }
.cell-name { font-weight: 600; color: var(--color-ink-strong); }
.cell-code { color: var(--color-ink-base); font-size: 13px; }
.cell-meta { color: var(--color-ink-muted); font-size: 12px; }
.kind-tag {
  font-family: var(--font-mono);
  font-size: 11px; font-weight: 600;
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--color-surface-sunken);
  color: var(--color-ink-muted);
}
.kind-tag[data-kind="custom"] { color: var(--color-accent-ink); background: var(--color-accent-soft); }
.mr-1 { margin-right: 4px; }
</style>
