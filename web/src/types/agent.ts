// Agent 模型配置领域类型,与后端 internal/service/agentconf 的 json tag 逐字对应。

/** select 字段的一个选项:value 为写回值,label 为展示名。 */
export interface AgentFieldOption {
  value: string
  label: string
}

/** 字段(列)描述:既描述模型表格的"列",也承载该列的可编辑性。 */
export interface AgentFieldSpec {
  key: string
  label: string
  /** 列的渲染形态:text 文本、select 下拉、number 数字、bool 开关。 */
  type: 'text' | 'select' | 'number' | 'bool'
  /** 仅 select 使用:下拉选项。 */
  options?: AgentFieldOption[]
  required: boolean
  /** 字段说明,界面展示。 */
  help?: string
  /** 只读列仅展示,不接受提交。 */
  readonly?: boolean
}

/** Agent 配置现状视图;后端保证不回传任何凭据字段。 */
export interface AgentSnapshot {
  name: string
  display_name: string
  status: 'found' | 'not_found'
  config_path: string
  /** 模型表格的列描述。 */
  columns: AgentFieldSpec[]
  /** 指引或管理边界说明。 */
  message?: string
}

/** 模型清单中的一个模型条目(凭据绝不包含)。 */
export interface AgentModelEntry {
  /** ZCode 有供应商层;WorkBuddy 为空。 */
  provider_id?: string
  provider_name?: string
  model_id: string
  display_name?: string
  /** 非凭据字段:展示 + 可编辑,键集为 columns 的 key 子集。 */
  fields: Record<string, unknown>
}

/** 对一个模型条目的白名单局部修改。 */
export interface AgentModelPatch {
  /** ZCode 定位键;WorkBuddy 为空。 */
  provider_id?: string
  model_id: string
  /** 仅白名单键;含白名单外键会被后端以 4403 拒绝。 */
  fields: Record<string, unknown>
}
