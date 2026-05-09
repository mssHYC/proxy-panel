<template>
  <div class="nodes" :class="{ 'is-loading-overlay': loading }">
    <div class="toolbar">
      <p class="toolbar__hint">共 <span class="num">{{ nodes.length }}</span> 个节点。</p>
      <div class="toolbar__actions">
        <Tooltip content="跳过 5s 防抖窗口，立即把当前节点/用户配置下发到内核（Xray 热加载，Sing-box 重启）">
          <Button :loading="syncing" @click="handleManualSync">
            <RefreshCw :size="14" :stroke-width="1.6" /> 立即应用变更
          </Button>
        </Tooltip>
        <Button variant="primary" @click="openDialog()">
          <Plus :size="14" :stroke-width="2" /> 新增节点
        </Button>
      </div>
    </div>

    <table v-if="nodes.length || loading" class="dt">
      <thead>
        <tr>
          <th>节点</th>
          <th>地址</th>
          <th>协议</th>
          <th>传输</th>
          <th>安全</th>
          <th>内核</th>
          <th>启用</th>
          <th>健康</th>
          <th class="is-numeric">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in nodes" :key="row.id">
          <td><span class="cell-name">{{ row.name }}</span></td>
          <td><span class="mono cell-addr">{{ row.host }}<span class="cell-addr__sep">:</span>{{ row.port }}</span></td>
          <td><span class="proto-tag" :data-proto="row.protocol">{{ row.protocol.toUpperCase() }}</span></td>
          <td><span class="mono cell-meta">{{ row.transport || '—' }}</span></td>
          <td><span class="mono cell-meta">{{ getSecurity(row) }}</span></td>
          <td><span class="mono cell-meta">{{ row.kernel_type }}</span></td>
          <td>
            <Switch :model-value="row.enable" :disabled="row._switching" @update:model-value="(v) => handleToggle(row, v)" />
          </td>
          <td>
            <Tooltip v-if="row.last_check_at" :content="healthTooltip(row)">
              <StatusDot :state="row.last_check_ok ? 'ok' : 'crit'">
                {{ row.last_check_ok ? '在线' : '离线' }}
              </StatusDot>
            </Tooltip>
            <StatusDot v-else state="off">待检测</StatusDot>
          </td>
          <td class="is-numeric">
            <div class="row-actions">
              <button class="row-actions__btn" @click="openDialog(row)" title="编辑">
                <Pencil :size="14" :stroke-width="1.6" />
              </button>
              <button class="row-actions__btn row-actions__btn--danger" @click="handleDelete(row)" title="删除">
                <Trash2 :size="14" :stroke-width="1.6" />
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>

    <div v-if="!loading && !nodes.length" class="empty-state">
      <p class="empty-state__title">还没有节点</p>
      <p class="empty-state__hint">节点是真正承载流量的入口。配置好至少一个节点后，用户的订阅链接才有内容可下发。</p>
      <Button variant="primary" @click="openDialog()">
        <Plus :size="14" :stroke-width="2" /> 添加第一个节点
      </Button>
    </div>

    <Modal v-model:open="dialogVisible" :title="isEdit ? '编辑节点' : '新增节点'" :width="760">
      <!-- ============ 基础 ============ -->
      <section class="nf-section">
        <h4 class="nf-section__title">基础</h4>
        <div class="nf-grid">
          <Field class="nf-col-2" label="节点名称" :error="errors.name">
            <Input v-model="form.name" placeholder="如: Tokyo-01" />
          </Field>
          <Field class="nf-col-2" label="主机地址" hint="客户端连接的 IP 或域名（可与监听 IP 不同）" :error="errors.host">
            <Input v-model="form.host" placeholder="example.com 或 1.2.3.4" />
          </Field>
          <Field label="监听 IP" hint="留空使用 0.0.0.0">
            <Input v-model="form.listen" placeholder="0.0.0.0" />
          </Field>
          <Field label="端口" :error="errors.port">
            <NumberInput v-model="form.port" :min="1" :max="65535" />
          </Field>
          <Field label="排序" hint="数值越小越靠前">
            <NumberInput v-model="form.sort_order" :min="0" />
          </Field>
        </div>
      </section>

      <!-- ============ 协议 ============ -->
      <section class="nf-section">
        <h4 class="nf-section__title">协议</h4>
        <div class="nf-grid">
          <Field label="协议">
            <Select
              :model-value="form.protocol"
              :options="protocols.map(p => ({ label: p.label, value: p.value }))"
              @update:model-value="(v) => { form.protocol = String(v); onProtocolChange() }"
            />
          </Field>
          <Field label="内核">
            <Select
              :model-value="form.kernel_type"
              :options="availableKernels.map(k => ({ label: k, value: k }))"
              @update:model-value="(v) => (form.kernel_type = String(v))"
            />
          </Field>
          <Field v-if="form.protocol === 'ss'" class="nf-col-2" label="加密方式">
            <Select
              :model-value="form.ss_method"
              :options="ssMethods.map(m => ({ label: m, value: m }))"
              @update:model-value="(v) => (form.ss_method = String(v))"
            />
          </Field>
        </div>
      </section>

      <!-- ============ 传输 ============ -->
      <section v-if="hasTransport" class="nf-section">
        <h4 class="nf-section__title">传输</h4>
        <div class="nf-grid">
          <Field class="nf-col-2" label="传输方式">
            <Select
              :model-value="form.transport"
              :options="availableTransports.map(t => ({ label: t.label, value: t.value }))"
              @update:model-value="(v) => (form.transport = String(v))"
            />
          </Field>
          <template v-if="form.transport === 'ws' || form.transport === 'httpupgrade'">
            <Field label="路径"><Input v-model="form.ws_path" placeholder="/" /></Field>
            <Field label="Host" hint="可选，伪装 Host 头"><Input v-model="form.ws_host" /></Field>
          </template>
          <template v-if="form.transport === 'grpc'">
            <Field label="Service Name"><Input v-model="form.grpc_service_name" placeholder="grpc-service" /></Field>
            <div class="nf-toggle">
              <div class="nf-toggle__head">
                <span class="nf-toggle__label">Multi Mode</span>
                <small class="nf-toggle__hint">启用多路复用</small>
              </div>
              <Switch v-model="form.grpc_multi_mode" />
            </div>
          </template>
          <template v-if="form.transport === 'h2'">
            <Field label="路径"><Input v-model="form.h2_path" placeholder="/" /></Field>
            <Field label="Host" hint="可选"><Input v-model="form.h2_host" /></Field>
          </template>
        </div>
      </section>

      <!-- ============ 安全 ============ -->
      <section v-if="hasSecurity" class="nf-section">
        <h4 class="nf-section__title">安全</h4>
        <div class="nf-grid">
          <Field class="nf-col-2" label="安全方式">
            <Select
              :model-value="form.security"
              :options="availableSecurities.map(s => ({ label: s.label, value: s.value }))"
              @update:model-value="(v) => (form.security = String(v))"
            />
          </Field>

          <template v-if="form.security === 'tls'">
            <Field label="SNI" hint="服务器域名"><Input v-model="form.sni" /></Field>
            <Field label="uTLS 指纹">
              <Select
                :model-value="form.fingerprint"
                :options="fingerprints.map(f => ({ label: f, value: f }))"
                @update:model-value="(v) => (form.fingerprint = String(v))"
              />
            </Field>
            <Field class="nf-col-2" label="ALPN" hint="留空自动协商">
              <MultiSelect
                v-model="form.alpn"
                :options="[{label:'h2',value:'h2'},{label:'http/1.1',value:'http/1.1'}]"
                placeholder="选择 ALPN"
              />
            </Field>
            <Field class="nf-col-2" label="证书文件">
              <div class="nf-cert">
                <Input v-model="form.cert_path" placeholder="/opt/proxy-panel/certs/domain.crt" />
                <Button @click="fillSystemCert">填充</Button>
              </div>
            </Field>
            <Field class="nf-col-2" label="私钥文件">
              <div class="nf-cert">
                <Input v-model="form.key_path" placeholder="/opt/proxy-panel/certs/domain.key" />
                <Button @click="fillSystemCert">填充</Button>
              </div>
            </Field>
            <div class="nf-toggle nf-col-2">
              <div class="nf-toggle__head">
                <span class="nf-toggle__label">跳过证书验证</span>
                <small class="nf-toggle__hint">仅用于自签证书</small>
              </div>
              <Switch v-model="form.allow_insecure" />
            </div>
          </template>

          <template v-if="form.security === 'reality'">
            <Field label="目标地址 Dest"><Input v-model="form.reality_dest" placeholder="www.google.com:443" /></Field>
            <Field label="SNI" hint="Server Names"><Input v-model="form.sni" placeholder="www.google.com" /></Field>
            <Field class="nf-col-2 nf-keypair" label="x25519 密钥对" hint="自动生成 Private Key / Public Key / Short IDs">
              <Button variant="primary" :loading="generatingKeypair" @click="handleGenerateKeypair">自动生成</Button>
            </Field>
            <Field class="nf-col-2" label="Private Key">
              <Input v-model="form.reality_private_key" />
            </Field>
            <Field class="nf-col-2" label="Public Key" hint="客户端使用">
              <Input v-model="form.reality_public_key" />
            </Field>
            <Field class="nf-col-2" label="Short IDs" hint="多个用逗号分隔">
              <Input v-model="form.reality_short_id" placeholder="abcd1234,ef56" />
            </Field>
            <Field label="uTLS 指纹">
              <Select
                :model-value="form.fingerprint"
                :options="fingerprints.map(f => ({ label: f, value: f }))"
                @update:model-value="(v) => (form.fingerprint = String(v))"
              />
            </Field>
            <Field v-if="form.protocol === 'vless'" label="Flow">
              <Select
                :model-value="form.flow || ''"
                :options="[{label:'无',value:''},{label:'xtls-rprx-vision',value:'xtls-rprx-vision'}]"
                @update:model-value="(v) => (form.flow = String(v))"
              />
            </Field>
          </template>
        </div>
      </section>

      <!-- ============ Hysteria2 ============ -->
      <section v-if="form.protocol === 'hysteria2'" class="nf-section">
        <h4 class="nf-section__title">Hysteria2</h4>
        <div class="nf-grid">
          <Field class="nf-col-2" label="SNI" hint="可选"><Input v-model="form.sni" /></Field>
          <Field class="nf-col-2" label="证书文件">
            <div class="nf-cert">
              <Input v-model="form.cert_path" placeholder="/opt/proxy-panel/certs/domain.crt" />
              <Button @click="fillSystemCert">填充</Button>
            </div>
          </Field>
          <Field class="nf-col-2" label="私钥文件">
            <div class="nf-cert">
              <Input v-model="form.key_path" placeholder="/opt/proxy-panel/certs/domain.key" />
              <Button @click="fillSystemCert">填充</Button>
            </div>
          </Field>
          <Field label="混淆类型">
            <Select
              :model-value="form.hy2_obfs_type || ''"
              :options="[{label:'无',value:''},{label:'salamander',value:'salamander'}]"
              @update:model-value="(v) => (form.hy2_obfs_type = String(v))"
            />
          </Field>
          <Field v-if="form.hy2_obfs_type" label="混淆密码">
            <Input v-model="form.hy2_obfs_password" />
          </Field>
          <Field label="最大上行" hint="Mbps · 0 不限速">
            <NumberInput v-model="form.hy2_max_up_mbps" :min="0" :max="20" />
          </Field>
          <Field label="最大下行" hint="Mbps · 0 不限速">
            <NumberInput v-model="form.hy2_max_down_mbps" :min="0" :max="20" />
          </Field>
          <div class="nf-toggle nf-col-2">
            <div class="nf-toggle__head">
              <span class="nf-toggle__label">跳过证书验证</span>
              <small class="nf-toggle__hint">客户端侧</small>
            </div>
            <Switch v-model="form.allow_insecure" />
          </div>
          <div class="nf-toggle nf-col-2">
            <div class="nf-toggle__head">
              <span class="nf-toggle__label">忽略客户端带宽</span>
              <small class="nf-toggle__hint">服务端全权控制</small>
            </div>
            <Switch v-model="form.hy2_ignore_client_bandwidth" />
          </div>
        </div>
      </section>

      <template #footer>
        <Button @click="dialogVisible = false">取消</Button>
        <Button variant="primary" :loading="submitting" @click="handleSubmit">{{ isEdit ? '保存' : '创建' }}</Button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { Plus, Pencil, Trash2, RefreshCw } from 'lucide-vue-next'
