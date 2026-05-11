<template>
  <div class="rules">
    <div class="rules__bar">
      <p class="rules__hint">自定义规则优先级最高，不受预设方案影响。</p>
      <Button variant="primary" @click="onAdd">
        <Plus :size="14" :stroke-width="2" /> 新增规则
      </Button>
    </div>

    <table v-if="config.customRules?.length" class="dt dt--responsive">
      <thead>
        <tr>
          <th>名称</th>
          <th>Site</th>
          <th>IP</th>
          <th>Domain Suffix</th>
          <th>IP CIDR</th>
          <th>出站</th>
          <th class="is-numeric">排序</th>
          <th class="is-numeric">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in config.customRules" :key="row.ID">
          <td><span class="cell-name">{{ row.Name }}</span></td>
          <td data-label="Site">
            <Tag v-for="t in row.SiteTags" :key="t" class="mr-1">{{ t }}</Tag>
            <span v-if="!row.SiteTags?.length" class="cell-none">—</span>
          </td>
          <td data-label="IP">
            <Tag v-for="t in row.IPTags" :key="t" class="mr-1">{{ t }}</Tag>
            <span v-if="!row.IPTags?.length" class="cell-none">—</span>
          </td>
          <td data-label="Domain"><span class="mono cell-meta cell-wrap">{{ row.DomainSuffix?.join(', ') || '—' }}</span></td>
          <td data-label="IP CIDR"><span class="mono cell-meta cell-wrap">{{ row.IPCIDR?.join(', ') || '—' }}</span></td>
          <td data-label="出站"><span class="mono cell-out">{{ row.OutboundLiteral || groupName(row.OutboundGroupID) }}</span></td>
          <td class="is-numeric" data-label="排序"><span class="num cell-meta">{{ row.SortOrder }}</span></td>
          <td class="is-numeric dt-actions">
            <div class="row-actions">
              <button class="row-actions__btn" @click="onEdit(row)" title="编辑">
                <Pencil :size="14" :stroke-width="1.6" />
              </button>
              <button class="row-actions__btn row-actions__btn--danger" @click="onDelete(row)" title="删除">
                <Trash2 :size="14" :stroke-width="1.6" />
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>

    <div v-if="!config.customRules?.length" class="empty-state">
      <p class="empty-state__title">还没有自定义规则</p>
      <p class="empty-state__hint">用自定义规则覆盖远程规则集没覆盖到的域名 / IP 段。</p>
    </div>

    <CustomRuleEditDialog v-model="editing" :groups="config.groups" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Pencil, Trash2 } from 'lucide-vue-next'
import { Button, Tag, toast, confirm } from '../../../ui'
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
    ID: 0, Name: '',
    SiteTags: [], IPTags: [],
    DomainSuffix: [], DomainKeyword: [], IPCIDR: [], SrcIPCIDR: [],
    Protocol: '', Port: '',
    OutboundGroupID: props.config.groups[0]?.ID ?? null,
    OutboundLiteral: '', SortOrder: 100,
  }
}

function onEdit(row: CustomRule) {
  editing.value = JSON.parse(JSON.stringify(row))
}

async function onSave(row: CustomRule) {
  if (row.ID === 0) await createCustomRule(row)
  else await updateCustomRule(row.ID, row)
  editing.value = null
  toast.success('已保存')
  emit('refresh')
}

async function onDelete(row: CustomRule) {
  try {
    await confirm({ message: `删除规则 ${row.Name}？`, tone: 'danger', confirmText: '删除' })
  } catch { return }
  await deleteCustomRule(row.ID)
  emit('refresh')
}
</script>

<style scoped>
.rules { display: flex; flex-direction: column; gap: 16px; }
.rules__bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.rules__hint { margin: 0; font-size: 13px; color: var(--color-ink-muted); }

.cell-name { font-weight: 600; color: var(--color-ink-strong); }
.cell-meta { color: var(--color-ink-muted); font-size: 12px; }
.cell-none { color: var(--color-ink-soft); font-size: 12px; }
.cell-out { color: var(--color-accent-ink); font-weight: 600; font-size: 13px; }
.cell-wrap { white-space: normal; word-break: break-all; }
.mr-1 { margin-right: 4px; }

@media (max-width: 1023px) {
  .rules__bar { flex-direction: column; align-items: stretch; gap: 10px; }
  .rules__hint { font-size: 12px; }
  /* Mono cells with long domain/CIDR lists wrap on the value column */
  .dt--responsive tbody td > .cell-wrap { text-align: right; }
}
</style>
