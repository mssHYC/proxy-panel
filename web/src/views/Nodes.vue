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
            <el-tag size="small">{{ row.protocol }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="传输" width="100">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ row.transport || '-' }}</el-tag>
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
      width="560px"
      destroy-on-close
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="100px"
      >
        <el-form-item label="节点名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入节点名称" />
        </el-form-item>
        <el-form-item label="主机地址" prop="host">
          <el-input v-model="form.host" placeholder="请输入主机地址" />
        </el-form-item>
        <el-form-item label="端口" prop="port">
          <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 100%" />
        </el-form-item>
        <el-form-item label="协议" prop="protocol">
          <el-select v-model="form.protocol" placeholder="请选择协议" style="width: 100%">
            <el-option v-for="p in protocols" :key="p" :label="p" :value="p" />
          </el-select>
        </el-form-item>
        <el-form-item label="传输方式" prop="transport">
          <el-select v-model="form.transport" placeholder="请选择传输方式" clearable style="width: 100%">
            <el-option v-for="t in transports" :key="t" :label="t" :value="t" />
          </el-select>
        </el-form-item>
        <el-form-item label="内核类型" prop="kernel_type">
          <el-select v-model="form.kernel_type" placeholder="请选择内核" style="width: 100%">
            <el-option label="xray" value="xray" />
            <el-option label="singbox" value="singbox" />
          </el-select>
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" style="width: 100%" />
        </el-form-item>
        <el-form-item label="协议配置">
          <el-input
            v-model="form.settings"
            type="textarea"
            :rows="4"
            placeholder='{"sni": "example.com", "path": "/ws", "tls": true}'
          />
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
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { getNodes, createNode, updateNode, deleteNode } from '../api/node'

const loading = ref(false)
const nodes = ref<any[]>([])
const dialogVisible = ref(false)
const isEdit = ref(false)
const editId = ref<number | null>(null)
const submitting = ref(false)
const formRef = ref<FormInstance>()

const protocols = ['vless', 'vmess', 'trojan', 'ss', 'hysteria2']
const transports = ['tcp', 'ws', 'grpc', 'h2', 'reality']

const defaultForm = () => ({
  name: '',
  host: '',
  port: 443,
  protocol: '',
  transport: '',
  kernel_type: 'xray',
  sort_order: 0,
  settings: '',
})

const form = reactive(defaultForm())

const rules: FormRules = {
  name: [{ required: true, message: '请输入节点名称', trigger: 'blur' }],
  host: [{ required: true, message: '请输入主机地址', trigger: 'blur' }],
  port: [{ required: true, message: '请输入端口', trigger: 'blur' }],
  protocol: [{ required: true, message: '请选择协议', trigger: 'change' }],
}

const fetchNodes = async () => {
  loading.value = true
  try {
    const { data } = await getNodes()
    nodes.value = (data.nodes || data || []).map((n: any) => ({ ...n, _switching: false }))
  } catch (e) {
    console.error('获取节点列表失败', e)
  } finally {
    loading.value = false
  }
}

const openDialog = (row?: any) => {
  if (row) {
    isEdit.value = true
    editId.value = row.id
    Object.assign(form, {
      name: row.name,
      host: row.host,
      port: row.port,
      protocol: row.protocol,
      transport: row.transport || '',
      kernel_type: row.kernel_type || 'xray',
      sort_order: row.sort_order || 0,
      settings: row.settings ? (typeof row.settings === 'string' ? row.settings : JSON.stringify(row.settings, null, 2)) : '',
    })
  } else {
    isEdit.value = false
    editId.value = null
    Object.assign(form, defaultForm())
  }
  dialogVisible.value = true
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  submitting.value = true
  try {
    const payload: any = { ...form }
    // 尝试解析 settings 为 JSON
    if (payload.settings) {
      try {
        payload.settings = JSON.parse(payload.settings)
      } catch {
        // 保持字符串
      }
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
    ElMessage.error(e.response?.data?.message || '操作失败')
  } finally {
    submitting.value = false
  }
}

const handleToggle = async (row: any, val: boolean) => {
  row._switching = true
  try {
    await updateNode(row.id, { ...row, enable: val })
    ElMessage.success(val ? '已启用' : '已禁用')
  } catch (e: any) {
    row.enable = !val
    ElMessage.error(e.response?.data?.message || '操作失败')
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
    ElMessage.error(e.response?.data?.message || '删除失败')
  }
}

onMounted(() => {
  fetchNodes()
})
</script>
