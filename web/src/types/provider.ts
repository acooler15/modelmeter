// 接口配置领域类型,与后端 ProviderView / ProviderInput 的 json tag 一一对应。
// 后端永不输出明文 Key,这里只有脱敏字段。

/** 接口配置视图:api_key_masked 为脱敏值(如 sk-****klmn)。 */
export interface ProviderView {
  id: number
  name: string
  base_url: string
  api_key_masked: string
  created_at: string // 后端时间为 UTC ISO 字符串
  updated_at: string
}

/** 新增/编辑接口配置的输入;编辑时 api_key 留空表示沿用原值。 */
export interface ProviderInput {
  name: string
  base_url: string
  api_key: string
}
