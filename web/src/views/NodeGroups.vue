<template>
  <div class="ngroups" :class="{ 'is-loading-overlay': loading }">
    <div class="toolbar">
      <p class="toolbar__hint">
        共 <span class="num">{{ groups.length }}</span> 个分组。
      </p>
      <Button variant="primary" @click="openDialog()">
        <Plus :size="14" :stroke-width="2" /> 新增分组
      </Button>
    </div>

    <table v-if="groups.length || loading" class="dt dt--responsive">
      <thead>
        <tr>
          <th>分组</th>
          <th>包含节点</th>
          <th class="is-numeric">节点数</th>
          <th class="is-numeric">排序</th>
          <th class="is-numeric">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in groups" :key="row.id">
          <td><span class="cell-name">{{ row.name }}</span></td>
          <td data-label="包含节点">
            <template v-if="row.node_ids?.length">
              <Tag v-for="nid in row.node_ids" :key="nid" :mono="false" class="mr-1">{{ nodeName(nid) }}</Tag>
            </template>
            <span v-else class="cell-none">—</span>
          </td>
          <td class="is-numeric" data-label="节点数"><span class="num">{{ row.node_ids?.length || 0 }}</span></td>
          <td class="is-numeric" data-label="排序"><span class="num cell-meta">{{ row.sort_order }}</span></td>
          <td class="is-numeric dt-actions">
            <div class="row-actions">
              <button class="row-actions__btn" @click="openDialog(row)" title="编辑">
                <Pencil :size="14" :stroke-width="1.6" />
              </button>
              <button class="row-actions__btn row-actions__btn--danger" @click="onDelete(row.id)" title="删除">
                <Trash2 :size="14" :stroke-width="1.6" />
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>

    <div v-if="!loading && !groups.length" class="empty-state">
      <p class="empty-state__title">还没有节点分组</p>
      <p class="empty-state__hint">分组用于把若干节点打包，在套餐里授权一组节点而不是单个节点。</p>
      <Button variant="primary" @click="openDialog()">
        <Plus :size="14" :stroke-width="2" /> 添加第一个分组
      </Button>
    </div>

    <Modal v-model:open="dialogVisible" :title="isEdit ? '编辑分组' : '新增分组'" :width="540">
      <Field label="名称" layout="row">
        <Input v-model="form.name" placeholder="例如：日本节点" />
      </Field>
      <Field label="排序" hint="数值越小越靠前" layout="row">
        <NumberInput v-model="form.sort_order" :min="0" />
      </Field>
      <Field label="节点" layout="row">
        <MultiSelect
          v-model="form.node_ids"
          :options="allNodes.map((n) => ({ label: n.name, value: n.id }))"
          placeholder="选择该分组包含的节点"
        />
      </Field>
      <template #footer>
        <Button @click="dialogVisible = false">取消</Button>
        <Button variant="primary" @click="handleSubmit">{{ isEdit ? '保存' : '创建' }}</Button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus, Pencil, Trash2 } from 'lucide-vue-next'
import { Button, Input, NumberInput, MultiSelect, Modal, Field, Tag, toast, confirm } from '../ui'
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

const nodeName = (id: number) => allNodes.value.find(n => n.id === id)?.name || `#${id}`

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
  if (!form.value.name.trim()) { toast.warn('请输入分组名称'); return }
  try {
    if (isEdit.value && editingId.value) await updateNodeGroup(editingId.value, form.value)
    else await createNodeGroup(form.value)
    toast.success('保存成功')
    dialogVisible.value = false
    await fetchAll()
  } catch (e: any) {
    toast.error(e?.response?.data?.error || '保存失败')
  }
}

async function onDelete(id: number) {
  try {
    await confirm({ message: '确认删除该分组？', tone: 'danger', confirmText: '删除' })
    await deleteNodeGroup(id)
    toast.success('已删除')
    await fetchAll()
  } catch (e: any) {
    if (e === 'cancel') return
    toast.error(e?.response?.data?.error || '删除失败')
  }
}

onMounted(fetchAll)
</script>

<style scoped>
.ngroups { display: flex; flex-direction: column; gap: 24px; }
.cell-name { font-weight: 600; color: var(--color-ink-strong); }
.cell-meta { color: var(--color-ink-muted); font-size: 12px; }
.cell-none { color: var(--color-ink-soft); }
.mr-1 { margin-right: 4px; }
</style>
