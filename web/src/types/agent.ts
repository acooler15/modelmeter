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
  /** 能力位:该工具是否支持从接口添加模型条目(工具属性,未找到配置时同样成立)。 */
  supports_add_models: boolean
  /** 能力位:该工具是否支持删除模型条目(同上)。 */
  supports_remove_models: boolean
  /** 添加模型的可挂靠供应商候选(非敏感字段);仅 ZCode 且配置 found 时填充。 */
  add_targets?: AgentAddTarget[]
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

/** 删除目标定位:provider_id 仅 ZCode 有,WorkBuddy/CodeBuddy 为空。 */
export interface AgentModelRef {
  provider_id?: string
  model_id: string
}

/** ZCode 添加模型的可挂靠供应商候选(非敏感字段,随 Snapshot 下发)。 */
export interface AgentAddTarget {
  provider_id: string
  provider_name?: string
}

/** ZCode 落点规格;WorkBuddy/CodeBuddy 整体忽略(前端直接省略该字段)。 */
export interface AgentAddTargetSpec {
  /** 落点模式,缺省按 existing 处理。 */
  mode?: 'existing' | 'new'
  /** mode=existing 时必填:挂靠的供应商 ID。 */
  provider_id?: string
  /** mode=new 的供应商显示名,空回退接口名。 */
  provider_name?: string
  /** mode=new 的 API 协议,空缺省 openai-chat-completions。 */
  api_type?: 'openai-chat-completions' | 'openai-responses'
}

/** 从接口添加模型的请求体;凭据不在其中,由后端从接口记录装配。 */
export interface AgentAddModelsPayload {
  provider_id: number
  model_ids: string[]
  /** 接口地址覆盖值;空表示沿用接口记录的 Base URL。 */
  base_url?: string
  /** 仅 ZCode 需要;其余工具省略。 */
  target?: AgentAddTargetSpec
}

/** 添加结果:最新清单 + 逐项成败,前端据此反馈"成功 N/跳过 M"。 */
export interface AgentAddModelsResult {
  entries: AgentModelEntry[]
  added: string[]
  skipped: string[]
}