import {
  Button, Input, NumberInput, Select, MultiSelect, Switch, Modal, Field, Tooltip, StatusDot,
  toast, confirm,
} from '../ui'
import { getNodes, createNode, updateNode, deleteNode, generateRealityKeypair } from '../api/node'
import { getSettings } from '../api/setting'
import request from '../api/request'

const loading = ref(false)
const nodes = ref<any[]>([])
const dialogVisible = ref(false)
const isEdit = ref(false)
const editId = ref<number | null>(null)
const submitting = ref(false)
const syncing = ref(false)
const errors = reactive<{ name?: string; host?: string; port?: string }>({})

async function handleManualSync() {
  syncing.value = true
  try {
    await request.post('/kernel/sync')
    toast.success('内核配置已立即下发')
  } catch (e: any) {
    toast.error(e.response?.data?.error || '同步失败')
  } finally {
    syncing.value = false
  }
}

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
  ss: [],
  hysteria2: [],
}

const protocolSecurityMap: Record<string, { label: string; value: string }[]> = {
  vless:   [{ label: '无', value: 'none' }, { label: 'TLS', value: 'tls' }, { label: 'Reality', value: 'reality' }],
  vmess:   [{ label: '无', value: 'none' }, { label: 'TLS', value: 'tls' }],
  trojan:  [{ label: 'TLS', value: 'tls' }, { label: 'Reality', value: 'reality' }],
  ss: [],
  hysteria2: [],
}

