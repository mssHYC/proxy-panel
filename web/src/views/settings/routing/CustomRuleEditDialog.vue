<template>
  <el-dialog
    :model-value="!!modelValue"
    title="编辑自定义规则"
    width="720px"
    @update:model-value="$emit('update:modelValue', null)"
  >
    <el-form v-if="modelValue" label-width="140px">
      <el-form-item label="名称">
        <el-input v-model="modelValue.Name" />
      </el-form-item>
      <el-form-item label="Site Tags">
        <el-select v-model="modelValue.SiteTags" multiple filterable allow-create style="width: 100%" />
      </el-form-item>
      <el-form-item label="IP Tags">
        <el-select v-model="modelValue.IPTags" multiple filterable allow-create style="width: 100%" />
      </el-form-item>
      <el-form-item label="Domain Suffix">
        <el-select v-model="modelValue.DomainSuffix" multiple filterable allow-create style="width: 100%" />
      </el-form-item>
      <el-form-item label="Domain Keyword">
        <el-select v-model="modelValue.DomainKeyword" multiple filterable allow-create style="width: 100%" />
      </el-form-item>
      <el-form-item label="IP CIDR">
        <el-select v-model="modelValue.IPCIDR" multiple filterable allow-create style="width: 100%" />
      </el-form-item>
      <el-form-item label="出站">
        <el-radio-group v-model="outboundMode">
          <el-radio label="group">出站组</el-radio>
          <el-radio label="literal">字面量</el-radio>
        </el-radio-group>
        <el-select v-if="outboundMode === 'group'" v-model="modelValue.OutboundGroupID" style="width: 100%; margin-top: 8px">
          <el-option v-for="g in groups" :key="g.ID" :label="g.DisplayName" :value="g.ID" />
        </el-select>
        <el-select v-else v-model="modelValue.OutboundLiteral" style="width: 100%; margin-top: 8px">
          <el-option label="DIRECT" value="DIRECT" />
          <el-option label="REJECT" value="REJECT" />
        </el-select>
      </el-form-item>
      <el-form-item label="排序">
        <el-input-number v-model="modelValue.SortOrder" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:modelValue', null)">取消</el-button>
      <el-button type="primary" @click="onSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { CustomRule, Group } from './types'

const props = defineProps<{ modelValue: CustomRule | null; groups: Group[] }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: CustomRule | null): void
  (e: 'save', v: CustomRule): void
}>()

const outboundMode = ref<'group' | 'literal'>('group')

watch(
  () => props.modelValue,
  (v) => {
    if (!v) return
    outboundMode.value = v.OutboundLiteral ? 'literal' : 'group'
  },
)

function onSave() {
  if (!props.modelValue) return
  const v = { ...props.modelValue }
  if (outboundMode.value === 'group') v.OutboundLiteral = ''
  else v.OutboundGroupID = null
  emit('save', v)
}
</script>
