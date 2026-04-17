// 支持的规则类型（与 Clash / mihomo / sing-box 共通）
export const RULE_TYPES = [
  'DOMAIN', 'DOMAIN-SUFFIX', 'DOMAIN-KEYWORD',
  'IP-CIDR', 'IP-CIDR6', 'GEOSITE', 'GEOIP', 'PROCESS-NAME',
] as const

export type RuleType = typeof RULE_TYPES[number]

// 只有这几种类型支持 no-resolve 标志
export const IP_RULE_TYPES: readonly string[] = ['IP-CIDR', 'IP-CIDR6', 'GEOIP']

// 可选择的目标策略组（来自原 Settings.vue:192 的提示列表）
export const TARGET_GROUPS = [
  '手动切换', '自动选择', '全球代理', '流媒体', 'Telegram', 'Google', 'YouTube',
  'Netflix', 'Spotify', 'HBO', 'Bing', 'OpenAI', 'ClaudeAI', 'Disney', 'GitHub',
  '国内媒体', '本地直连', '漏网之鱼', 'DIRECT', 'REJECT',
] as const

export interface Rule {
  type: RuleType | 'UNKNOWN'
  value: string
  target: string
  noResolve: boolean
  raw?: string  // UNKNOWN 行保留原始文本，保存时原样输出
}

/**
 * 解析后端 custom_rules 文本（每行一条规则）
 * - 空行跳过
 * - 第一列不在 RULE_TYPES 内的行当作 UNKNOWN 保留 raw 原文
 */
export function parseRules(text: string): Rule[] {
  if (!text) return []
  const rules: Rule[] = []
  for (const rawLine of text.split('\n')) {
    const line = rawLine.trim()
    if (!line) continue
    try {
      const parts = line.split(',').map(s => s.trim())
      const type = parts[0] as RuleType
      if (!(RULE_TYPES as readonly string[]).includes(type)) {
        rules.push({ type: 'UNKNOWN', value: '', target: '', noResolve: false, raw: line })
        continue
      }
      rules.push({
        type,
        value: parts[1] || '',
        target: parts[2] || '',
        noResolve: parts.some(p => p === 'no-resolve'),
      })
    } catch {
      rules.push({ type: 'UNKNOWN', value: '', target: '', noResolve: false, raw: line })
    }
  }
  return rules
}

/**
 * 序列化 rules 回文本（每行一条）
 * - UNKNOWN 行直接输出 raw
 * - value 或 target 为空的行跳过（视为未填完）
 * - noResolve 仅在 IP_RULE_TYPES 上生效
 */
export function serializeRules(rules: Rule[]): string {
  const lines: string[] = []
  for (const r of rules) {
    if (r.type === 'UNKNOWN') {
      if (r.raw) lines.push(r.raw)
      continue
    }
    if (!r.value || !r.target) continue
    let line = `${r.type},${r.value},${r.target}`
    if (r.noResolve && IP_RULE_TYPES.includes(r.type)) {
      line += ',no-resolve'
    }
    lines.push(line)
  }
  return lines.join('\n')
}
