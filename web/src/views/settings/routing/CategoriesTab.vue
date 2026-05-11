<template>
  <div class="cats">
    <div class="cats__bar">
      <div class="cats__preset">
        <span class="cats__preset-label">预设方案</span>
        <Select
          :model-value="presetCode"
          :options="config.presets.map(p => ({ label: p.DisplayName, value: p.Code }))"
          placeholder="选择预设"
          class="cats__preset-sel"
          @update:model-value="(v) => (presetCode = String(v))"
        />
        <Button variant="primary" :disabled="!presetCode" @click="onApplyPreset">应用 · 覆盖启用分类</Button>
      </div>
      <Button @click="onAddCustom">
        <Plus :size="14" :stroke-width="2" /> 新增自定义分类
      </Button>
    </div>

    <table v-if="config.categories.length" class="dt dt--responsive">
      <thead>
        <tr>
          <th>名称</th>
          <th>类型</th>
          <th>Site Tags</th>
          <th>IP Tags</th>
          <th>默认出站组</th>
          <th>启用</th>
          <th class="is-numeric">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in config.categories" :key="row.ID">
          <td><span class="cell-name">{{ row.DisplayName }}</span></td>
          <td data-label="类型"><span class="kind-tag" :data-kind="row.Kind">{{ row.Kind === 'system' ? '系统' : '自定义' }}</span></td>
          <td data-label="Site Tags">
            <Tag v-for="t in row.SiteTags" :key="t" class="mr-1">{{ t }}</Tag>
            <span v-if="!row.SiteTags?.length" class="cell-none">—</span>
          </td>
          <td data-label="IP Tags">
            <Tag v-for="t in row.IPTags" :key="t" class="mr-1">{{ t }}</Tag>
            <span v-if="!row.IPTags?.length" class="cell-none">—</span>
          </td>
          <td data-label="出站组">
            <Select
              :model-value="row.DefaultGroupID"
              :options="config.groups.map(g => ({ label: g.DisplayName, value: g.ID }))"
              @update:model-value="(v) => { row.DefaultGroupID = v as number; onUpdate(row) }"
            />
          </td>
          <td data-label="启用">
            <Switch :model-value="row.Enabled" @update:model-value="(v) => { row.Enabled = v; onUpdate(row) }" />
          </td>
          <td class="is-numeric dt-actions">
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

    <CategoryEditDialog v-model="editing" :readonly="editingIsSystem" :groups="config.groups" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Pencil, Trash2 } from 'lucide-vue-next'
import { Button, Select, Switch, Tag, toast, confirm } from '../../../ui'
import { applyPreset, createCategory, updateCategory, deleteCategory } from '../../../api/routing'
import type { RoutingConfig, Category } from './types'
import CategoryEditDialog from './CategoryEditDialog.vue'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()

const presetCode = ref('')
const editing = ref<Category | null>(null)
const editingIsSystem = ref(false)

async function onApplyPreset() {
  try {
    await confirm({ message: '将覆盖当前启用的分类，确定？', tone: 'danger', confirmText: '应用' })
  } catch { return }
  await applyPreset(presetCode.value)
  toast.success('已应用')
  emit('refresh')
}

async function onUpdate(row: Category) {
  await updateCategory(row.ID, row)
  toast.success('已保存')
  emit('refresh')
}

function onEdit(row: Category) {
  editingIsSystem.value = row.Kind === 'system'
  editing.value = JSON.parse(JSON.stringify(row))
}

function onAddCustom() {
  editingIsSystem.value = false
  editing.value = {
    ID: 0, Code: '', DisplayName: '', Kind: 'custom',
    SiteTags: [], IPTags: [],
    InlineDomainSuffix: [], InlineDomainKeyword: [], InlineIPCIDR: [],
    Protocol: '',
    DefaultGroupID: props.config.groups[0]?.ID ?? null,
    Enabled: true, SortOrder: 500,
  }
}

async function onSave(row: Category) {
  if (row.ID === 0) await createCategory(row)
  else await updateCategory(row.ID, row)
  editing.value = null
  toast.success('已保存')
  emit('refresh')
}

async function onDelete(row: Category) {
  try {
    await confirm({ message: `删除自定义分类 ${row.DisplayName}？`, tone: 'danger', confirmText: '删除' })
  } catch { return }
  await deleteCategory(row.ID)
  emit('refresh')
}
</script>

<style scoped>
.cats { display: flex; flex-direction: column; gap: 16px; }

.cats__bar {
  display: flex; align-items: center; justify-content: space-between;
  gap: 12px; flex-wrap: wrap;
}
.cats__preset { display: flex; gap: 10px; align-items: center; }
.cats__preset-label {
  font-size: 12px; font-weight: 600;
  letter-spacing: 0.04em; text-transform: uppercase;
  color: var(--color-ink-muted);
}
.cats__preset-sel { width: 200px; }

.cell-name { font-weight: 600; color: var(--color-ink-strong); }
.cell-none { color: var(--color-ink-soft); font-size: 12px; }

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

@media (max-width: 1023px) {
  .cats__bar { flex-direction: column; align-items: stretch; gap: 12px; }
  .cats__preset { flex-direction: column; align-items: stretch; gap: 8px; }
  .cats__preset-sel { width: 100%; }
  /* Default-group Select inside the responsive table cell stretches to fill
     the value column instead of fighting it. */
  .dt--responsive tbody td > :deep(.sel) { width: 100%; max-width: 220px; }
}
</style>
