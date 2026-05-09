<template>
  <div class="dashboard" :class="{ 'is-loading-overlay': loading }">
    <section class="summary">
      <p class="summary__line">
        今天到目前为止，
        <em class="num">{{ formatBytes(todayTotal) }}</em>
        流量穿过 ProxyPanel，由
        <em class="num">{{ dashboard.users.enabled }}</em>
        位活跃用户消耗，分发自
        <em class="num">{{ dashboard.nodes.enabled }}</em>
        个在线节点。
      </p>
      <p class="summary__meta">
        共有 {{ dashboard.users.total }} 位注册用户，{{ dashboard.nodes.total }} 个节点。
      </p>
    </section>

    <hr class="divider-h" />

    <section>
      <header class="section-head">
        <div>
          <p class="eyebrow">服务器流量</p>
          <h2 class="section-head__title">本周期已用</h2>
        </div>
        <span v-if="hasLimit" class="section-head__hint">
          配额 <span class="num">{{ formatBytes(dashboard.server_traffic.limit_bytes) }}</span>
        </span>
        <span v-else class="section-head__hint">未设置配额</span>
      </header>

      <div class="usage">
        <div class="usage__numbers">
          <span class="usage__total num">{{ formatBytes(serverUsed) }}</span>
          <span v-if="hasLimit" class="usage__pct num" :data-state="quotaState">
            {{ usagePercent }}%
          </span>
        </div>
        <ProgressBar v-if="hasLimit" :percent="usagePercent" :thresholds="{ warn: 80, crit: 100 }" />
        <dl class="usage__split">
          <div>
            <dt>上行</dt>
            <dd class="num">{{ formatBytes(dashboard.server_traffic.total_up) }}</dd>
          </div>
          <div>
            <dt>下行</dt>
            <dd class="num">{{ formatBytes(dashboard.server_traffic.total_down) }}</dd>
          </div>
          <div>
            <dt>今日</dt>
            <dd class="num">{{ formatBytes(todayTotal) }}</dd>
          </div>
        </dl>
      </div>
    </section>

    <hr class="divider-h" />

    <section>
      <header class="section-head">
        <div>
          <p class="eyebrow">内核</p>
          <h2 class="section-head__title">运行状态</h2>
        </div>
      </header>

      <ul class="kernels">
        <li v-for="k in kernels" :key="k.name" class="kernel">
          <div class="kernel__id">
            <StatusDot :state="k.running ? 'ok' : 'crit'" pulse />
            <span class="kernel__name">{{ k.label }}</span>
            <span class="kernel__state">{{ k.running ? '运行中' : '已停止' }}</span>
          </div>
          <Button variant="secondary" :loading="restartingKernel === k.name" @click="restartKernel(k.name)">
            重启
          </Button>
        </li>
      </ul>
    </section>

    <hr class="divider-h" />

    <section>
      <header class="section-head">
        <div>
          <p class="eyebrow">趋势</p>
          <h2 class="section-head__title">近 30 天流量</h2>
        </div>
        <span class="section-head__hint">堆叠：上行 + 下行</span>
      </header>
      <TrafficChart />
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, defineAsyncComponent, onMounted } from 'vue'
import { Button, StatusDot, ProgressBar, toast } from '../ui'
import { getDashboard } from '../api/dashboard'
import request from '../api/request'
import { formatBytes } from '../utils/format'

const TrafficChart = defineAsyncComponent(() => import('../components/TrafficChart.vue'))

const loading = ref(false)
const restartingKernel = ref('')

const dashboard = ref({
  users: { total: 0, enabled: 0 },
  nodes: { total: 0, enabled: 0 },
  server_traffic: { total_up: 0, total_down: 0, limit_bytes: 0 },
  today_traffic: { upload: 0, download: 0, total: 0 },
  kernel_status: { xray: false, 'sing-box': false } as Record<string, boolean>,
})

