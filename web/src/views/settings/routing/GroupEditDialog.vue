<template>
  <Modal :open="!!modelValue" :width="640" title="编辑出站组" @update:open="(v) => !v && emit('update:modelValue', null)">
    <template v-if="modelValue">
      <Field label="Code" layout="row">
        <Input v-model="modelValue.Code" :disabled="readonlyCode" />
      </Field>
      <Field label="显示名" layout="row">
        <Input v-model="modelValue.DisplayName" />
      </Field>
      <Field label="类型" layout="row">
        <Select
          :model-value="modelValue.Type"
          :options="[{ label: 'selector', value: 'selector' }, { label: 'urltest', value: 'urltest' }]"
          :disabled="readonlyCode"
          @update:model-value="(v) => (modelValue!.Type = String(v) as 'selector' | 'urltest')"
        />
      </Field>
      <Field label="成员" hint="支持 <ALL> / DIRECT / REJECT / 节点名 / 其他出站组 Code" layout="row">
        <TagInput v-model="modelValue.Members" />
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
import type { Group } from './types'

defineProps<{ modelValue: Group | null; readonlyCode: boolean; groups: Group[] }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: Group | null): void
  (e: 'save', v: Group): void
}>()
</script>
