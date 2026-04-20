<template>
  <el-card>
    <template #header>数据备份 / 恢复</template>
    <el-alert type="info" :closable="false" style="margin-bottom:12px">
      导出：获取当前 SQLite 数据库一致性快照。恢复：上传后服务将自动重启（需 systemd 托管）。
    </el-alert>
    <el-space wrap>
      <el-button type="primary" :loading="exporting" @click="handleExport">导出数据库</el-button>
      <el-upload
        :auto-upload="false"
        :show-file-list="false"
        accept=".db"
        :on-change="(f: any) => pickFile(f.raw)"
      >
        <el-button :loading="importing">选择文件并恢复</el-button>
      </el-upload>
    </el-space>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { exportBackup, importBackup } from '../../api/backup'

const exporting = ref(false)
const importing = ref(false)

async function handleExport() {
  exporting.value = true
  try {
    const { data } = await exportBackup()
    const blob = new Blob([data as BlobPart], { type: 'application/x-sqlite3' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `proxy-panel-${new Date().toISOString().replace(/[:.]/g, '-')}.db`
    a.click()
    URL.revokeObjectURL(url)
  } finally {
    exporting.value = false
  }
}

async function pickFile(file: File) {
  if (!file) return
  try {
    await ElMessageBox.confirm(
      '恢复将覆盖当前所有数据并重启服务，确认继续？',
      '确认恢复',
      { type: 'warning' }
    )
  } catch {
    return
  }
  importing.value = true
  try {
    await importBackup(file)
    ElMessage.success('导入完成，服务正在重启，请 60 秒后刷新')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '导入失败')
  } finally {
    importing.value = false
  }
}
</script>
