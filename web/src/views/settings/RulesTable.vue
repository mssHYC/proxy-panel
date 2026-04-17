<template>
  <div>
    <!-- 表头 -->
    <div class="grid gap-2 items-center px-2 pb-2 text-xs text-gray-500 font-semibold border-b"
         :style="{ gridTemplateColumns: GRID_TEMPLATE }">
      <span></span>
      <span>类型</span>
      <span>值</span>
      <span>目标组</span>
      <span>no-resolve</span>
      <span></span>
    </div>

    <!-- 拖拽列表 -->
    <draggable
      :model-value="rules"
      @update:model-value="$emit('update:rules', $event)"
      item-key="__key"
      handle=".drag-handle"
      animation="150"
    >
      <template #item="{ element, index }">
        <div class="grid gap-2 items-center px-2 py-2 border-b border-dashed"
             :style="{ gridTemplateColumns: GRID_TEMPLATE }">
          <!-- 拖拽手柄 -->
          <span class="drag-handle cursor-grab text-gray-400 text-center">☰</span>

          <!-- 类型 -->
          <el-tag v-if="element.type === 'UNKNOWN'" type="info" size="small">未知</el-tag>
          <el-select v-else
            :model-value="element.type"
            @update:model-value="updateType(index, $event)"
            size="small">
            <el-option v-for="t in RULE_TYPES" :key="t" :label="t" :value="t" />
          </el-select>

          <!-- 值 -->
          <el-input v-if="element.type === 'UNKNOWN'"
            :model-value="element.raw"
            readonly size="small" class="font-mono" />
          <el-input v-else
            :model-value="element.value"
            @update:model-value="(v: string) => updateField(index, 'value', v)"
            size="small" placeholder="如 example.com / 1.2.3.0/24" />

          <!-- 目标组 -->
          <el-select v-if="element.type === 'UNKNOWN'"
            model-value="—" disabled size="small">
            <el-option value="—" label="—" />
          </el-select>
          <el-select v-else
            :model-value="element.target"
            @update:model-value="(v: string) => updateField(index, 'target', v)"
            size="small">
            <el-option v-for="g in TARGET_GROUPS" :key="g" :label="g" :value="g" />
          </el-select>

          <!-- no-resolve -->
          <el-switch
            :model-value="element.noResolve"
            @update:model-value="(v: boolean) => updateField(index, 'noResolve', v)"
            :disabled="element.type === 'UNKNOWN' || !IP_RULE_TYPES.includes(element.type)"
            size="small" />

          <!-- 删除 -->
          <el-button type="danger" size="small" link @click="remove(index)">
            <el-icon><Delete /></el-icon>
          </el-button>
        </div>
      </template>
    </draggable>

    <div class="mt-3">
      <el-button size="small" @click="addRow">
        <el-icon><Plus /></el-icon>
        <span class="ml-1">添加规则</span>
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import draggable from 'vuedraggable'
import { Delete, Plus } from '@element-plus/icons-vue'
import { RULE_TYPES, TARGET_GROUPS, IP_RULE_TYPES, type Rule } from './rules-types'

const GRID_TEMPLATE = '32px 160px 1fr 160px 90px 40px'

const props = defineProps<{ rules: Rule[] }>()
const emit = defineEmits<{ 'update:rules': [rules: Rule[]] }>()

function updateType(index: number, newType: unknown) {
  const next = [...props.rules]
  const row = { ...next[index], type: newType as Rule['type'] }
  // 切到非 IP 类型时，强制 noResolve = false
  if (!IP_RULE_TYPES.includes(newType as string)) {
    row.noResolve = false
  }
  next[index] = row
  emit('update:rules', next)
}

function updateField<K extends keyof Rule>(index: number, key: K, value: Rule[K]) {
  const next = [...props.rules]
  const row = { ...next[index], [key]: value }
  next[index] = row
  emit('update:rules', next)
}

function addRow() {
  emit('update:rules', [
    ...props.rules,
    { type: 'DOMAIN-SUFFIX', value: '', target: '全球代理', noResolve: false },
  ])
}

function remove(index: number) {
  const next = [...props.rules]
  next.splice(index, 1)
  emit('update:rules', next)
}
</script>
