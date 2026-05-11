<template>
  <div class="s-blocks">
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">规则集前缀</h3>
        <p class="s-block__hint">控制客户端从哪里下载规则集文件。默认走 GitHub 加速镜像，多数情况下不需要修改。</p>
      </div>
      <Field label="Clash geosite" hint=".mrs" layout="row">
        <Input v-model="s['routing.site_ruleset_base_url.clash']" class="adv-mono" @blur="save('routing.site_ruleset_base_url.clash')" />
      </Field>
      <Field label="Clash geoip" hint=".mrs" layout="row">
        <Input v-model="s['routing.ip_ruleset_base_url.clash']" class="adv-mono" @blur="save('routing.ip_ruleset_base_url.clash')" />
      </Field>
      <Field label="Sing-box geosite" hint=".srs" layout="row">
        <Input v-model="s['routing.site_ruleset_base_url.singbox']" class="adv-mono" @blur="save('routing.site_ruleset_base_url.singbox')" />
      </Field>
      <Field label="Sing-box geoip" hint=".srs" layout="row">
        <Input v-model="s['routing.ip_ruleset_base_url.singbox']" class="adv-mono" @blur="save('routing.ip_ruleset_base_url.singbox')" />
      </Field>
      <Field label="Surge / Shadowrocket site" hint="留空降级 GEOSITE" layout="row">
        <Input v-model="s['routing.surge_site_ruleset_base_url']" class="adv-mono" @blur="save('routing.surge_site_ruleset_base_url')" />
      </Field>
      <Field label="兜底出站组" hint="所有规则未命中的流量去向" layout="row">
        <Select
          :model-value="s['routing.final_outbound']"
          :options="config.groups.map(g => ({ label: g.DisplayName, value: g.Code }))"
          class="adv-final"
          @update:model-value="(v) => { s['routing.final_outbound'] = String(v); save('routing.final_outbound') }"
        />
      </Field>
    </section>

    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">从旧格式导入</h3>
        <p class="s-block__hint">每行 <code>TYPE,VALUE,OUTBOUND</code>，例如 <code>DOMAIN-SUFFIX,google.com,Google</code>。</p>
      </div>
      <Field label="规则文本" layout="row">
        <Textarea v-model="legacyText" :rows="10" mono placeholder="DOMAIN-SUFFIX,google.com,Google&#10;DOMAIN-KEYWORD,spotify,DIRECT&#10;IP-CIDR,91.108.0.0/16,Telegram" />
      </Field>
      <Field label="导入模式" layout="row">
        <Select
          :model-value="legacyMode"
          :options="[
            { label: '追加（保留启用分类）', value: 'prepend' },
            { label: '覆盖（关闭所有系统分类）', value: 'override' },
          ]"
          class="legacy-mode"
          @update:model-value="(v) => (legacyMode = v as 'prepend' | 'override')"
        />
      </Field>
      <div class="s-actions">
        <Button variant="primary" :disabled="!legacyText.trim()" @click="onImport">导入</Button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { Button, Input, Textarea, Select, Field, toast } from '../../../ui'
import { importLegacy } from '../../../api/routing'
import { updateSettings } from '../../../api/setting'
import type { RoutingConfig } from './types'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()
const s = reactive<Record<string, string>>({ ...props.config.settings })
watch(
  () => props.config.settings,
  (v) => { Object.assign(s, v) },
  { deep: true },
)
const legacyText = ref('')
const legacyMode = ref<'prepend' | 'override'>('prepend')

async function save(key: string) {
  await updateSettings({ [key]: s[key] })
  toast.success('已保存')
}

async function onImport() {
  const res = await importLegacy(legacyText.value, legacyMode.value)
  toast.success(`导入 ${(res as any)?.data?.imported ?? '?'} 条`)
  legacyText.value = ''
  emit('refresh')
}
</script>

<style scoped>
.adv-mono :deep(.input__field) { font-family: var(--font-mono); font-size: 13px; }
.adv-final { width: 240px; max-width: 100%; }
.legacy-mode { width: 280px; max-width: 100%; }
code {
  background: var(--color-surface-sunken);
  padding: 1px 6px;
  border-radius: 3px;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-ink-base);
}
</style>
