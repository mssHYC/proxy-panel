<template>
  <Modal :open="!!modelValue" :width="640" title="编辑分类" @update:open="(v) => !v && emit('update:modelValue', null)">
    <template v-if="modelValue">
      <Field label="Code" layout="row">
        <Input v-model="modelValue.Code" :disabled="readonly" />
      </Field>
      <Field label="显示名" layout="row">
        <Input v-model="modelValue.DisplayName" :disabled="readonly" />
      </Field>
      <Field label="Site Tags" hint="如 google, youtube" layout="row">
        <TagInput v-model="modelValue.SiteTags" :disabled="readonly" placeholder="输入并回车添加" />
      </Field>
      <Field label="IP Tags" layout="row">
        <TagInput v-model="modelValue.IPTags" :disabled="readonly" />
      </Field>
      <Field label="内联 domain_suffix" layout="row">
        <TagInput v-model="modelValue.InlineDomainSuffix" :disabled="readonly" />
      </Field>
      <Field label="内联 domain_keyword" layout="row">
        <TagInput v-model="modelValue.InlineDomainKeyword" :disabled="readonly" />
      </Field>
      <Field label="内联 ip_cidr" layout="row">
        <TagInput v-model="modelValue.InlineIPCIDR" :disabled="readonly" />
      </Field>
      <Field label="默认出站组" layout="row">
        <Select
          :model-value="modelValue.DefaultGroupID"
          :options="groups.map(g => ({ label: g.DisplayName, value: g.ID }))"
          @update:model-value="(v) => (modelValue!.DefaultGroupID = v as number)"
        />
      </Field>
      <Field label="排序" layout="row">
        <NumberInput v-model="modelValue.SortOrder" />
      </Field>
    </template>
    <template #footer>
      <Button @click="emit('update:modelValue', null)">取消</Button>
      <Button variant="primary" @click="emit('save', modelValue!)">保存</Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { Button, Input, NumberInput, Select, Modal, Field } from '../../../ui'
import TagInput from './TagInput.vue'
import type { Category, Group } from './types'

defineProps<{ modelValue: Category | null; readonly: boolean; groups: Group[] }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: Category | null): void
  (e: 'save', v: Category): void
}>()
</script>
