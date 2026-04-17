<template>
  <div v-loading="loading" class="space-y-4">
    <el-card shadow="hover">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">自定义分流规则</span>
          <el-tag type="info" size="small">优先于默认规则执行</el-tag>
        </div>
      </template>

      <!-- 规则模式 -->
      <el-form label-width="100px" class="mb-4">
        <el-form-item label="规则模式">
          <el-radio-group v-model="mode">
            <el-radio value="prepend">
              追加模式
              <span class="text-xs text-gray-400 ml-1">（自定义规则 + 默认规则）</span>
            </el-radio>
            <el-radio value="override">
              完全自定义
              <span class="text-xs text-gray-400 ml-1">（仅使用下方规则，忽略默认规则）</span>
            </el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>

      <el-alert
        v-if="mode === 'override'"
        type="warning" :closable="false" show-icon class="mb-4"
        description="完全自定义模式：默认的分流规则和 rule-provider 将不会生成，所有分流逻辑完全由下方规则控制。" />
      <el-alert
        v-else
        type="info" :closable="false" show-icon class="mb-4"
        description="追加模式：下方的自定义规则会插入到默认规则之前优先匹配，Clash / Surge / Sing-box 订阅同步生效。" />

      <!-- 视图切换 -->
      <div class="flex items-center gap-2 mb-3">
        <el-radio-group v-model="viewMode" size="small" @change="onViewModeChange">
          <el-radio-button value="table">表格</el-radio-button>
          <el-radio-button value="text">高级（文本）</el-radio-button>
        </el-radio-group>
        <span class="ml-auto text-xs text-gray-400">{{ rules.length }} 条规则</span>
      </div>

      <!-- 表格视图 -->
      <RulesTable v-if="viewMode === 'table'" v-model:rules="rules" />

      <!-- 文本视图 -->
      <el-input v-else
        v-model="rawText"
        type="textarea" :rows="12"
        placeholder="每行一条规则，示例：
DOMAIN-SUFFIX,example.com,全球代理
IP-CIDR,1.2.3.0/24,本地直连,no-resolve"
        class="font-mono" />

      <div class="mt-2 text-xs text-gray-400">
        可用策略组：{{ TARGET_GROUPS.join(' / ') }}
      </div>
    </el-card>

    <div class="flex justify-end">
      <el-button type="primary" size="large" :loading="saving" @click="handleSave">
        保存分流规则
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getSettings, updateSettings } from '../../api/setting'
import RulesTable from './RulesTable.vue'
import { parseRules, serializeRules, TARGET_GROUPS, type Rule } from './rules-types'

const loading = ref(false)
const saving = ref(false)
const rules = ref<Rule[]>([])
const mode = ref<'prepend' | 'override'>('prepend')
const viewMode = ref<'table' | 'text'>('table')
const rawText = ref('')

// 切 view 时双向同步 rules[] 和 rawText
function onViewModeChange(v: string | number | boolean | undefined) {
  if (v === 'text') {
    rawText.value = serializeRules(rules.value)
  } else {
    rules.value = parseRules(rawText.value)
  }
}

async function fetchState() {
  loading.value = true
  try {
    const { data } = await getSettings()
    const map: Record<string, string> = {}
    if (Array.isArray(data)) {
      data.forEach((item: any) => { map[item.key] = item.value })
    } else if (data.settings) {
      if (Array.isArray(data.settings)) {
        data.settings.forEach((item: any) => { map[item.key] = item.value })
      } else {
        Object.assign(map, data.settings)
      }
    } else {
      Object.assign(map, data)
    }
    const text = map.custom_rules || ''
    rules.value = parseRules(text)
    rawText.value = text
    mode.value = (map.custom_rules_mode as 'prepend' | 'override') || 'prepend'
  } catch (e) {
    console.error('加载规则失败', e)
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    // 如果当前是文本模式，以 rawText 为准；否则以 rules[] 为准
    const payload = viewMode.value === 'text'
      ? rawText.value
      : serializeRules(rules.value)
    await updateSettings({
      custom_rules: payload,
      custom_rules_mode: mode.value,
    })
    // 保存后以后端为准刷新一次，防止本地与服务器不同步
    await fetchState()
    ElMessage.success('分流规则已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(fetchState)
</script>