const protocolKernelMap: Record<string, string[]> = {
  vless: ['xray', 'singbox'], vmess: ['xray', 'singbox'], trojan: ['xray', 'singbox'],
  ss: ['xray', 'singbox'], hysteria2: ['singbox'],
}

const ssMethods = [
  'aes-256-gcm', 'aes-128-gcm', 'chacha20-ietf-poly1305',
  '2022-blake3-aes-256-gcm', '2022-blake3-aes-128-gcm', '2022-blake3-chacha20-poly1305',
]

const fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized']

const systemCertPath = ref('')
const systemKeyPath = ref('')

const generatingKeypair = ref(false)
async function handleGenerateKeypair() {
  generatingKeypair.value = true
  try {
    const { data } = await generateRealityKeypair()
    form.reality_private_key = data.private_key
    form.reality_public_key = data.public_key
    form.reality_short_id = (data.short_ids as string[]).join(',')
    toast.success('密钥对和 Short IDs 已生成')
  } catch { toast.error('生成失败') }
  finally { generatingKeypair.value = false }
}

async function loadSystemCertPaths() {
  try {
    const { data } = await getSettings()
    const map: Record<string, string> = typeof data === 'object' && !Array.isArray(data) ? data : {}
    systemCertPath.value = map.system_cert_path || ''
    systemKeyPath.value = map.system_key_path || ''
  } catch {/* ignore */}
}

