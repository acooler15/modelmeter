// New API 模型费率领域类型,与后端 internal/service/newapi.go 的 json tag 一一对应。

/** New API 配置视图:token_masked 为脱敏值,后端永不返回明文令牌。 */
export interface NewAPIConfigView {
  configured: boolean // 是否已保存过配置
  base_url: string
  token_masked: string
}

/** 保存 New API 配置的输入;token 留空表示沿用原令牌。 */
export interface NewAPIConfigInput {
  base_url: string
  token: string
}

/** 单个模型的费率条目;上游缺字段时后端按零值降级。 */
export interface RateEntry {
  model_name: string
  quota_type: number // 0=倍率计费,1=按次计费
  model_ratio: number
  completion_ratio: number
  model_price: number
}

/** 单次调用成本估算结果;formula 为后端给出的中文口径说明。 */
export interface CostEstimate {
  model: string
  quota_type: number
  usd: number
  quota: number
  formula: string
}

/** 成本估算响应:available=false 表示未配置/未命中/失败,estimate 为 null。 */
export interface EstimateResult {
  available: boolean
  estimate: CostEstimate | null
}
