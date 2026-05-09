<template>
  <Modal :open="!!modelValue" :width="640" :title="readonly ? '查看分类' : '编辑分类'" @update:open="(v) => !v && emit('update:modelValue', null)">
    <template v-if="modelValue">
      <Alert v-if="readonly" tone="info" class="mb">
        系统分类，仅供查看。如需自定义，请在列表点「新增自定义分类」。
      </Alert>

      <Field label="Code" layout="row">
        <Input v-if="!readonly" v-model="modelValue.Code" />
        <span v-else class="ro-text mono">{{ modelValue.Code }}</span>
      </Field>
      <Field label="显示名" layout="row">
        <Input v-if="!readonly" v-model="modelValue.DisplayName" />
        <span v-else class="ro-text">{{ modelValue.DisplayName }}</span>
      </Field>
      <Field label="Site Tags" hint="如 google, youtube" layout="row">
        <TagInput v-model="modelValue.SiteTags" :disabled="readonly" />
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
        <NumberInput v-model="modelValue.SortOrder" :disabled="readonly" />
      </Field>
    </template>
    <template #footer>
      <Button @click="emit('update:modelValue', null)">{{ readonly ? '关闭' : '取消' }}</Button>
      <Button v-if="!readonly" variant="primary" @click="emit('save', modelValue!)">保存</Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { Button, Input, NumberInput, Select, Modal, Field, Alert } from '../../../ui'
import TagInput from './TagInput.vue'
import type { Category, Group } from './types'

defineProps<{ modelValue: Category | null; readonly: boolean; groups: Group[] }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: Category | null): void
  (e: 'save', v: Category): void
}>()
</script>

<style scoped>
.mb { margin-bottom: 12px; }
.ro-text {
  display: inline-block;
  padding-top: 8px;
  font-size: 14px;
  color: var(--color-ink-strong);
}
.ro-text.mono {
  font-family: var(--font-mono);
  color: var(--color-ink-base);
}
</style>
