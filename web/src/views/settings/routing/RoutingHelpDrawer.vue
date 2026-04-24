<template>
  <el-drawer v-model="visible" title="分流规则配置教程" direction="rtl" size="520px">
    <el-collapse v-model="activeNames">
      <!-- 1. 核心概念 -->
      <el-collapse-item title="核心概念" name="concepts">
        <p>分流规则决定你的网络流量走哪条线路。整体数据流如下：</p>
        <p class="flow">流量进入 → 匹配「分类」或「自定义规则」 → 转发到「出站组」 → 出站组选择具体节点</p>
        <p><strong>三个核心概念：</strong></p>
        <ul>
          <li><strong>出站组</strong> — 定义一组节点的选择策略（手动选择或自动测速），是流量的"出口"</li>
          <li><strong>规则分类</strong> — 通过引用远程规则集（Site Tags / IP Tags）批量匹配一大类流量，指向某个出站组</li>
          <li><strong>自定义规则</strong> — 手动填写域名、IP 等条件精确匹配流量，指向出站组或直连/拒绝</li>
        </ul>
        <p><strong>匹配优先级：</strong></p>
        <ol>
          <li>自定义规则（优先级最高）</li>
          <li>规则分类（按排序值从小到大）</li>
          <li>兜底出站组（所有规则都没匹配到的流量）</li>
        </ol>
      </el-collapse-item>

      <!-- 2. 预设方案与规则分类 -->
      <el-collapse-item title="预设方案与规则分类" name="categories">
        <h4>预设方案</h4>
        <p>系统提供三个预设，一键切换启用哪些分类：</p>
        <ul>
          <li><strong>最小规则</strong> — 仅启用局域网、国内直连、Non-China，适合简单使用</li>
          <li><strong>均衡规则</strong> — 在最小基础上增加 Google、YouTube、GitHub、AI 服务、Telegram</li>
          <li><strong>完整规则</strong> — 启用所有 18 个系统分类</li>
        </ul>
        <p>点击「应用（覆盖启用分类）」会<strong>覆盖</strong>所有分类的启用状态，请注意。</p>

        <h4>新增自定义分类</h4>
        <p>点击「+ 新增自定义分类」，各字段说明：</p>
        <ul>
          <li><el-tag size="small">Code</el-tag> — 唯一标识符，如 <code>my_spotify</code>，不可重复</li>
          <li><el-tag size="small">显示名</el-tag> — 界面显示的名称，如「Spotify 音乐」</li>
          <li><el-tag size="small">Site Tags</el-tag> — 引用 geosite 远程规则集的名称（如 <code>spotify</code>、<code>google</code>），客户端会自动下载对应的域名规则集来匹配流量。数据来源：MetaCubeX/meta-rules-dat</li>
          <li><el-tag size="small">IP Tags</el-tag> — 引用 geoip 远程规则集的名称（如 <code>cn</code>、<code>telegram</code>），匹配 IP 地址</li>
          <li><el-tag size="small">内联 domain_suffix</el-tag> — 手动填写域名后缀，如填 <code>example.com</code> 可匹配 <code>example.com</code> 及其所有子域名</li>
          <li><el-tag size="small">内联 domain_keyword</el-tag> — 手动填写域名关键词，域名中包含该关键词即匹配</li>
          <li><el-tag size="small">内联 ip_cidr</el-tag> — 手动填写 IP 段，如 <code>10.0.0.0/8</code></li>
          <li><el-tag size="small">默认出站组</el-tag> — 匹配到的流量走哪个出站组</li>
          <li><el-tag size="small">排序</el-tag> — 数值越小越靠前，优先匹配</li>
        </ul>
        <p><strong>提示：</strong>大多数情况下只需要填 Site Tags + 出站组就够了，内联字段用于补充规则集覆盖不到的域名/IP。</p>
      </el-collapse-item>

      <!-- 3. 出站组 -->
      <el-collapse-item title="出站组" name="groups">
        <h4>两种类型</h4>
        <ul>
          <li><strong>selector（手动选择）</strong> — 在客户端中手动切换使用哪个节点</li>
          <li><strong>urltest（自动测速）</strong> — 自动选择延迟最低的节点</li>
        </ul>

        <h4>成员配置</h4>
        <ul>
          <li>填写具体节点名称，如 <code>香港01</code>、<code>日本02</code></li>
          <li><code>&lt;ALL&gt;</code> — 特殊宏，自动展开为所有可用节点</li>
          <li><code>DIRECT</code> — 直连，不走代理</li>
          <li>也可以引用其他出站组的 Code，如 <code>auto_select</code></li>
        </ul>

        <h4>新建步骤</h4>
        <ol>
          <li>进入「出站组」tab，点击「+ 新增自定义组」</li>
          <li>填写 Code（唯一标识）和显示名</li>
          <li>选择类型（selector 或 urltest）</li>
          <li>添加成员节点</li>
          <li>保存</li>
        </ol>
      </el-collapse-item>

      <!-- 4. 自定义规则 -->
      <el-collapse-item title="自定义规则" name="rules">
        <h4>与分类的区别</h4>
        <ul>
          <li><strong>分类</strong> — 通过 Site Tags / IP Tags 引用远程规则集，批量匹配一整类网站</li>
          <li><strong>自定义规则</strong> — 手动填写具体的域名/IP，精确匹配特定目标</li>
          <li>自定义规则<strong>不受预设方案影响</strong>，始终生效</li>
          <li>自定义规则优先级高于分类</li>
        </ul>

        <h4>各字段说明</h4>
        <ul>
          <li><el-tag size="small">名称</el-tag> — 规则的描述性名称</li>
          <li><el-tag size="small">Site Tags</el-tag> — 同分类中的 Site Tags，引用 geosite 规则集</li>
          <li><el-tag size="small">IP Tags</el-tag> — 同分类中的 IP Tags，引用 geoip 规则集</li>
          <li><el-tag size="small">Domain Suffix</el-tag> — 域名后缀匹配，如 <code>scdn.co</code></li>
          <li><el-tag size="small">Domain Keyword</el-tag> — 域名关键词匹配</li>
          <li><el-tag size="small">IP CIDR</el-tag> — IP 段匹配，如 <code>91.108.0.0/16</code></li>
          <li><el-tag size="small">Src IP CIDR</el-tag> — 源 IP 匹配，用于按来源 IP 分流</li>
          <li><el-tag size="small">出站</el-tag> — 选择一个出站组，或直接选 <code>DIRECT</code>（直连）/ <code>REJECT</code>（拒绝）</li>
          <li><el-tag size="small">排序</el-tag> — 数值越小越靠前</li>
        </ul>
      </el-collapse-item>

      <!-- 5. 高级设置 -->
      <el-collapse-item title="高级设置" name="advanced">
        <h4>URL 前缀覆写</h4>
        <p>控制客户端从哪里下载规则集文件。默认使用 GitHub 加速镜像，<strong>一般不需要修改</strong>。</p>
        <p>需要修改的情况：</p>
        <ul>
          <li>默认镜像不可用 → 换成其他 GitHub 加速地址</li>
          <li>自建了规则集仓库 → 填你自己的地址</li>
        </ul>

        <h4>兜底出站组</h4>
        <p>所有分类和自定义规则都没匹配到的流量，最终走哪个出站组。默认是「漏网之鱼」。</p>

        <h4>从旧格式导入</h4>
        <p>从其他工具迁移时，将旧规则文本粘贴进来批量导入为自定义规则。</p>
        <p>格式为每行一条：<code>TYPE,VALUE,OUTBOUND</code></p>
        <p>示例：</p>
        <pre>DOMAIN-SUFFIX,google.com,Google
