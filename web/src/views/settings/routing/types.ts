export interface Category {
  ID: number
  Code: string
  DisplayName: string
  Kind: 'system' | 'custom'
  SiteTags: string[]
  IPTags: string[]
  InlineDomainSuffix: string[]
  InlineDomainKeyword: string[]
  InlineIPCIDR: string[]
  Protocol: string
  DefaultGroupID: number | null
  Enabled: boolean
  SortOrder: number
}

export interface Group {
  ID: number
  Code: string
  DisplayName: string
  Type: 'selector' | 'urltest'
  Members: string[]
  Kind: 'system' | 'custom'
  SortOrder: number
}

export interface CustomRule {
  ID: number
  Name: string
  SiteTags: string[]
  IPTags: string[]
  DomainSuffix: string[]
  DomainKeyword: string[]
  IPCIDR: string[]
  SrcIPCIDR: string[]
  Protocol: string
  Port: string
  OutboundGroupID: number | null
  OutboundLiteral: string
  SortOrder: number
}

export interface Preset {
  Code: string
  DisplayName: string
  EnabledCategories: string[]
}

export interface RoutingConfig {
  categories: Category[]
  groups: Group[]
  customRules: CustomRule[]
  presets: Preset[]
  settings: Record<string, string>
}