function fillSystemCert() {
  if (!systemCertPath.value || !systemKeyPath.value) {
    toast.warn('系统未配置 TLS 证书，请在 config.yaml 或安装脚本中设置')
    return
  }
  form.cert_path = systemCertPath.value
  form.key_path = systemKeyPath.value
  toast.success('已填充系统证书路径')
}

const availableTransports = computed(() => protocolTransportMap[form.protocol] || [])
const availableSecurities = computed(() => protocolSecurityMap[form.protocol] || [])
const availableKernels = computed(() => protocolKernelMap[form.protocol] || ['xray'])
const hasTransport = computed(() => availableTransports.value.length > 0)
const hasSecurity = computed(() => availableSecurities.value.length > 0)

const defaultForm = () => ({
  name: '', listen: '', host: '', port: 443,
  protocol: 'vless', transport: 'tcp', kernel_type: 'xray', sort_order: 0,
  security: 'none', sni: '', fingerprint: 'chrome', alpn: [] as string[],
  allow_insecure: false, cert_path: '', key_path: '',
  reality_dest: '', reality_private_key: '', reality_public_key: '', reality_short_id: '', flow: '',
  ws_path: '/', ws_host: '',
  grpc_service_name: '', grpc_multi_mode: false,
  h2_path: '/', h2_host: '',
  ss_method: 'aes-256-gcm',
  hy2_obfs_type: '', hy2_obfs_password: '',
  hy2_max_up_mbps: 10, hy2_max_down_mbps: 10, hy2_ignore_client_bandwidth: false,
})
const form = reactive(defaultForm())

