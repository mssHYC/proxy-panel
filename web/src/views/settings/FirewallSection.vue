<template>
  <div v-loading="loading" class="space-y-4">
    <el-alert type="info" :closable="false" show-icon>
      <template #title>
        保存设置仅写入配置；如需立即对齐运行中面板，请在保存后点击"立即应用"。
        关闭后不会主动回收已存在的防火墙规则。
      </template>
    </el-alert>

    <el-card shadow="hover">
      <template #header><span class="font-bold">防火墙设置</span></template>
      <el-form label-width="180px">
        <el-form-item label="启用节点端口自动同步">
          <el-switch v-model="enable" />
        </el-form-item>
        <el-form-item label="后端">
          <el-select v-model="backend" :disabled="!enable" style="width: 200px">
            <el-option value="ufw" label="ufw" />
            <el-option value="firewalld" label="firewalld" />
          </el-select>
          <el-button :disabled="!backend" :loading="probing" class="!ml-2" @click="handleProbe">
            预检后端
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <div class="flex justify-end gap-2">
      <el-button :loading="applying" :disabled="!saved" @click="handleApply">
        立即应用（对齐存量端口）
      </el-button>
      <el-button type="primary" size="large" :loading="saving" @click="handleSave">
        保存防火墙设置
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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
  } catch (e) {
    console.error('加载防火墙设置失败', e)
  } finally {
    loading.value = false
  }
}

async function handleProbe() {
  if (!backend.value) return
  probing.value = true
  try {
    const { data } = await probeFirewall(backend.value)
    if (data.ok) {
      ElMessage.success(data.message || '后端可用')
    } else {
      ElMessage.error(data.message || '后端不可用')
    }
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '预检失败')
  } finally {
    probing.value = false
  }
}

async function handleSave() {
  if (enable.value && !backend.value) {
    ElMessage.error('启用防火墙时必须选择 backend')
    return
  }
  saving.value = true
  try {
    await updateSettings({
      firewall_enable: String(enable.value),
      firewall_backend: backend.value || '',
    })
    saved.value = true
    ElMessage.success('设置已保存，点击"立即应用"可即时生效')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleApply() {
  try {
    await ElMessageBox.confirm(
      '立即应用将热替换运行中的防火墙服务，并对存量 enable 节点端口做一次对齐。关闭时不会主动回收已有规则。确定继续？',
      '确认应用',
      { type: 'warning' }
    )
  } catch {
    return
  }
  applying.value = true
  try {
    const { data } = await applyFirewall()
    ElMessage.success(
      `已立即应用：enabled=${data.enabled}, backend=${data.backend || '-'}, 对齐端口=${data.applied_ports}`
    )
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '应用失败')
  } finally {
    applying.value = false
  }
}

onMounted(fetchState)
</script>
