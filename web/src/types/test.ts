// 模型测试领域类型,与后端 json tag 一一对应(见 internal/service/modeltest.go
// 与 internal/service/llmclient/protocol.go)。

/** 对话协议标识;三种协议的报文差异由后端处理,前端仅作单选。 */
export type TestProtocol = 'chat_completions' | 'responses' | 'anthropic'

/** 测试请求参数;temperature/max_tokens 为 null 表示留空(不随请求发送)。 */
export interface TestRequest {
  provider_id: number
  protocol: TestProtocol
  model: string
  system: string
  user: string
  temperature: number | null
  max_tokens: number | null
  stream: boolean
}

/** token 用量;上游未返回时后端各字段为 0,展示层显示「未返回」。 */
export interface TestUsage {
  prompt: number
  completion: number
  total: number
}

/** 一次测试的统一结果;流式时 reply 为全部增量拼接。 */
export interface TestResult {
  reply: string
  usage: TestUsage
  first_latency_ms: number
  total_latency_ms: number
}

/** 测试记录:成功与失败都落库,后端只保留最近 200 条。 */
export interface TestRecord {
  id: number
  provider_id: number
  provider_name: string
  protocol: TestProtocol
  model: string
  system_prompt: string
  user_message: string
  temperature: number | null
  max_tokens: number | null
  stream: boolean
  succeeded: boolean
  reply: string
  err_message: string
  first_latency_ms: number
  total_latency_ms: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  created_at: string
}

/** 流式 SSE 事件:后端 data: {...} 帧反序列化后的联合类型。 */
export type TestStreamEvent =
  | { type: 'delta'; text: string }
  | { type: 'done'; result: TestResult; record: TestRecord }
  | { type: 'error'; message: string }
