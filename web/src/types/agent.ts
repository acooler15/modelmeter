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
  /** 列的渲染形态:text 文本、select 下拉、number 数字、bool 开关、list 多选(可自由添加)。 */
  type: 'text' | 'select' | 'number' | 'bool' | 'list'
  /** select 为单选下拉选项;list 为多选候选项(空则由用户自由输入)。 */
  options?: AgentFieldOption[]
  required: boolean
  /** 字段说明,界面展示。 */
  help?: string
  /** 只读列仅展示,不接受提交。 */
  readonly?: boolean
}

/** "默认模型"现状:对应 ZCode config.defaultModelSelection。 */
export interface AgentDefaultModel {
  provider_id: string
  model_id: string
  /** 推理档位,未设置时缺省。 */
  reasoning_level?: string
}

/** 默认模型修改:provider_id+model_id 均空=清除,均非空=设置,混合被后端以 4403 拒绝。 */
export interface AgentDefaultModelPatch {
  provider_id: string
  model_id: string
  /** 仅设置时有效,缺省=不写 options。 */
  reasoning_level?: string
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
  /** 能力位:该工具是否有"默认模型"概念(能力是工具属性,未找到配置时同样成立)。 */
  supports_default_model: boolean
  /** 当前默认模型;缺省/null=未设置或不支持该能力。 */
  default_model?: AgentDefaultModel | null
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
