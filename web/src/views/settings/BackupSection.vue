<template>
  <div class="s-blocks">
    <section class="s-block">
      <div class="s-block__head">
        <h3 class="s-block__title">备份与恢复</h3>
        <p class="s-block__hint">
          导出获取当前 SQLite 数据库的一致性快照。恢复会覆盖现有数据并重启服务，需要 systemd 托管。
        </p>
      </div>

      <div class="backup-actions">
        <Button variant="primary" :loading="exporting" @click="handleExport">
          <Download :size="14" :stroke-width="1.6" /> 导出数据库
        </Button>
        <FileInput accept=".db" label="选择 .db 文件并恢复" @change="onPick" :disabled="importing" />
      </div>

      <Alert tone="warn">
        恢复操作不可撤销，请先导出当前数据作为备份。
      </Alert>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Download } from 'lucide-vue-next'
import { Button, FileInput, Alert, toast, confirm } from '../../ui'
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
  } finally { exporting.value = false }
}

async function onPick(files: File[]) {
  const file = files[0]
  if (!file) return
  try {
    await confirm({
      title: '确认恢复',
      message: '恢复将覆盖当前所有数据并重启服务，确认继续？',
      tone: 'danger',
      confirmText: '恢复',
    })
  } catch { return }
  importing.value = true
  try {
    await importBackup(file)
    toast.success('导入完成，服务正在重启，请 60 秒后刷新')
  } catch (e: any) { toast.error(e?.response?.data?.error || '导入失败') }
  finally { importing.value = false }
}
</script>

<style scoped>
.backup-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  padding: 8px 0 4px;
}
</style>
