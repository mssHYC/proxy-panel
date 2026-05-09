<template>
  <Modal :open="!!modelValue" :width="720" title="编辑自定义规则" @update:open="(v) => !v && emit('update:modelValue', null)">
    <template v-if="modelValue">
      <Field label="名称" layout="row">
        <Input v-model="modelValue.Name" />
      </Field>
      <Field label="Site Tags" layout="row"><TagInput v-model="modelValue.SiteTags" /></Field>
      <Field label="IP Tags" layout="row"><TagInput v-model="modelValue.IPTags" /></Field>
      <Field label="Domain Suffix" layout="row"><TagInput v-model="modelValue.DomainSuffix" /></Field>
      <Field label="Domain Keyword" layout="row"><TagInput v-model="modelValue.DomainKeyword" /></Field>
      <Field label="IP CIDR" layout="row"><TagInput v-model="modelValue.IPCIDR" /></Field>
      <Field label="出站" layout="row">
        <RadioGroup
          :model-value="outboundMode"
          :options="[{ label: '出站组', value: 'group' }, { label: '字面量', value: 'literal' }]"
          @update:model-value="(v) => (outboundMode = v as 'group' | 'literal')"
        />
        <div class="outbound-pick">
          <Select
            v-if="outboundMode === 'group'"
            :model-value="modelValue.OutboundGroupID"
            :options="groups.map(g => ({ label: g.DisplayName, value: g.ID }))"
            @update:model-value="(v) => (modelValue!.OutboundGroupID = v as number)"
          />
          <Select
            v-else
            :model-value="modelValue.OutboundLiteral"
            :options="[{ label: 'DIRECT', value: 'DIRECT' }, { label: 'REJECT', value: 'REJECT' }]"
            @update:model-value="(v) => (modelValue!.OutboundLiteral = String(v))"
          />
        </div>
      </Field>
      <Field label="排序" layout="row">
        <NumberInput v-model="modelValue.SortOrder" />
      </Field>
    </template>
    <template #footer>
      <Button @click="emit('update:modelValue', null)">取消</Button>
      <Button variant="primary" @click="onSave">保存</Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Button, Input, NumberInput, Select, RadioGroup, Modal, Field } from '../../../ui'
import TagInput from './TagInput.vue'
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

<style scoped>
.outbound-pick { margin-top: 8px; }
</style>
