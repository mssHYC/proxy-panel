<template>
  <el-dialog
    :model-value="!!modelValue"
    title="编辑分类"
    width="640px"
    @update:model-value="$emit('update:modelValue', null)"
  >
    <el-form v-if="modelValue" label-width="160px">
      <el-form-item label="Code">
        <el-input v-model="modelValue.Code" :disabled="readonly" />
      </el-form-item>
      <el-form-item label="显示名">
        <el-input v-model="modelValue.DisplayName" :disabled="readonly" />
      </el-form-item>
      <el-form-item label="Site Tags">
        <el-select
          v-model="modelValue.SiteTags"
          :disabled="readonly"
          multiple
          filterable
          allow-create
          placeholder="如 google, youtube"
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="IP Tags">
        <el-select
          v-model="modelValue.IPTags"
          :disabled="readonly"
          multiple
          filterable
          allow-create
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="内联 domain_suffix">
        <el-select
          v-model="modelValue.InlineDomainSuffix"
          :disabled="readonly"
          multiple
          filterable
          allow-create
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="内联 domain_keyword">
        <el-select
          v-model="modelValue.InlineDomainKeyword"
          :disabled="readonly"
          multiple
          filterable
          allow-create
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="内联 ip_cidr">
        <el-select
          v-model="modelValue.InlineIPCIDR"
          :disabled="readonly"
          multiple
          filterable
          allow-create
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="默认出站组">
        <el-select v-model="modelValue.DefaultGroupID" style="width: 100%">
          <el-option v-for="g in groups" :key="g.ID" :label="g.DisplayName" :value="g.ID" />
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
import type { Category, Group } from './types'

defineProps<{
  modelValue: Category | null
  readonly: boolean
  groups: Group[]
}>()

defineEmits<{
  (e: 'update:modelValue', v: Category | null): void
  (e: 'save', v: Category): void
}>()
</script>