DOMAIN-KEYWORD,spotify,DIRECT
IP-CIDR,91.108.0.0/16,Telegram</pre>
        <p>两种模式：</p>
        <ul>
          <li><strong>追加</strong> — 导入的规则加到现有规则前面，系统分类不变</li>
          <li><strong>覆盖</strong> — 导入后关闭所有系统分类，完全使用导入的规则</li>
        </ul>
      </el-collapse-item>

      <!-- 6. 完整示例 -->
      <el-collapse-item title="完整示例：让 Spotify 走专用节点" name="example">
        <h4>第 1 步：新增出站组</h4>
        <p>进入「出站组」tab → 点击「+ 新增自定义组」：</p>
        <ul>
          <li>Code: <code>spotify_nodes</code></li>
          <li>显示名: <code>Spotify 专用</code></li>
          <li>类型: <code>selector</code>（手动选）或 <code>urltest</code>（自动测速）</li>
          <li>成员: 选择你的节点，如 <code>香港01</code>、<code>日本02</code></li>
        </ul>

        <h4>第 2 步：新增自定义分类</h4>
        <p>进入「规则分类」tab → 点击「+ 新增自定义分类」：</p>
        <ul>
          <li>Code: <code>my_spotify</code></li>
          <li>显示名: <code>Spotify 音乐</code></li>
          <li>Site Tags: 输入 <code>spotify</code> 后按回车</li>
          <li>默认出站组: 选择刚创建的「Spotify 专用」</li>
          <li>排序: <code>45</code></li>
        </ul>
        <p>保存后确保启用开关已打开。</p>

        <h4>第 3 步（可选）：新增自定义规则</h4>
        <p>如果有 geosite 规则集未覆盖的域名，进入「自定义规则」tab → 点击「+ 新增规则」：</p>
        <ul>
          <li>名称: <code>spotify-cdn</code></li>
          <li>Domain Suffix: <code>scdn.co</code>, <code>spotifycdn.com</code></li>
          <li>出站: 选择「Spotify 专用」</li>
        </ul>
        <p>这样 Spotify 的所有流量就会走你的专用节点了。</p>
      </el-collapse-item>
    </el-collapse>
  </el-drawer>
</template>

<script setup lang="ts">
const visible = defineModel<boolean>({ default: false })
const activeNames = ref<string[]>([])

import { ref } from 'vue'
</script>

<style scoped>
h4 {
  margin: 12px 0 6px;
  font-size: 14px;
  font-weight: 600;
}
p {
  margin: 6px 0;
  line-height: 1.6;
}
ul, ol {
  margin: 6px 0;
  padding-left: 20px;
  line-height: 1.8;
}
code {
  background: var(--el-fill-color-light);
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 13px;
}
pre {
  background: var(--el-fill-color-light);
  padding: 10px 14px;
  border-radius: 6px;
  font-size: 13px;
  line-height: 1.6;
  overflow-x: auto;
}
.flow {
  background: var(--el-color-primary-light-9);
  padding: 8px 14px;
  border-radius: 6px;
  font-weight: 500;
  text-align: center;
}
</style>
