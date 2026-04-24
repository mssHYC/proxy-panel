<template>
  <div>
    <h3>URL 前缀覆写</h3>
    <el-form label-width="300px" style="max-width: 900px">
      <el-form-item label="Clash geosite (.mrs) 前缀">
        <el-input v-model="s['routing.site_ruleset_base_url.clash']" @change="save('routing.site_ruleset_base_url.clash')" />
      </el-form-item>
      <el-form-item label="Clash geoip (.mrs) 前缀">
        <el-input v-model="s['routing.ip_ruleset_base_url.clash']" @change="save('routing.ip_ruleset_base_url.clash')" />
      </el-form-item>
      <el-form-item label="Sing-box geosite (.srs) 前缀">
        <el-input v-model="s['routing.site_ruleset_base_url.singbox']" @change="save('routing.site_ruleset_base_url.singbox')" />
      </el-form-item>
      <el-form-item label="Sing-box geoip (.srs) 前缀">
        <el-input v-model="s['routing.ip_ruleset_base_url.singbox']" @change="save('routing.ip_ruleset_base_url.singbox')" />
      </el-form-item>
      <el-form-item label="Surge/Shadowrocket site 前缀（空=降级 GEOSITE）">
        <el-input v-model="s['routing.surge_site_ruleset_base_url']" @change="save('routing.surge_site_ruleset_base_url')" />
      </el-form-item>
      <el-form-item label="兜底出站组">
        <el-select v-model="s['routing.final_outbound']" @change="save('routing.final_outbound')" style="width: 300px">
          <el-option v-for="g in config.groups" :key="g.Code" :label="g.DisplayName" :value="g.Code" />
        </el-select>
      </el-form-item>
    </el-form>

    <h3 style="margin-top: 24px">从旧格式导入</h3>
    <el-form label-width="120px" style="max-width: 900px">
      <el-form-item label="旧规则文本">
        <el-input v-model="legacyText" type="textarea" :rows="10" placeholder="每行 TYPE,VALUE,OUTBOUND" />
      </el-form-item>
      <el-form-item label="模式">
        <el-select v-model="legacyMode" style="width: 200px">
          <el-option label="追加（保留启用分类）" value="prepend" />
          <el-option label="覆盖（关闭所有系统分类）" value="override" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="onImport">导入</el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
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
  ElMessage.success('已保存')
}

async function onImport() {
  const res = await importLegacy(legacyText.value, legacyMode.value)
  ElMessage.success(`导入 ${(res as any)?.data?.imported ?? '?'} 条`)
  legacyText.value = ''
  emit('refresh')
}
</script>
