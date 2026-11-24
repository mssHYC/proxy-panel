<template>
  <details class="candidates">
    <summary>筛选候选网站</summary>
    <div class="candidates__body">
      <p>Fedora 镜像目录按 VPS 出口推荐站点。地区来自镜像登记信息，无法精确到机房。</p>
      <Button :loading="loading" :disabled="scanning" @click="loadCatalog">获取附近镜像</Button>
      <ul v-if="mirrors.length" class="candidates__mirrors" aria-label="附近镜像">
        <li v-for="mirror in mirrors" :key="mirror.host">
          <label>
            <input v-model="selected" type="checkbox" :value="mirror.host" :disabled="scanning" />
            {{ mirror.host }} <small>{{ regionName(mirror.country) }}</small>
          </label>
        </li>
      </ul>
      <label for="reality-custom-hosts">自选域名</label>
      <Textarea id="reality-custom-hosts" v-model="custom" :disabled="scanning" :rows="2" placeholder="每行一个域名，可与目录中的镜像一起检测" />
      <Button :loading="scanning" :disabled="loading || !hosts.length || hosts.length > 8" @click="scan">
        检测所选域名<span v-if="hosts.length">（{{ hosts.length }}）</span>
      </Button>
      <p v-if="hosts.length > 8" role="alert">每次最多检测 8 个域名，请减少所选数量。</p>
      <p v-if="error" role="alert">{{ error }}</p>
      <ul v-if="results.length" class="candidates__results" aria-label="候选检测结果" aria-live="polite">
        <li v-for="result in results" :key="result.host">
          <div>
            <strong>{{ result.host }}</strong>
            <p>{{ result.message }}<span v-if="result.latency_ms != null"> · {{ result.latency_ms }} ms</span></p>
            <small v-if="result.cdn_evidence">{{ result.cdn_provider === 'unknown' ? '代理或缓存特征' : result.cdn_provider }} · {{ result.cdn_evidence }}</small>
            <small v-else-if="result.status === 'compatible'">未识别到已知 CDN 特征</small>
          </div>
          <Button v-if="result.status === 'compatible' && result.cdn_provider === 'unknown' && !result.cdn_evidence" @click="emit('select', result.host)">填入 Dest 和 SNI</Button>
        </li>
      </ul>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Button, Textarea } from '../ui'
import { checkRealityCandidates, getRealityCatalog, type RealityCandidate, type RealityMirror } from '../api/node'

const emit = defineEmits<{ select: [host: string] }>()
const mirrors = ref<RealityMirror[]>([])
const selected = ref<string[]>([])
const custom = ref('')
const results = ref<RealityCandidate[]>([])
const loading = ref(false)
const scanning = ref(false)
const error = ref('')
const requests = new AbortController()
onBeforeUnmount(() => requests.abort())

const regions = new Intl.DisplayNames(['zh-CN'], { type: 'region' })
const regionName = (country: string) => country ? regions.of(country) : '地区未知'
const hosts = computed(() => [...new Set([
  ...selected.value,
  ...custom.value.toLowerCase().split(/[\s,，]+/).filter(Boolean),
])])
watch(hosts, () => { results.value = []; error.value = '' })

async function loadCatalog() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await getRealityCatalog(requests.signal)
    mirrors.value = data.mirrors
    selected.value = data.mirrors.map(m => m.host)
  } catch (e: any) {
    if (!requests.signal.aborted) error.value = e.response?.data?.error || '获取目录失败，可手动输入候选域名'
  } finally { loading.value = false }
}

async function scan() {
  scanning.value = true
  error.value = ''
  results.value = []
  try {
    const { data } = await checkRealityCandidates(hosts.value, requests.signal)
    results.value = data.results
  } catch (e: any) {
    if (!requests.signal.aborted) error.value = e.response?.data?.error || '检测失败，请重试'
  } finally { scanning.value = false }
}
</script>

<style scoped>
.candidates { font-size: 13px; }
.candidates summary { cursor: pointer; color: var(--color-accent-ink); font-weight: 600; }
.candidates__body { display: grid; gap: 9px; margin-top: 12px; }
.candidates__body > button { justify-self: start; }
.candidates p, .candidates small { margin: 0; color: var(--color-ink-muted); }
.candidates ul { list-style: none; padding: 0; margin: 0; }
.candidates__mirrors { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr)); gap: 6px; }
.candidates__mirrors label { display: flex; align-items: baseline; gap: 5px; overflow-wrap: anywhere; }
.candidates__results li { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; padding: 8px 0; border-top: 1px solid var(--color-ink-faint); }
.candidates__results li > div { min-width: 0; overflow-wrap: anywhere; }
</style>
