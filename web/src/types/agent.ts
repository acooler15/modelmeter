// Agent 模型配置领域类型,与后端 internal/service/agentconf 的 json tag 一一对应。

/** select 字段的一个选项:value 为写回值,label 为展示名。 */
export interface AgentFieldOption {
  value: string
  label: string
}

/** 字段描述,驱动修改配置对话框的动态表单。 */
export interface AgentFieldSpec {
  key: string
  label: string
  type: 'text' | 'select'
  options?: AgentFieldOption[]
  required: boolean
  help?: string
}

/** Agent 配置现状视图;后端保证不回传任何凭据字段。 */
export interface AgentSnapshot {
  name: string
  display_name: string
  status: 'found' | 'not_found'
  config_path: string
  fields: AgentFieldSpec[]
  values: Record<string, string>
  message?: string
}
