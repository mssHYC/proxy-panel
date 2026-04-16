<template>
  <div v-loading="loading" class="p-4 space-y-4">
    <!-- 顶部栏 -->
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-bold">节点管理</h2>
      <el-button type="primary" @click="openDialog()">
        <el-icon class="mr-1"><Plus /></el-icon>新增节点
      </el-button>
    </div>

    <!-- 节点表格 -->
    <el-card shadow="hover">
      <el-table :data="nodes" stripe>
        <el-table-column prop="name" label="节点名称" min-width="140" />
        <el-table-column label="地址" min-width="180">
          <template #default="{ row }">{{ row.host }}:{{ row.port }}</template>
        </el-table-column>
        <el-table-column label="协议" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="protocolColor(row.protocol)">{{ row.protocol }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="传输" width="100">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ row.transport || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="安全" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="securityColor(parsedSettings(row).security)">
              {{ parsedSettings(row).security || 'none' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="内核" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="row.kernel_type === 'xray' ? '' : 'success'">
              {{ row.kernel_type }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <el-switch
              v-model="row.enable"
              :loading="row._switching"
              @change="(val: boolean) => handleToggle(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column prop="sort_order" label="排序" width="70" />
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDialog(row)">编辑</el-button>
            <el-popconfirm title="确认删除该节点？" @confirm="handleDelete(row.id)">
              <template #reference>
                <el-button link type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑节点' : '新增节点'"
      width="620px"
      destroy-on-close
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="100px"
      >
        <!-- 基础信息 -->
        <el-divider content-position="left">基础信息</el-divider>
        <el-form-item label="节点名称" prop="name">
          <el-input v-model="form.name" placeholder="如: Tokyo-01" />
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="16">
            <el-form-item label="主机地址" prop="host">
              <el-input v-model="form.host" placeholder="IP 或域名" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="端口" prop="port">
              <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- 协议 + 内核 -->
        <el-divider content-position="left">协议配置</el-divider>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="协议" prop="protocol">
              <el-select v-model="form.protocol" placeholder="请选择" style="width: 100%" @change="onProtocolChange">
                <el-option v-for="p in protocols" :key="p.value" :label="p.label" :value="p.value" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="内核">
              <el-select v-model="form.kernel_type" style="width: 100%">
                <el-option
                  v-for="k in availableKernels"
                  :key="k"
                  :label="k"
                  :value="k"
                />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- 传输方式 (非 ss / hysteria2) -->
        <el-form-item v-if="showTransport" label="传输方式">
          <el-select v-model="form.transport" style="width: 100%" @change="onTransportChange">
            <el-option
              v-for="t in availableTransports"
              :key="t.value"
              :label="t.label"
              :value="t.value"
            />
          </el-select>
        </el-form-item>

        <!-- 安全设置 (非 reality, 非 ss, 非 hysteria2) -->
        <el-form-item v-if="showSecurity" label="安全">
          <el-select v-model="form.security" style="width: 100%">
            <el-option label="无" value="none" />
            <el-option label="TLS" value="tls" />
          </el-select>
        </el-form-item>

        <!-- TLS 配置 -->
        <template v-if="form.security === 'tls'">
          <el-form-item label="SNI">
            <el-input v-model="form.sni" placeholder="如: example.com" />
          </el-form-item>
          <el-form-item label="跳过验证">
            <el-switch v-model="form.allow_insecure" />
          </el-form-item>
        </template>

        <!-- Reality 配置 -->
        <template v-if="form.transport === 'reality'">
          <el-divider content-position="left">Reality 配置</el-divider>
          <el-form-item label="SNI">
            <el-input v-model="form.sni" placeholder="如: www.google.com" />
          </el-form-item>
          <el-form-item label="目标地址">
            <el-input v-model="form.reality_dest" placeholder="如: www.google.com:443" />
          </el-form-item>
          <el-form-item label="Private Key">
            <el-input v-model="form.reality_private_key" placeholder="xray x25519 生成" />
          </el-form-item>
          <el-form-item label="Public Key">
            <el-input v-model="form.reality_public_key" placeholder="对应的公钥" />
          </el-form-item>
          <el-form-item label="Short ID">
            <el-input v-model="form.reality_short_id" placeholder="如: abcd1234" />
          </el-form-item>
          <el-form-item label="指纹">
            <el-select v-model="form.fingerprint" style="width: 100%">
              <el-option label="chrome" value="chrome" />
              <el-option label="firefox" value="firefox" />
              <el-option label="safari" value="safari" />
              <el-option label="edge" value="edge" />
              <el-option label="random" value="random" />
            </el-select>
          </el-form-item>
          <el-form-item label="Flow">
            <el-select v-model="form.flow" style="width: 100%" clearable>
              <el-option label="无" value="" />
              <el-option label="xtls-rprx-vision" value="xtls-rprx-vision" />
            </el-select>
          </el-form-item>
        </template>

        <!-- WebSocket 配置 -->
        <template v-if="form.transport === 'ws'">
          <el-divider content-position="left">WebSocket 配置</el-divider>
          <el-form-item label="路径">
            <el-input v-model="form.ws_path" placeholder="如: /ws" />
          </el-form-item>
          <el-form-item label="Host">
            <el-input v-model="form.ws_host" placeholder="可选，伪装域名" />
          </el-form-item>
        </template>

        <!-- gRPC 配置 -->
        <template v-if="form.transport === 'grpc'">
          <el-divider content-position="left">gRPC 配置</el-divider>
          <el-form-item label="Service Name">
            <el-input v-model="form.grpc_service_name" placeholder="如: grpc-service" />
          </el-form-item>
        </template>

        <!-- HTTP/2 配置 -->
        <template v-if="form.transport === 'h2'">
          <el-divider content-position="left">HTTP/2 配置</el-divider>
          <el-form-item label="路径">
            <el-input v-model="form.h2_path" placeholder="如: /h2" />
          </el-form-item>
          <el-form-item label="Host">
            <el-input v-model="form.h2_host" placeholder="可选" />
          </el-form-item>
        </template>

        <!-- Shadowsocks 加密方式 -->
        <template v-if="form.protocol === 'ss'">
          <el-divider content-position="left">Shadowsocks 配置</el-divider>
          <el-form-item label="加密方式">
            <el-select v-model="form.ss_method" style="width: 100%">
              <el-option label="aes-256-gcm" value="aes-256-gcm" />
              <el-option label="aes-128-gcm" value="aes-128-gcm" />
              <el-option label="chacha20-ietf-poly1305" value="chacha20-ietf-poly1305" />
              <el-option label="2022-blake3-aes-256-gcm" value="2022-blake3-aes-256-gcm" />
              <el-option label="2022-blake3-aes-128-gcm" value="2022-blake3-aes-128-gcm" />
            </el-select>
          </el-form-item>
        </template>

        <!-- Hysteria2 配置 -->
        <template v-if="form.protocol === 'hysteria2'">
          <el-divider content-position="left">Hysteria2 配置</el-divider>
          <el-form-item label="SNI">
            <el-input v-model="form.sni" placeholder="可选" />
          </el-form-item>
          <el-form-item label="混淆类型">
            <el-select v-model="form.hy2_obfs_type" style="width: 100%" clearable>
              <el-option label="无" value="" />
              <el-option label="salamander" value="salamander" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="form.hy2_obfs_type" label="混淆密码">
            <el-input v-model="form.hy2_obfs_password" placeholder="混淆密码" />
          </el-form-item>
          <el-form-item label="跳过验证">
            <el-switch v-model="form.allow_insecure" />
          </el-form-item>
        </template>

        <!-- 其他 -->
        <el-divider content-position="left">其他</el-divider>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { getNodes, createNode, updateNode, deleteNode } from '../api/node'

const loading = ref(false)
const nodes = ref<any[]>([])
const dialogVisible = ref(false)
const isEdit = ref(false)
const editId = ref<number | null>(null)
const submitting = ref(false)
const formRef = ref<FormInstance>()

// ---- 协议 / 传输 / 内核的关系映射 ----
const protocols = [
  { label: 'VLESS', value: 'vless' },
  { label: 'VMess', value: 'vmess' },
  { label: 'Trojan', value: 'trojan' },
  { label: 'Shadowsocks', value: 'ss' },
  { label: 'Hysteria2', value: 'hysteria2' },
]

// 协议 → 支持的传输方式
const protocolTransports: Record<string, { label: string; value: string }[]> = {
  vless: [
    { label: 'TCP', value: 'tcp' },
    { label: 'WebSocket', value: 'ws' },
    { label: 'gRPC', value: 'grpc' },
    { label: 'HTTP/2', value: 'h2' },
    { label: 'Reality', value: 'reality' },
  ],
  vmess: [
    { label: 'TCP', value: 'tcp' },
    { label: 'WebSocket', value: 'ws' },
    { label: 'gRPC', value: 'grpc' },
    { label: 'HTTP/2', value: 'h2' },
  ],
  trojan: [
    { label: 'TCP', value: 'tcp' },
    { label: 'WebSocket', value: 'ws' },
    { label: 'gRPC', value: 'grpc' },
  ],
  ss: [],
  hysteria2: [],
}

// 协议 → 支持的内核
const protocolKernels: Record<string, string[]> = {
  vless: ['xray', 'singbox'],
  vmess: ['xray', 'singbox'],
  trojan: ['xray', 'singbox'],
  ss: ['xray', 'singbox'],
  hysteria2: ['singbox'],
}

// ---- 计算属性 ----
const availableTransports = computed(() => protocolTransports[form.protocol] || [])
const availableKernels = computed(() => protocolKernels[form.protocol] || ['xray'])
const showTransport = computed(() => availableTransports.value.length > 0)
const showSecurity = computed(() =>
  form.transport !== 'reality' && form.protocol !== 'ss' && form.protocol !== 'hysteria2' && showTransport.value
)

// ---- 表单 ----
const defaultForm = () => ({
  name: '',
  host: '',
  port: 443,
  protocol: 'vless',
  transport: 'tcp',
  kernel_type: 'xray',
  sort_order: 0,
  // 安全
  security: 'none' as string,
  sni: '',
  allow_insecure: false,
  // Reality
  reality_dest: '',
  reality_private_key: '',
  reality_public_key: '',
  reality_short_id: '',
  fingerprint: 'chrome',
  flow: '',
  // WebSocket
  ws_path: '/',
  ws_host: '',
  // gRPC
  grpc_service_name: '',
  // HTTP/2
  h2_path: '/',
  h2_host: '',
  // Shadowsocks
  ss_method: 'aes-256-gcm',
  // Hysteria2
  hy2_obfs_type: '',
  hy2_obfs_password: '',
})

const form = reactive(defaultForm())

const rules: FormRules = {
  name: [{ required: true, message: '请输入节点名称', trigger: 'blur' }],
  host: [{ required: true, message: '请输入主机地址', trigger: 'blur' }],
  port: [{ required: true, message: '请输入端口', trigger: 'blur' }],
  protocol: [{ required: true, message: '请选择协议', trigger: 'change' }],
}

// ---- 协议切换时重置关联字段 ----
function onProtocolChange() {
  const transports = protocolTransports[form.protocol] || []
  if (transports.length > 0) {
    form.transport = transports[0].value
  } else {
    form.transport = ''
  }
  const kernels = protocolKernels[form.protocol] || ['xray']
  if (!kernels.includes(form.kernel_type)) {
    form.kernel_type = kernels[0]
  }
  form.security = 'none'
  if (form.protocol === 'hysteria2') {
    form.kernel_type = 'singbox'
  }
}

function onTransportChange() {
  if (form.transport === 'reality') {
    form.security = 'none' // reality 自带安全，不需要额外 TLS
  }
}

// ---- 表单数据 ↔ settings JSON 转换 ----
function formToSettings(): string {
  const s: Record<string, any> = {}

  // 安全/TLS
  if (form.transport === 'reality') {
    s.security = 'reality'
    s.sni = form.sni
    s.dest = form.reality_dest
    s.private_key = form.reality_private_key
    s.public_key = form.reality_public_key
    s.short_id = form.reality_short_id
    s.fingerprint = form.fingerprint
    s.flow = form.flow
    s.server_names = form.sni ? [form.sni] : []
    s.short_ids = form.reality_short_id ? [form.reality_short_id] : []
  } else if (form.security === 'tls') {
    s.security = 'tls'
    s.sni = form.sni
    s.tls = true
    if (form.allow_insecure) s.allow_insecure = true
  }

  // 传输
  if (form.transport === 'ws') {
    s.path = form.ws_path
    if (form.ws_host) s.host = form.ws_host
  } else if (form.transport === 'grpc') {
    s.service_name = form.grpc_service_name
  } else if (form.transport === 'h2') {
    s.path = form.h2_path
    if (form.h2_host) s.host = form.h2_host
  }

  // Shadowsocks
  if (form.protocol === 'ss') {
    s.method = form.ss_method
  }

  // Hysteria2
  if (form.protocol === 'hysteria2') {
    if (form.sni) s.sni = form.sni
    if (form.hy2_obfs_type) {
      s.obfs = form.hy2_obfs_type
      s.obfs_password = form.hy2_obfs_password
    }
    if (form.allow_insecure) s.skip_cert_verify = true
  }

  return JSON.stringify(s)
}

function settingsToForm(settingsStr: string) {
  let s: Record<string, any> = {}
  try {
    s = JSON.parse(settingsStr || '{}')
  } catch { return }

  form.sni = s.sni || s.serverName || ''
  form.allow_insecure = s.allow_insecure || s.skip_cert_verify || false

  // 安全类型
  if (s.security === 'reality' || form.transport === 'reality') {
    form.reality_dest = s.dest || ''
    form.reality_private_key = s.private_key || s.privateKey || ''
    form.reality_public_key = s.public_key || s.publicKey || ''
    form.reality_short_id = s.short_id || (Array.isArray(s.short_ids) ? s.short_ids[0] : '') || ''
    form.fingerprint = s.fingerprint || s.fp || 'chrome'
    form.flow = s.flow || ''
  } else if (s.tls || s.security === 'tls') {
    form.security = 'tls'
  } else {
    form.security = 'none'
  }

  // 传输
  form.ws_path = s.path || '/'
  form.ws_host = s.host || ''
  form.h2_path = s.path || '/'
  form.h2_host = s.host || ''
  form.grpc_service_name = s.service_name || s.serviceName || ''

  // SS
  form.ss_method = s.method || 'aes-256-gcm'

  // Hy2
  form.hy2_obfs_type = s.obfs || ''
  form.hy2_obfs_password = s.obfs_password || ''
}

// ---- 表格辅助 ----
function parsedSettings(row: any): Record<string, any> {
  try {
    return JSON.parse(row.settings || '{}')
  } catch { return {} }
}

function protocolColor(protocol: string): string {
  const map: Record<string, string> = { vless: '', vmess: 'success', trojan: 'warning', ss: 'danger', hysteria2: 'info' }
  return map[protocol] || ''
}

function securityColor(security: string): string {
  if (security === 'tls') return 'success'
  if (security === 'reality') return 'warning'
  return 'info'
}

// ---- 数据操作 ----
const fetchNodes = async () => {
  loading.value = true
  try {
    const { data } = await getNodes()
    nodes.value = (data.nodes || data || []).map((n: any) => ({ ...n, _switching: false }))
  } catch {
    ElMessage.error('获取节点列表失败')
  } finally {
    loading.value = false
  }
}

const openDialog = (row?: any) => {
  Object.assign(form, defaultForm())
  if (row) {
    isEdit.value = true
    editId.value = row.id
    form.name = row.name
    form.host = row.host
    form.port = row.port
    form.protocol = row.protocol
    form.transport = row.transport || 'tcp'
    form.kernel_type = row.kernel_type || 'xray'
    form.sort_order = row.sort_order || 0
    settingsToForm(row.settings)
  } else {
    isEdit.value = false
    editId.value = null
  }
  dialogVisible.value = true
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  submitting.value = true
  try {
    const payload: any = {
      name: form.name,
      host: form.host,
      port: form.port,
      protocol: form.protocol,
      transport: form.transport || 'tcp',
      kernel_type: form.kernel_type,
      sort_order: form.sort_order,
      settings: formToSettings(),
    }
    if (isEdit.value && editId.value) {
      await updateNode(editId.value, payload)
      ElMessage.success('节点更新成功')
    } else {
      await createNode(payload)
      ElMessage.success('节点创建成功')
    }
    dialogVisible.value = false
    await fetchNodes()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  } finally {
    submitting.value = false
  }
}

const handleToggle = async (row: any, val: boolean) => {
  row._switching = true
  try {
    await updateNode(row.id, { enable: val })
    ElMessage.success(val ? '已启用' : '已禁用')
  } catch (e: any) {
    row.enable = !val
    ElMessage.error(e.response?.data?.error || '操作失败')
  } finally {
    row._switching = false
  }
}

const handleDelete = async (id: number) => {
  try {
    await deleteNode(id)
    ElMessage.success('删除成功')
    await fetchNodes()
  } catch (e: any) {
    ElMessage.error(e.response?.data?.error || '删除失败')
  }
}

onMounted(fetchNodes)
</script>