const serverUsed = computed(() => {
  const t = dashboard.value.server_traffic
  return t.total_up + t.total_down
})
const hasLimit = computed(() => dashboard.value.server_traffic.limit_bytes > 0)
const usagePercent = computed(() => {
  const limit = dashboard.value.server_traffic.limit_bytes
  if (limit <= 0) return 0
  return Math.round((serverUsed.value / limit) * 100)
})
const quotaState = computed(() => {
  if (!hasLimit.value) return 'info'
  if (usagePercent.value >= 100) return 'crit'
  if (usagePercent.value >= 80) return 'warn'
  return 'ok'
})
const todayTotal = computed(() => {
  const t = dashboard.value.today_traffic
  return (t.upload || 0) + (t.download || 0)
})
const kernels = computed(() => [
  { name: 'xray',     label: 'Xray',     running: dashboard.value.kernel_status.xray },
  { name: 'sing-box', label: 'Sing-box', running: dashboard.value.kernel_status['sing-box'] },
])

const fetchDashboard = async () => {
  loading.value = true
  try {
    const { data } = await getDashboard()
    dashboard.value = data
  } catch (e) {
    console.error('获取仪表盘数据失败', e)
  } finally {
    loading.value = false
  }
}

const restartKernel = async (name: string) => {
  restartingKernel.value = name
  try {
    await request.post('/kernel/restart', { name })
    toast.success(`${name} 重启成功`)
    await fetchDashboard()
  } catch (e: any) {
    toast.error(e.response?.data?.message || `${name} 重启失败`)
  } finally {
    restartingKernel.value = ''
  }
}

onMounted(fetchDashboard)
</script>

<style scoped>
.dashboard { display: flex; flex-direction: column; gap: 40px; }

.summary { padding-top: 8px; }
.summary__line {
  font-family: var(--font-serif);
  font-size: 22px;
  line-height: 1.55;
  color: var(--color-ink-strong);
  font-weight: 500;
  letter-spacing: -0.005em;
  max-width: 68ch;
  margin: 0;
}
.summary__line .num {
  font-family: var(--font-mono);
  font-style: normal;
  font-weight: 600;
  color: var(--color-accent-ink);
  font-feature-settings: 'tnum';
  padding: 0 2px;
}
.summary__meta {
  margin: 12px 0 0;
  font-size: 13px;
  color: var(--color-ink-muted);
}

.usage { display: flex; flex-direction: column; gap: 20px; max-width: 720px; }
.usage__numbers { display: flex; align-items: baseline; gap: 16px; }
.usage__total {
  font-family: var(--font-mono);
  font-size: 36px;
  line-height: 1;
  font-weight: 600;
  color: var(--color-ink-strong);
  letter-spacing: -0.02em;
}
.usage__pct {
  font-family: var(--font-mono);
  font-size: 15px;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 4px;
  background: var(--color-status-ok-soft);
  color: var(--color-status-ok);
}
.usage__pct[data-state="warn"] { background: var(--color-status-warn-soft); color: var(--color-status-warn); }
.usage__pct[data-state="crit"] { background: var(--color-status-crit-soft); color: var(--color-status-crit); }

.usage__split {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 160px));
  gap: 24px;
  margin: 0;
}
.usage__split div { display: flex; flex-direction: column; gap: 4px; }
.usage__split dt { font-size: 11px; font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; color: var(--color-ink-muted); margin: 0; }
.usage__split dd { margin: 0; font-size: 17px; font-weight: 600; color: var(--color-ink-strong); font-family: var(--font-mono); }

.kernels {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--color-ink-faint);
  max-width: 720px;
}
.kernel {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 4px;
  border-bottom: 1px solid var(--color-ink-faint);
}
.kernel__id { display: flex; align-items: baseline; gap: 14px; }
.kernel__name { font-weight: 600; color: var(--color-ink-strong); min-width: 80px; }
.kernel__state { color: var(--color-ink-muted); font-size: 13px; }
</style>