function onProtocolChange() {
  const trs = protocolTransportMap[form.protocol] || []
  form.transport = trs.length > 0 ? trs[0].value : ''
  const secs = protocolSecurityMap[form.protocol] || []
  form.security = secs.length > 0 ? secs[0].value : 'none'
  const kernels = protocolKernelMap[form.protocol] || ['xray']
  if (!kernels.includes(form.kernel_type)) form.kernel_type = kernels[0]
  if (form.protocol === 'hysteria2') form.kernel_type = 'singbox'
  if (form.protocol === 'trojan') form.security = 'tls'
}

function formToSettings(): string {
  const s: Record<string, any> = {}
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
    if (form.reality_short_id) { s.short_id = form.reality_short_id; s.short_ids = form.reality_short_id.split(',').map(x => x.trim()) }
    if (form.fingerprint) s.fingerprint = form.fingerprint
    if (form.flow) s.flow = form.flow
  }
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
  if (form.protocol === 'ss') s.method = form.ss_method
  if (form.protocol === 'hysteria2') {
    if (form.sni) s.sni = form.sni
    if (form.cert_path) s.cert_path = form.cert_path
    if (form.key_path) s.key_path = form.key_path
    if (form.hy2_obfs_type) { s.obfs = form.hy2_obfs_type; s.obfs_password = form.hy2_obfs_password }
    s.max_up_mbps = Number(form.hy2_max_up_mbps) || 0
    s.max_down_mbps = Number(form.hy2_max_down_mbps) || 0
    if (form.allow_insecure) s.skip_cert_verify = true
    if (form.hy2_ignore_client_bandwidth) s.ignore_client_bandwidth = true
  }
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
  form.ws_path = s.path || '/'
  form.ws_host = s.host || ''
  form.h2_path = s.path || '/'
  form.h2_host = s.host || ''
  form.grpc_service_name = s.service_name || s.serviceName || ''
  form.grpc_multi_mode = s.multi_mode || false
  form.ss_method = s.method || 'aes-256-gcm'
  form.hy2_obfs_type = s.obfs || ''
  form.hy2_obfs_password = s.obfs_password || ''
  const upFromSettings = Number(s.max_up_mbps)
  form.hy2_max_up_mbps = Number.isFinite(upFromSettings) ? upFromSettings : 10
  const downFromSettings = Number(s.max_down_mbps)
  form.hy2_max_down_mbps = Number.isFinite(downFromSettings) ? downFromSettings : 10
  form.hy2_ignore_client_bandwidth = !!s.ignore_client_bandwidth
}

function getSecurity(row: any): string {
  try { const s = JSON.parse(row.settings || '{}'); return s.security || (s.tls ? 'tls' : 'none') } catch { return 'none' }
}

function healthTooltip(row: any) {
  const t = row.last_check_at ? new Date(row.last_check_at).toLocaleString() : '-'
  const isQUIC = row.protocol === 'hysteria2' || row.protocol === 'hy2' || row.protocol === 'tuic'
  const note = isQUIC ? '\n注: QUIC 仅探测端口可达性，未验证账号/密码/业务可用性' : ''
  if (row.last_check_ok) return `最近检测: ${t}${note}`
  return `最近检测: ${t}\n失败次数: ${row.fail_count}\n错误: ${row.last_check_err || '-'}${note}`
}

const fetchNodes = async () => {
  loading.value = true
  try {
    const { data } = await getNodes()
    nodes.value = (data.nodes || data || []).map((n: any) => ({ ...n, _switching: false }))
  } catch { toast.error('获取节点列表失败') }
  finally { loading.value = false }
}

