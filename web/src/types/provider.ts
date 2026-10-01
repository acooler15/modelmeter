// 接口配置领域类型,与后端 ProviderView / ProviderInput 的 json tag 一一对应。
// 接口配置视图按产品决策返回明文 api_key(本地单用户工具,用户需随时查看
// 自己录入的凭据);New API 令牌(token_masked)与 Agent 模型清单凭据仍脱敏。

/** 接口配置视图:api_key 为完整明文,属于"对外脱敏"规范的有意例外。 */
export interface ProviderView {
  id: number
  name: string
  base_url: string
  api_key: string
  created_at: string // 后端时间为 UTC ISO 字符串
  updated_at: string
}

/** 新增/编辑接口配置的输入;编辑时 api_key 留空表示沿用原值。 */
export interface ProviderInput {
  name: string
  base_url: string
  api_key: string
}
