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
        <el-table-column prop="name" label="节点名称" min-width="130" />
        <el-table-column label="地址" min-width="180">
          <template #default="{ row }">{{ row.host }}:{{ row.port }}</template>
        </el-table-column>
        <el-table-column label="协议" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="protocolColor(row.protocol)">{{ row.protocol.toUpperCase() }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="传输" width="90">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ row.transport || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="安全" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="securityColor(getSecurity(row))">
              {{ getSecurity(row) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="内核" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.kernel_type === 'xray' ? '' : 'success'">{{ row.kernel_type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="70" align="center">
          <template #default="{ row }">
            <el-switch v-model="row.enable" :loading="row._switching" @change="(val: boolean) => handleToggle(row, val)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
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
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑节点' : '新增节点'" width="660px" destroy-on-close top="5vh">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px" class="max-h-[70vh] overflow-y-auto pr-2">

        <!-- ===== 基础 ===== -->
        <el-divider content-position="left">基础信息</el-divider>
        <el-form-item label="节点名称" prop="name">
          <el-input v-model="form.name" placeholder="如: Tokyo-01" />
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="8">
            <el-form-item label="监听 IP">
              <el-input v-model="form.listen" placeholder="0.0.0.0" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="端口" prop="port">
              <el-input v-model.number="form.port" type="number" placeholder="443" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="排序">
              <el-input v-model.number="form.sort_order" type="number" placeholder="0" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="主机地址" prop="host">
          <el-input v-model="form.host" placeholder="客户端连接的 IP 或域名 (可与监听 IP 不同)" />
        </el-form-item>

        <!-- ===== 协议 ===== -->
        <el-divider content-position="left">协议</el-divider>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="协议" prop="protocol">
              <el-select v-model="form.protocol" style="width:100%" @change="onProtocolChange">
                <el-option v-for="p in protocols" :key="p.value" :label="p.label" :value="p.value" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="内核">
              <el-select v-model="form.kernel_type" style="width:100%">
                <el-option v-for="k in availableKernels" :key="k" :label="k" :value="k" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Shadowsocks 加密 -->
        <el-form-item v-if="form.protocol === 'ss'" label="加密方式">
          <el-select v-model="form.ss_method" style="width:100%">
            <el-option v-for="m in ssMethods" :key="m" :label="m" :value="m" />
          </el-select>
        </el-form-item>

        <!-- ===== 传输 ===== -->
        <template v-if="hasTransport">
          <el-divider content-position="left">传输</el-divider>
          <el-form-item label="传输方式">
            <el-select v-model="form.transport" style="width:100%" @change="onTransportChange">
              <el-option v-for="t in availableTransports" :key="t.value" :label="t.label" :value="t.value" />
            </el-select>
          </el-form-item>

          <!-- TCP 无额外配置 -->

          <!-- WebSocket -->
          <template v-if="form.transport === 'ws'">
            <el-form-item label="路径">
              <el-input v-model="form.ws_path" placeholder="/" />
            </el-form-item>
            <el-form-item label="Host">
              <el-input v-model="form.ws_host" placeholder="可选，伪装 Host 头" />
            </el-form-item>
          </template>

          <!-- gRPC -->
          <template v-if="form.transport === 'grpc'">
            <el-form-item label="Service Name">
              <el-input v-model="form.grpc_service_name" placeholder="如: grpc-service" />
            </el-form-item>
            <el-form-item label="Multi Mode">
              <el-switch v-model="form.grpc_multi_mode" />
            </el-form-item>
          </template>

          <!-- HTTP/2 -->
          <template v-if="form.transport === 'h2'">
            <el-form-item label="路径">
              <el-input v-model="form.h2_path" placeholder="/" />
            </el-form-item>
            <el-form-item label="Host">
              <el-input v-model="form.h2_host" placeholder="可选" />
            </el-form-item>
          </template>

          <!-- HTTPUpgrade -->
          <template v-if="form.transport === 'httpupgrade'">
            <el-form-item label="路径">
              <el-input v-model="form.ws_path" placeholder="/" />
            </el-form-item>
            <el-form-item label="Host">
              <el-input v-model="form.ws_host" placeholder="可选" />
            </el-form-item>
          </template>
        </template>

        <!-- ===== 安全 ===== -->
        <template v-if="hasSecurity">
          <el-divider content-position="left">安全</el-divider>
          <el-form-item label="安全方式">
            <el-select v-model="form.security" style="width:100%">
              <el-option v-for="s in availableSecurities" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
          </el-form-item>

          <!-- ---- TLS ---- -->
          <template v-if="form.security === 'tls'">
            <el-form-item label="SNI">
              <el-input v-model="form.sni" placeholder="服务器域名" />
            </el-form-item>
            <el-form-item label="uTLS 指纹">
              <el-select v-model="form.fingerprint" style="width:100%">
                <el-option v-for="f in fingerprints" :key="f" :label="f" :value="f" />
              </el-select>
            </el-form-item>
            <el-form-item label="ALPN">
              <el-select v-model="form.alpn" multiple style="width:100%" placeholder="留空自动">
                <el-option label="h2" value="h2" />
                <el-option label="http/1.1" value="http/1.1" />
              </el-select>
            </el-form-item>
            <el-form-item label="证书文件路径">
              <div class="flex gap-2 w-full">
                <el-input v-model="form.cert_path" placeholder="如: /opt/proxy-panel/certs/domain.crt" />
                <el-dropdown trigger="click" @command="fillCertPath">
                  <el-button>引用<el-icon class="ml-1"><ArrowDown /></el-icon></el-button>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item v-for="c in systemCerts" :key="c.label" :command="c.cert">{{ c.label }}</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </div>
            </el-form-item>
            <el-form-item label="私钥文件路径">
              <div class="flex gap-2 w-full">
                <el-input v-model="form.key_path" placeholder="如: /opt/proxy-panel/certs/domain.key" />
                <el-dropdown trigger="click" @command="fillKeyPath">
                  <el-button>引用<el-icon class="ml-1"><ArrowDown /></el-icon></el-button>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item v-for="c in systemCerts" :key="c.label" :command="c.key">{{ c.label }}</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </div>
            </el-form-item>
            <el-form-item label="跳过证书验证">
              <el-switch v-model="form.allow_insecure" />
              <span class="ml-2 text-xs text-gray-400">客户端侧，仅用于自签证书</span>
            </el-form-item>
          </template>

          <!-- ---- Reality ---- -->
          <template v-if="form.security === 'reality'">
            <el-form-item label="目标地址 (Dest)">
              <el-input v-model="form.reality_dest" placeholder="如: www.google.com:443" />
            </el-form-item>
            <el-form-item label="SNI (Server Names)">
              <el-input v-model="form.sni" placeholder="如: www.google.com" />
            </el-form-item>
            <el-form-item label="Private Key">
              <el-input v-model="form.reality_private_key" placeholder="xray x25519 生成的私钥" />
            </el-form-item>
            <el-form-item label="Public Key">
              <el-input v-model="form.reality_public_key" placeholder="对应的公钥 (客户端使用)" />
            </el-form-item>
            <el-form-item label="Short IDs">
              <el-input v-model="form.reality_short_id" placeholder="如: abcd1234 (多个逗号分隔)" />
            </el-form-item>
            <el-form-item label="uTLS 指纹">
              <el-select v-model="form.fingerprint" style="width:100%">
                <el-option v-for="f in fingerprints" :key="f" :label="f" :value="f" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="form.protocol === 'vless'" label="Flow">
              <el-select v-model="form.flow" style="width:100%" clearable>
                <el-option label="无" value="" />
                <el-option label="xtls-rprx-vision" value="xtls-rprx-vision" />
              </el-select>
            </el-form-item>
          </template>
        </template>

        <!-- ===== Hysteria2 专属 ===== -->
        <template v-if="form.protocol === 'hysteria2'">
          <el-divider content-position="left">Hysteria2 配置</el-divider>
          <el-form-item label="SNI">
            <el-input v-model="form.sni" placeholder="可选" />
          </el-form-item>
          <el-form-item label="证书文件路径">
            <div class="flex gap-2 w-full">
              <el-input v-model="form.cert_path" placeholder="如: /opt/proxy-panel/certs/hy2.crt" />
              <el-dropdown trigger="click" @command="fillCertPath">
                <el-button>引用<el-icon class="ml-1"><ArrowDown /></el-icon></el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-for="c in systemCerts" :key="c.label" :command="c.cert">{{ c.label }}</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </el-form-item>
          <el-form-item label="私钥文件路径">
            <div class="flex gap-2 w-full">
              <el-input v-model="form.key_path" placeholder="如: /opt/proxy-panel/certs/hy2.key" />
              <el-dropdown trigger="click" @command="fillKeyPath">
                <el-button>引用<el-icon class="ml-1"><ArrowDown /></el-icon></el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-for="c in systemCerts" :key="c.label" :command="c.key">{{ c.label }}</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </el-form-item>
          <el-form-item label="混淆类型">
            <el-select v-model="form.hy2_obfs_type" style="width:100%" clearable>
              <el-option label="无" value="" />
              <el-option label="salamander" value="salamander" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="form.hy2_obfs_type" label="混淆密码">
            <el-input v-model="form.hy2_obfs_password" placeholder="混淆密码" />
          </el-form-item>
          <el-form-item label="跳过证书验证">
            <el-switch v-model="form.allow_insecure" />
            <span class="ml-2 text-xs text-gray-400">客户端侧</span>
          </el-form-item>
        </template>
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

// ---- 状态 ----
const loading = ref(false)
const nodes = ref<any[]>([])
const dialogVisible = ref(false)
const isEdit = ref(false)
const editId = ref<number | null>(null)
const submitting = ref(false)
const formRef = ref<FormInstance>()

// ---- 常量：协议 / 传输 / 安全 / 内核 映射 ----
const protocols = [
  { label: 'VLESS', value: 'vless' },
  { label: 'VMess', value: 'vmess' },
  { label: 'Trojan', value: 'trojan' },
  { label: 'Shadowsocks', value: 'ss' },
  { label: 'Hysteria2', value: 'hysteria2' },
]

const protocolTransportMap: Record<string, { label: string; value: string }[]> = {
  vless:   [{ label: 'TCP', value: 'tcp' }, { label: 'WebSocket', value: 'ws' }, { label: 'gRPC', value: 'grpc' }, { label: 'HTTP/2', value: 'h2' }, { label: 'HTTPUpgrade', value: 'httpupgrade' }],
  vmess:   [{ label: 'TCP', value: 'tcp' }, { label: 'WebSocket', value: 'ws' }, { label: 'gRPC', value: 'grpc' }, { label: 'HTTP/2', value: 'h2' }, { label: 'HTTPUpgrade', value: 'httpupgrade' }],
  trojan:  [{ label: 'TCP', value: 'tcp' }, { label: 'WebSocket', value: 'ws' }, { label: 'gRPC', value: 'grpc' }],
  ss:      [],
  hysteria2: [],
}

const protocolSecurityMap: Record<string, { label: string; value: string }[]> = {
  vless:   [{ label: '无', value: 'none' }, { label: 'TLS', value: 'tls' }, { label: 'Reality', value: 'reality' }],
  vmess:   [{ label: '无', value: 'none' }, { label: 'TLS', value: 'tls' }],
  trojan:  [{ label: 'TLS', value: 'tls' }, { label: 'Reality', value: 'reality' }],
  ss:      [],
  hysteria2: [],
}

const protocolKernelMap: Record<string, string[]> = {
  vless: ['xray', 'singbox'],
  vmess: ['xray', 'singbox'],
  trojan: ['xray', 'singbox'],
  ss: ['xray', 'singbox'],
  hysteria2: ['singbox'],
}

const ssMethods = [
  'aes-256-gcm', 'aes-128-gcm', 'chacha20-ietf-poly1305',
  '2022-blake3-aes-256-gcm', '2022-blake3-aes-128-gcm', '2022-blake3-chacha20-poly1305',
]

const fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized']

// 系统预设证书路径
const systemCerts = [
  { label: 'acme.sh 签发证书', cert: '/opt/proxy-panel/certs/${domain}.crt', key: '/opt/proxy-panel/certs/${domain}.key' },
  { label: 'Cloudflare Origin', cert: '/opt/proxy-panel/certs/origin.crt', key: '/opt/proxy-panel/certs/origin.key' },
  { label: 'Hysteria2 自签证书', cert: '/opt/proxy-panel/certs/hy2.crt', key: '/opt/proxy-panel/certs/hy2.key' },
]

function fillCertPath(path: string) {
  form.cert_path = path.replace('${domain}', form.sni || 'example.com')
}
function fillKeyPath(path: string) {
  form.key_path = path.replace('${domain}', form.sni || 'example.com')
}

// ---- 计算属性 ----
const availableTransports = computed(() => protocolTransportMap[form.protocol] || [])
const availableSecurities = computed(() => protocolSecurityMap[form.protocol] || [])
const availableKernels = computed(() => protocolKernelMap[form.protocol] || ['xray'])
const hasTransport = computed(() => availableTransports.value.length > 0)
const hasSecurity = computed(() => availableSecurities.value.length > 0)

// ---- 表单 ----
const defaultForm = () => ({
  name: '',
  listen: '',
  host: '',
  port: 443,
  protocol: 'vless',
  transport: 'tcp',
  kernel_type: 'xray',
  sort_order: 0,
  // 安全
  security: 'none',
  sni: '',
  fingerprint: 'chrome',
  alpn: [] as string[],
  allow_insecure: false,
  cert_path: '',
  key_path: '',
  // Reality
  reality_dest: '',
  reality_private_key: '',
  reality_public_key: '',
  reality_short_id: '',
  flow: '',
  // WebSocket / HTTPUpgrade
  ws_path: '/',
  ws_host: '',
  // gRPC
  grpc_service_name: '',
  grpc_multi_mode: false,
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

// ---- 联动 ----
function onProtocolChange() {
  // 重置传输
  const trs = protocolTransportMap[form.protocol] || []
  form.transport = trs.length > 0 ? trs[0].value : ''
  // 重置安全
  const secs = protocolSecurityMap[form.protocol] || []
  form.security = secs.length > 0 ? secs[0].value : 'none'
  // 修正内核
  const kernels = protocolKernelMap[form.protocol] || ['xray']
  if (!kernels.includes(form.kernel_type)) form.kernel_type = kernels[0]
  if (form.protocol === 'hysteria2') form.kernel_type = 'singbox'
  if (form.protocol === 'trojan') form.security = 'tls' // trojan 默认 TLS
}

function onTransportChange() {
  // reality 作为 transport 时不需要，现在 reality 是 security
}

// ---- settings JSON ↔ 表单 转换 ----
function formToSettings(): string {
  const s: Record<string, any> = {}

  // 安全
  s.security = form.security
  if (form.security === 'tls') {
    if (form.sni) s.sni = form.sni
    s.tls = true
    if (form.fingerprint) s.fingerprint = form.fingerprint
    if (form.alpn.length > 0) s.alpn = form.alpn
    if (form.cert_path) s.cert_path = form.cert_path
    if (form.key_path) s.key_path = form.key_path
    if (form.allow_insecure) s.allow_insecure = true
  } else if (form.security === 'reality') {
    if (form.sni) { s.sni = form.sni; s.server_names = [form.sni] }
    if (form.reality_dest) s.dest = form.reality_dest
    if (form.reality_private_key) s.private_key = form.reality_private_key
    if (form.reality_public_key) s.public_key = form.reality_public_key
    if (form.reality_short_id) { s.short_id = form.reality_short_id; s.short_ids = form.reality_short_id.split(',').map((x: string) => x.trim()) }
    if (form.fingerprint) s.fingerprint = form.fingerprint
    if (form.flow) s.flow = form.flow
  }

  // 传输
  if (form.transport === 'ws' || form.transport === 'httpupgrade') {
    if (form.ws_path) s.path = form.ws_path
    if (form.ws_host) s.host = form.ws_host
  } else if (form.transport === 'grpc') {
    if (form.grpc_service_name) s.service_name = form.grpc_service_name
    if (form.grpc_multi_mode) s.multi_mode = true
  } else if (form.transport === 'h2') {
    if (form.h2_path) s.path = form.h2_path
    if (form.h2_host) s.host = form.h2_host
  }

  // Shadowsocks
  if (form.protocol === 'ss') s.method = form.ss_method

  // Hysteria2
  if (form.protocol === 'hysteria2') {
    if (form.sni) s.sni = form.sni
    if (form.cert_path) s.cert_path = form.cert_path
    if (form.key_path) s.key_path = form.key_path
    if (form.hy2_obfs_type) { s.obfs = form.hy2_obfs_type; s.obfs_password = form.hy2_obfs_password }
    if (form.allow_insecure) s.skip_cert_verify = true
  }

  // 监听地址
  if (form.listen) s.listen = form.listen

  return JSON.stringify(s)
}

function settingsToForm(settingsStr: string) {
  let s: Record<string, any> = {}
  try { s = JSON.parse(settingsStr || '{}') } catch { return }

  form.sni = s.sni || s.serverName || ''
  form.allow_insecure = s.allow_insecure || s.skip_cert_verify || false
  form.fingerprint = s.fingerprint || s.fp || 'chrome'
  form.alpn = s.alpn || []
  form.cert_path = s.cert_path || s.certPath || ''
  form.key_path = s.key_path || s.keyPath || ''
  form.listen = s.listen || ''

  // 安全
  if (s.security === 'reality') {
    form.security = 'reality'
    form.reality_dest = s.dest || ''
    form.reality_private_key = s.private_key || s.privateKey || ''
    form.reality_public_key = s.public_key || s.publicKey || ''
    form.reality_short_id = s.short_id || (Array.isArray(s.short_ids) ? s.short_ids.join(',') : '') || ''
    form.flow = s.flow || ''
  } else if (s.tls || s.security === 'tls') {
    form.security = 'tls'
  } else {
    form.security = s.security || 'none'
  }

  // 传输
  form.ws_path = s.path || '/'
  form.ws_host = s.host || ''
  form.h2_path = s.path || '/'
  form.h2_host = s.host || ''
  form.grpc_service_name = s.service_name || s.serviceName || ''
  form.grpc_multi_mode = s.multi_mode || false
  form.ss_method = s.method || 'aes-256-gcm'
  form.hy2_obfs_type = s.obfs || ''
  form.hy2_obfs_password = s.obfs_password || ''
}

// ---- 表格辅助 ----
function getSecurity(row: any): string {
  try { const s = JSON.parse(row.settings || '{}'); return s.security || (s.tls ? 'tls' : 'none') } catch { return 'none' }
}
function protocolColor(p: string) { return ({ vless: '', vmess: 'success', trojan: 'warning', ss: 'danger', hysteria2: 'info' } as any)[p] || '' }
function securityColor(s: string) { return s === 'tls' ? 'success' : s === 'reality' ? 'warning' : 'info' }

// ---- CRUD ----
const fetchNodes = async () => {
  loading.value = true
  try {
    const { data } = await getNodes()
    nodes.value = (data.nodes || data || []).map((n: any) => ({ ...n, _switching: false }))
  } catch { ElMessage.error('获取节点列表失败') }
  finally { loading.value = false }
}

const openDialog = (row?: any) => {
  Object.assign(form, defaultForm())
  if (row) {
    isEdit.value = true
    editId.value = row.id
    form.name = row.name; form.host = row.host; form.port = row.port
    form.protocol = row.protocol; form.transport = row.transport || 'tcp'
    form.kernel_type = row.kernel_type || 'xray'; form.sort_order = row.sort_order || 0
    settingsToForm(row.settings)
  } else {
    isEdit.value = false; editId.value = null
  }
  dialogVisible.value = true
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  submitting.value = true
  try {
    // 对 reality，transport 存 "tcp"，security 信息存在 settings 里
    let transport = form.transport
    if (form.security === 'reality' && !transport) transport = 'tcp'

    const payload: any = {
      name: form.name, host: form.host, port: form.port,
      protocol: form.protocol, transport: transport || 'tcp',
      kernel_type: form.kernel_type, sort_order: form.sort_order,
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
  } catch (e: any) { ElMessage.error(e.response?.data?.error || '操作失败') }
  finally { submitting.value = false }
}

const handleToggle = async (row: any, val: boolean) => {
  row._switching = true
  try {
    await updateNode(row.id, { enable: val })
    ElMessage.success(val ? '已启用' : '已禁用')
  } catch (e: any) { row.enable = !val; ElMessage.error(e.response?.data?.error || '操作失败') }
  finally { row._switching = false }
}

const handleDelete = async (id: number) => {
  try { await deleteNode(id); ElMessage.success('删除成功'); await fetchNodes() }
  catch (e: any) { ElMessage.error(e.response?.data?.error || '删除失败') }
}

onMounted(fetchNodes)
</script>
