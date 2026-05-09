<template>
  <div :class="['s-blocks', { 'is-loading-overlay': loading }]">
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">流量告警</h3>
        <p class="s-block__hint">服务器总流量达到阈值时推送预警；按周期重置后告警标记自动清零。</p>
      </div>
      <Field label="预警阈值" hint="百分比" layout="row">
        <NumberInput v-model="warnPercent" :min="1" :max="100" :step="5" />
      </Field>
      <Field label="采集间隔" hint="秒，太短会增加 CPU 负担" layout="row">
        <NumberInput v-model="collectInterval" :min="10" :step="10" />
      </Field>
      <Field label="服务器流量重置" layout="row">
        <div class="reset-row">
          <Select
            :model-value="resetPreset"
            :options="[
              { label: '每月 1 号 00:00', value: 'monthly' },
              { label: '每周一 00:00', value: 'weekly' },
              { label: '每日 00:00', value: 'daily' },
              { label: '自定义', value: 'custom' },
            ]"
            class="reset-row__preset"
            @update:model-value="(v) => onPresetChange(String(v))"
          />
          <Input v-model="resetCron" :disabled="resetPreset !== 'custom'" placeholder="0 0 1 * *" class="reset-row__cron" />
        </div>
        <p class="form-hint">5 段 cron（分 时 日 月 周），与 VPS 计费日对齐可减少误判</p>
      </Field>
      <div class="s-actions">
        <Button variant="primary" :loading="saving" @click="handleSave">保存告警设置</Button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Button, Input, NumberInput, Select, Field, toast } from '../../ui'
import { getSettings, updateSettings } from '../../api/setting'

const loading = ref(false)
const saving = ref(false)
const warnPercent = ref(80)
const collectInterval = ref(60)
const resetCron = ref('0 0 1 * *')
const resetPreset = ref<'monthly' | 'weekly' | 'daily' | 'custom'>('monthly')

const PRESET_MAP: Record<string, string> = {
  monthly: '0 0 1 * *',
  weekly:  '0 0 * * 1',
  daily:   '0 0 * * *',
}

function cronToPreset(expr: string): 'monthly' | 'weekly' | 'daily' | 'custom' {
  const trimmed = expr.trim().replace(/\s+/g, ' ')
  for (const [k, v] of Object.entries(PRESET_MAP)) {
    if (v === trimmed) return k as 'monthly' | 'weekly' | 'daily'
  }
  return 'custom'
}

function onPresetChange(val: string) {
  resetPreset.value = val as any
  if (val !== 'custom' && PRESET_MAP[val]) resetCron.value = PRESET_MAP[val]
}

async function fetchState() {
  loading.value = true
  try {
    const { data } = await getSettings()
    const map: Record<string, string> = {}
    if (Array.isArray(data)) data.forEach((item: any) => { map[item.key] = item.value })
    else if (data.settings) {
      if (Array.isArray(data.settings)) data.settings.forEach((item: any) => { map[item.key] = item.value })
      else Object.assign(map, data.settings)
    } else { Object.assign(map, data) }
    warnPercent.value = parseInt(map.warn_percent) || 80
    collectInterval.value = parseInt(map.collect_interval) || 60
    resetCron.value = map.reset_cron || '0 0 1 * *'
    resetPreset.value = cronToPreset(resetCron.value)
  } catch (e) { console.error('加载告警设置失败', e) }
  finally { loading.value = false }
}

async function handleSave() {
  const cron = resetCron.value.trim()
  if (cron.split(/\s+/).length !== 5) {
    toast.error('reset_cron 必须为 5 段格式，例如 0 0 1 * *')
    return
  }
  saving.value = true
  try {
    await updateSettings({
      warn_percent: String(warnPercent.value),
      collect_interval: String(collectInterval.value),
      reset_cron: cron,
    })
    toast.success('告警设置已保存，调度已热生效')
  } catch (e: any) {
    toast.error(e.response?.data?.error || '保存失败')
  } finally { saving.value = false }
}

onMounted(fetchState)
</script>

<style scoped>
.reset-row { display: flex; gap: 8px; flex-wrap: wrap; }
.reset-row__preset { width: 200px; }
.reset-row__cron { width: 220px; }
.reset-row__cron :deep(.input__field) { font-family: var(--font-mono); }
</style>
