// 模型列表领域类型,与后端 ModelInfo 的 json tag 一一对应。

/** 上游 /v1/models 返回的单个模型条目;仅 id 由后端保证非空。 */
export interface ModelInfo {
  id: string
  object: string
  owned_by: string
  created: number // Unix 秒级时间戳;上游未提供时为 0
  raw: unknown // 上游原始条目透传,结构不完全可信,展示时统一走 JSON 序列化
}
