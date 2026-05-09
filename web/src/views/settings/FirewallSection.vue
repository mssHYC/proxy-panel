<template>
  <div :class="['s-blocks', { 'is-loading-overlay': loading }]">
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">防火墙</h3>
        <p class="s-block__hint">
          自动同步节点端口到系统防火墙。「保存」仅写入配置；「立即应用」对齐运行中的服务并补齐存量端口。
          关闭后不会主动回收已存在的规则。
        </p>
      </div>

      <Field label="自动同步" hint="启用后，新增 / 编辑 / 删除节点时自动放行端口" layout="row">
        <Switch v-model="enable" />
      </Field>

      <Field label="后端" hint="预检会调用对应命令验证可用性，不会修改任何规则" layout="row">
        <div class="fw-row">
          <Select
            :model-value="backend"
            :options="[{ label: 'ufw', value: 'ufw' }, { label: 'firewalld', value: 'firewalld' }]"
            :disabled="!enable"
            placeholder="选择 backend"
            class="fw-row__sel"
            @update:model-value="(v) => (backend = v as 'ufw' | 'firewalld' | '')"
          />
          <Button :disabled="!backend" :loading="probing" @click="handleProbe">预检后端</Button>
        </div>
      </Field>

      <div class="s-actions">
        <Button :loading="applying" :disabled="!saved" @click="handleApply">立即应用 · 对齐存量端口</Button>
        <Button variant="primary" :loading="saving" @click="handleSave">保存防火墙设置</Button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Button, Switch, Select, Field, toast, confirm } from '../../ui'
import { getSettings, updateSettings, probeFirewall, applyFirewall } from '../../api/setting'

const loading = ref(false)
const saving = ref(false)
const applying = ref(false)
const probing = ref(false)
const saved = ref(true)

const enable = ref(false)
const backend = ref<'ufw' | 'firewalld' | ''>('')

async function fetchState() {
  loading.value = true
  try {
    const { data } = await getSettings()
    enable.value = data.firewall_enable === 'true'
    backend.value = (data.firewall_backend || '') as any
  } catch (e) { console.error('加载防火墙设置失败', e) }
  finally { loading.value = false }
}

async function handleProbe() {
  if (!backend.value) return
  probing.value = true
  try {
    const { data } = await probeFirewall(backend.value)
    if (data.ok) toast.success(data.message || '后端可用')
    else toast.error(data.message || '后端不可用')
  } catch (e: any) { toast.error(e.response?.data?.error || '预检失败') }
  finally { probing.value = false }
}

async function handleSave() {
  if (enable.value && !backend.value) { toast.error('启用防火墙时必须选择 backend'); return }
  saving.value = true
  try {
    await updateSettings({
      firewall_enable: String(enable.value),
      firewall_backend: backend.value || '',
    })
    saved.value = true
    toast.success('设置已保存，点击「立即应用」可即时生效')
  } catch (e: any) { toast.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

async function handleApply() {
  try {
    await confirm({
      title: '确认应用',
      message: '立即应用将热替换运行中的防火墙服务，并对存量 enable 节点端口做一次对齐。关闭时不会主动回收已有规则。确定继续？',
      tone: 'danger',
      confirmText: '应用',
    })
  } catch { return }
  applying.value = true
  try {
    const { data } = await applyFirewall()
    toast.success(`已立即应用：enabled=${data.enabled}, backend=${data.backend || '-'}, 对齐端口=${data.applied_ports}`)
  } catch (e: any) {
    toast.error(e.response?.data?.error || '应用失败')
  } finally { applying.value = false }
}

onMounted(fetchState)
</script>

<style scoped>
.fw-row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.fw-row__sel { width: 200px; }
</style>
