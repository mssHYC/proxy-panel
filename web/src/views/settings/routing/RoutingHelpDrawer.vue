<template>
  <Drawer :open="visible" title="分流规则配置教程" :width="540" @update:open="(v) => (visible = v)">
    <Collapse :model-value="activeNames" :items="items" multiple @update:model-value="(v) => (activeNames = v)">
      <template #concepts>
        <p>分流规则决定你的网络流量走哪条线路。整体数据流如下：</p>
        <p class="flow">流量进入 → 匹配「分类」或「自定义规则」 → 转发到「出站组」 → 出站组选择具体节点</p>
        <p><strong>三个核心概念：</strong></p>
        <ul>
          <li><strong>出站组</strong> 定义一组节点的选择策略（手动选择或自动测速），是流量的"出口"</li>
          <li><strong>规则分类</strong> 通过引用远程规则集（Site Tags / IP Tags）批量匹配一大类流量，指向某个出站组</li>
          <li><strong>自定义规则</strong> 手动填写域名、IP 等条件精确匹配流量，指向出站组或直连/拒绝</li>
        </ul>
        <p><strong>匹配优先级：</strong></p>
        <ol>
          <li>自定义规则（优先级最高）</li>
          <li>规则分类（按排序值从小到大）</li>
          <li>兜底出站组（所有规则都没匹配到的流量）</li>
        </ol>
      </template>

      <template #categories>
        <h4>预设方案</h4>
        <p>系统提供三个预设，一键切换启用哪些分类：</p>
        <ul>
          <li><strong>最小规则</strong> 仅启用局域网、国内直连、Non-China，适合简单使用</li>
          <li><strong>均衡规则</strong> 在最小基础上增加 Google、YouTube、GitHub、AI 服务、Telegram</li>
          <li><strong>完整规则</strong> 启用所有 18 个系统分类</li>
        </ul>
        <p>点击「应用（覆盖启用分类）」会<strong>覆盖</strong>所有分类的启用状态。</p>

        <h4>新增自定义分类</h4>
        <ul>
          <li><Tag>Code</Tag> 唯一标识符，如 <code>my_spotify</code></li>
          <li><Tag>显示名</Tag> 界面显示的名称</li>
          <li><Tag>Site Tags</Tag> 引用 geosite 远程规则集的名称（如 <code>spotify</code>、<code>google</code>）</li>
          <li><Tag>IP Tags</Tag> 引用 geoip 远程规则集（如 <code>cn</code>、<code>telegram</code>）</li>
          <li><Tag>内联 domain_suffix</Tag> 手动填写域名后缀</li>
          <li><Tag>内联 domain_keyword</Tag> 手动填写域名关键词</li>
          <li><Tag>内联 ip_cidr</Tag> 手动填写 IP 段，如 <code>10.0.0.0/8</code></li>
          <li><Tag>默认出站组</Tag> 匹配到的流量走哪个出站组</li>
          <li><Tag>排序</Tag> 数值越小越靠前</li>
        </ul>
      </template>

      <template #groups>
        <h4>两种类型</h4>
        <ul>
          <li><strong>selector（手动选择）</strong> 在客户端中手动切换使用哪个节点</li>
          <li><strong>urltest（自动测速）</strong> 自动选择延迟最低的节点</li>
        </ul>
        <h4>成员配置</h4>
        <ul>
          <li>填写具体节点名称，如 <code>香港01</code>、<code>日本02</code></li>
          <li><code>&lt;ALL&gt;</code> 特殊宏，自动展开为所有可用节点</li>
          <li><code>DIRECT</code> 直连，不走代理</li>
          <li>也可以引用其他出站组的 Code，如 <code>auto_select</code></li>
        </ul>
      </template>

      <template #rules>
        <h4>与分类的区别</h4>
        <ul>
          <li><strong>分类</strong> 通过 Site Tags / IP Tags 引用远程规则集，批量匹配一整类网站</li>
          <li><strong>自定义规则</strong> 手动填写具体的域名/IP，精确匹配特定目标</li>
          <li>自定义规则<strong>不受预设方案影响</strong>，始终生效</li>
          <li>自定义规则优先级高于分类</li>
        </ul>
      </template>

      <template #advanced>
        <h4>URL 前缀覆写</h4>
        <p>控制客户端从哪里下载规则集文件。默认走 GitHub 加速镜像，<strong>一般不需要修改</strong>。</p>
        <h4>兜底出站组</h4>
        <p>所有分类和自定义规则都没匹配到的流量，最终走哪个出站组。默认是「漏网之鱼」。</p>
        <h4>从旧格式导入</h4>
        <p>每行一条：<code>TYPE,VALUE,OUTBOUND</code></p>
        <pre>DOMAIN-SUFFIX,google.com,Google
DOMAIN-KEYWORD,spotify,DIRECT
IP-CIDR,91.108.0.0/16,Telegram</pre>
      </template>

      <template #example>
        <h4>第 1 步 新增出站组</h4>
        <ul>
          <li>Code: <code>spotify_nodes</code></li>
          <li>显示名: <code>Spotify 专用</code></li>
          <li>类型: <code>selector</code> 或 <code>urltest</code></li>
          <li>成员: 选择你的节点</li>
        </ul>
        <h4>第 2 步 新增自定义分类</h4>
        <ul>
          <li>Code: <code>my_spotify</code></li>
          <li>显示名: <code>Spotify 音乐</code></li>
          <li>Site Tags: <code>spotify</code></li>
          <li>默认出站组: 「Spotify 专用」</li>
          <li>排序: <code>45</code></li>
        </ul>
      </template>
    </Collapse>
  </Drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Drawer, Collapse, Tag } from '../../../ui'

const visible = defineModel<boolean>({ default: false })
const activeNames = ref<string[]>([])

const items = [
  { value: 'concepts',   title: '核心概念' },
  { value: 'categories', title: '预设方案与规则分类' },
  { value: 'groups',     title: '出站组' },
  { value: 'rules',      title: '自定义规则' },
  { value: 'advanced',   title: '高级设置' },
  { value: 'example',    title: '完整示例：让 Spotify 走专用节点' },
]
</script>

<style scoped>
h4 { margin: 12px 0 6px; font-size: 14px; font-weight: 600; color: var(--color-ink-strong); }
p { margin: 6px 0; line-height: 1.65; }
ul, ol { margin: 6px 0; padding-left: 20px; line-height: 1.85; }
code {
  background: var(--color-surface-sunken);
  padding: 1px 6px;
  border-radius: 3px;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-ink-base);
}
pre {
  background: var(--color-surface-sunken);
  padding: 10px 14px;
  border-radius: 6px;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.7;
  overflow-x: auto;
  color: var(--color-ink-base);
}
.flow {
  background: var(--color-accent-soft);
  color: var(--color-accent-ink);
  padding: 8px 14px;
  border-radius: 6px;
  font-weight: 500;
  text-align: center;
}
</style>