const openDialog = (row?: any) => {
  Object.assign(form, defaultForm())
  errors.name = ''; errors.host = ''; errors.port = ''
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
  errors.name = form.name.trim() ? '' : '请输入节点名称'
  errors.host = form.host.trim() ? '' : '请输入主机地址'
  errors.port = form.port ? '' : '请输入端口'
  if (errors.name || errors.host || errors.port) return
  submitting.value = true
  try {
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
      toast.success('节点更新成功')
    } else {
      await createNode(payload)
      toast.success('节点创建成功')
    }
    dialogVisible.value = false
    await fetchNodes()
  } catch (e: any) { toast.error(e.response?.data?.error || '操作失败') }
  finally { submitting.value = false }
}

const handleToggle = async (row: any, val: boolean) => {
  row._switching = true
  try {
    await updateNode(row.id, { enable: val })
    row.enable = val
    toast.success(val ? '已启用' : '已禁用')
  } catch (e: any) { row.enable = !val; toast.error(e.response?.data?.error || '操作失败') }
  finally { row._switching = false }
}

const handleDelete = async (row: any) => {
  try {
    await confirm({ title: '删除节点', message: `确认删除节点「${row.name}」？`, tone: 'danger', confirmText: '删除' })
    await deleteNode(row.id)
    toast.success('删除成功')
    await fetchNodes()
  } catch (e: any) { if (e === 'cancel') return; toast.error(e?.response?.data?.error || '删除失败') }
}

onMounted(() => {
  fetchNodes()
  loadSystemCertPaths()
})
</script>

<style scoped>
.nodes { display: flex; flex-direction: column; gap: 24px; }

.cell-name { font-weight: 600; color: var(--color-ink-strong); }
.cell-addr { color: var(--color-ink-base); font-size: 13px; }
.cell-addr__sep { color: var(--color-ink-soft); padding: 0 1px; }
.cell-meta { font-size: 12px; color: var(--color-ink-muted); text-transform: lowercase; }

.proto-tag {
  font-family: var(--font-mono);
  font-size: 11px; font-weight: 600;
  letter-spacing: 0.04em;
  padding: 3px 7px;
  border-radius: 4px;
  background: var(--color-surface-sunken);
  color: var(--color-ink-base);
  border: 1px solid var(--color-ink-faint);
}
.proto-tag[data-proto="vless"]     { color: var(--color-accent-ink); border-color: var(--color-accent-soft); }
.proto-tag[data-proto="trojan"]    { color: var(--color-status-warn); }
.proto-tag[data-proto="vmess"]     { color: var(--color-status-info); }
.proto-tag[data-proto="ss"]        { color: var(--color-ink-strong); }
.proto-tag[data-proto="hysteria2"] { color: var(--color-status-ok); }

/* Node form — sectioned 2-col grid for breathing room */
.nf-section {
  margin: 0 0 28px;
  padding-top: 18px;
  border-top: 1px solid var(--color-ink-faint);
}
.nf-section:first-of-type {
  border-top: 0;
  padding-top: 0;
}
.nf-section__title {
  font-family: var(--font-serif);
  font-size: 16px;
  font-weight: 600;
  color: var(--color-ink-strong);
  letter-spacing: -0.005em;
  margin: 0 0 14px;
}
.nf-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  column-gap: 20px;
  row-gap: 14px;
  align-items: start;
}
.nf-col-2 { grid-column: span 2; }

.nf-cert {
  display: flex;
  gap: 8px;
  align-items: stretch;
}
.nf-cert > :first-child { flex: 1; min-width: 0; }

/* Switch rows: label/hint on left, Switch on right */
.nf-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 4px 0;
}
.nf-toggle__head { display: flex; flex-direction: column; gap: 2px; }
.nf-toggle__label {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-ink-strong);
}
.nf-toggle__hint {
  font-size: 12px;
  color: var(--color-ink-muted);
}

/* Reality keypair action button stays compact */
.nf-keypair :deep(.field__control) {
  align-items: flex-start;
}

@media (max-width: 720px) {
  .nf-grid { grid-template-columns: 1fr; }
  .nf-col-2 { grid-column: auto; }
}
</style>
