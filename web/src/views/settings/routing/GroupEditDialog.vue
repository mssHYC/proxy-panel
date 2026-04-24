<template>
  <el-dialog
    :model-value="!!modelValue"
    title="编辑出站组"
    width="640px"
    @update:model-value="$emit('update:modelValue', null)"
  >
    <el-form v-if="modelValue" label-width="120px">
      <el-form-item label="Code">
        <el-input v-model="modelValue.Code" :disabled="readonlyCode" />
      </el-form-item>
      <el-form-item label="显示名">
        <el-input v-model="modelValue.DisplayName" />
      </el-form-item>
      <el-form-item label="类型">
        <el-select v-model="modelValue.Type" :disabled="readonlyCode">
          <el-option label="selector" value="selector" />
          <el-option label="urltest" value="urltest" />
        </el-select>
      </el-form-item>
      <el-form-item label="成员">
        <el-select v-model="modelValue.Members" multiple filterable allow-create style="width: 100%">
          <el-option label="<ALL> (全部节点)" value="<ALL>" />
          <el-option label="DIRECT" value="DIRECT" />
          <el-option label="REJECT" value="REJECT" />
          <el-option
            v-for="g in groups"
            :key="g.Code"
            :label="g.DisplayName + ' (' + g.Code + ')'"
            :value="g.Code"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="排序">
        <el-input-number v-model="modelValue.SortOrder" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:modelValue', null)">取消</el-button>
      <el-button type="primary" @click="$emit('save', modelValue!)">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { Group } from './types'

defineProps<{
  modelValue: Group | null
  readonlyCode: boolean
  groups: Group[]
}>()

defineEmits<{
  (e: 'update:modelValue', v: Group | null): void
  (e: 'save', v: Group): void
}>()
</script>
