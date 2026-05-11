<template>
  <div class="plans" :class="{ 'is-loading-overlay': loading }">
    <div class="toolbar">
      <p class="toolbar__hint">
        共 <span class="num">{{ plans.length }}</span> 个套餐，其中 <span class="num">{{ enabledCount }}</span> 启用。
      </p>
      <Button variant="primary" @click="openDialog()">
        <Plus :size="14" :stroke-width="2" /> 新增套餐
      </Button>
    </div>

    <table v-if="plans.length || loading" class="dt dt--responsive">
      <thead>
        <tr>
          <th>套餐</th>
          <th>流量</th>
          <th>有效期</th>
          <th>节点分组</th>
          <th>启用</th>
          <th class="is-numeric">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in plans" :key="row.id">
          <td><span class="cell-name">{{ row.name }}</span></td>
          <td data-label="流量"><span class="num cell-num">{{ formatBytesOrInf(row.traffic_limit) }}</span></td>
          <td data-label="有效期"><span class="num cell-num">{{ row.duration_days > 0 ? row.duration_days + ' 天' : '∞' }}</span></td>
          <td data-label="节点分组">
            <template v-if="row.node_group_ids?.length">
              <Tag v-for="gid in row.node_group_ids" :key="gid" :mono="false" class="mr-1">
                {{ groupName(gid) }}
              </Tag>
            </template>
            <span v-else class="cell-none">—</span>
          </td>
          <td data-label="状态">
            <StatusDot :state="row.enabled ? 'ok' : 'off'">{{ row.enabled ? '启用' : '停用' }}</StatusDot>
          </td>
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

    <div v-if="!loading && !plans.length" class="empty-state">
      <p class="empty-state__title">还没有套餐</p>
      <p class="empty-state__hint">套餐定义了用户的流量额度、有效期，以及可访问哪些节点分组。</p>
      <Button variant="primary" @click="openDialog()">
        <Plus :size="14" :stroke-width="2" /> 添加第一个套餐
      </Button>
    </div>

    <Modal v-model:open="dialogVisible" :title="isEdit ? '编辑套餐' : '新增套餐'" :width="560">
      <Field label="名称" layout="row">
        <Input v-model="form.name" placeholder="例如：标准月套餐 100G" />
      </Field>
      <Field label="流量上限" hint="GB · 0 为无限制" layout="row">
        <NumberInput v-model="trafficGB" :min="0" :precision="2" :step="1" />
      </Field>
      <Field label="有效期" hint="天 · 0 为无限制" layout="row">
        <NumberInput v-model="form.duration_days" :min="0" :step="1" />
      </Field>
      <Field label="排序" hint="数值越小越靠前" layout="row">
        <NumberInput v-model="form.sort_order" :min="0" />
      </Field>
      <Field label="启用" layout="row">
        <Switch v-model="form.enabled" />
      </Field>
      <Field label="节点分组" layout="row">
        <MultiSelect
          v-model="form.node_group_ids"
          :options="groups.map((g) => ({ label: g.name, value: g.id }))"
          placeholder="选择该套餐授权的节点分组"
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
import { ref, onMounted, computed, watch } from 'vue'
import { Plus, Pencil, Trash2 } from 'lucide-vue-next'
import {
  Button, Input, NumberInput, MultiSelect, Switch, Modal, Field, StatusDot, Tag,
  toast, confirm,
} from '../ui'
import { getPlans, createPlan, updatePlan, deletePlan, getNodeGroups } from '../api/plan'
import { formatBytes as formatBytesGlobal } from '../utils/format'

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

const enabledCount = computed(() => plans.value.filter((p) => p.enabled).length)

watch(trafficGB, (v) => {
  form.value.traffic_limit = Math.round((v || 0) * 1024 * 1024 * 1024)
})

const groupName = (id: number) => groups.value.find(g => g.id === id)?.name || `#${id}`

function formatBytesOrInf(b: number): string {
  if (!b) return '∞'
  return formatBytesGlobal(b)
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
  if (!form.value.name.trim()) { toast.warn('请输入套餐名称'); return }
  try {
    if (isEdit.value && editingId.value) await updatePlan(editingId.value, form.value)
    else await createPlan(form.value)
    toast.success('保存成功')
    dialogVisible.value = false
    await fetchAll()
  } catch (e: any) {
    toast.error(e?.response?.data?.error || '保存失败')
  }
}

async function onDelete(id: number) {
  try {
    await confirm({ title: '删除套餐', message: '确认删除该套餐？', tone: 'danger', confirmText: '删除' })
    await deletePlan(id)
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
.plans { display: flex; flex-direction: column; gap: 24px; }
.cell-name { font-weight: 600; color: var(--color-ink-strong); }
.cell-num { color: var(--color-ink-base); }
.cell-none { color: var(--color-ink-soft); }
.mr-1 { margin-right: 4px; }
</style>
