# 共享技术设计(父任务)

各子任务的 `design.md` 只写自身差异;本文登记跨子任务的共享决策,子任务不得与其冲突。

## 1. 数据模型(GORM,model 层)

| 模型 | 表名 | 关键字段 | 归属 |
|------|------|----------|------|
| Provider | providers | ID、Name(唯一索引)、BaseURL、APIKey、CreatedAt、UpdatedAt | interface-config |
| TestRecord | test_records | ID、ProviderID、ProviderName、Protocol、Model、SystemPrompt、UserMessage、Temperature、MaxTokens、Stream、Reply、ErrMessage、FirstLatencyMs、TotalLatencyMs、PromptTokens、CompletionTokens、TotalTokens、CreatedAt | model-test |
| Setting | settings | Key(主键)、Value(文本) | newapi-rates 首用 |

- 模型列表不落库,实时透传上游。
- TestRecord 只保留最近 200 条:每次插入后按 CreatedAt 清理超限记录(含失败记录)。
- Setting 首用于 New API 配置(键 `newapi`,值为 JSON:base_url、token)。
- AutoMigrate 统一在 `cmd/server/main.go` 的 `mustOpenDB` 处登记(遵循 database-guidelines)。

## 2. API 路由总表(`/api` 前缀,统一 JSON 信封)

| 路由 | 方法 | 归属子任务 |
|------|------|-----------|
| /api/health | GET | 已有 |
| /api/providers | GET / POST | interface-config |
| /api/providers/:id | PUT / DELETE | interface-config |
| /api/providers/:id/models | GET | model-list |
| /api/test | POST(非流式) | model-test |
| /api/test/stream | POST(SSE 流式) | model-test |
| /api/test/records | GET / DELETE(清空) | model-test |
| /api/newapi/config | GET / PUT | newapi-rates |
| /api/newapi/rates | GET | newapi-rates |
| /api/newapi/estimate | POST(单次成本估算,未命中也返回 code=0) | newapi-rates |
| /api/agents | GET(列表与配置现状) | agent-config |
| /api/agents/:name | GET / PUT | agent-config |
| /api/agents/:name/restore | POST | agent-config |

流式例外:`/api/test/stream` 开始推送后按 `text/event-stream` 输出、不再套信封;开始推送前的失败仍走错误信封。

## 3. 错误码分段(codes.go 登记)

| 段 | 资源 | 示例 |
|----|------|------|
| 1xxx | 接口配置 / 模型列表 | 1401 参数缺失或 URL 非法;1402 名称重复;1404 配置不存在;1500 上游网络错误;1501 上游鉴权失败;1502 上游响应异常 |
| 2xxx | 模型测试 | 2401 参数缺失;2500 上游请求失败;2501 上游鉴权失败;2502 上游响应解析失败 |
| 3xxx | New API 费率 | 3401 配置不完整;3500 上游网络错误;3501 上游鉴权失败;3502 上游响应异常 |
| 4xxx | Agent 配置 | 4401 不支持的 Agent 名;4402 读写或备份失败;4404 配置文件不存在 |

细分与文案在各子任务 `design.md` 落实;错误消息一律中文,上游错误必须区分「网络错误」与「鉴权错误」(P0-2 硬性要求)。

## 4. 上游代理客户端(service 层,internal/service/llmclient)

- 统一 HTTP 客户端封装:超时控制、Header 组装、按根因归类错误(网络 / 鉴权 / 其他),service 其余部分不直接拼 URL、不直接处理 HTTP 状态归类。
- URL 拼接规则:BaseURL 去除尾部 `/`;拼接路径时若 BaseURL 已以 `/v1` 结尾则不再追加 `/v1` 前缀(适配中转站两种填写习惯)。
- 三种协议各自实现请求构造 / 全量解析 / 流式事件解析,对上暴露统一接口:
  - `ChatRequest`:Protocol、Model、System、User、Temperature、MaxTokens、Stream。
  - `Result`:Reply、Usage{Prompt, Completion, Total}、FirstLatencyMs、TotalLatencyMs。
  - 流式:`StreamChunk{Delta}`,回调式消费。
- 协议差异要点:
  - **OpenAI Chat Completions**:`POST {base}/v1/chat/completions`;`Authorization: Bearer`;流式请求带 `stream_options: {"include_usage": true}`(上游不支持时 usage 记为未返回);增量取 `choices[].delta.content`。
  - **OpenAI Responses**:`POST {base}/v1/responses`;全量取 `output_text` 或汇总 `output[]` 文本;usage 为 `input_tokens / output_tokens / total_tokens`;流式事件取 `response.output_text.delta`。
  - **Anthropic Messages**:`POST {base}/v1/messages`;header `x-api-key` + `anthropic-version: 2023-06-01`;system 为顶层字段、max_tokens 必填;usage 为 `input_tokens / output_tokens`;流式事件 `message_start`、`content_block_delta`(text)、`message_delta`(usage)。
- 首字延迟 = 发出请求到收到第一个内容增量;总耗时 = 到响应结束。
- API Key 只在客户端内部进 Header,禁止写入任何日志。

## 5. 前端结构

- 路由与菜单(全中文):`/providers` 接口配置、`/models` 模型列表、`/test` 模型测试、`/rates` 模型费率、`/agents` Agent 配置;`MainLayout.vue` 菜单随子任务递增。
- `api/` 按资源一文件;类型入 `types/`;页面放 `views/<资源>/`;跨页面共享的状态提升为 Pinia store(providers 列表在 model-list 落地 store)。
- SSE 接收用 `fetch` + `ReadableStream` 手动解析(EventSource 不支持 POST),封装为 composable `useTestStream`。
- 复用 `useRequest` 处理非流式请求的加载与错误提示。

## 6. 安全

- 脱敏格式:`前3位 + **** + 后4位`(长度不足 8 位时全部 `*`);列表与详情一律脱敏,任何接口不返回明文。
- 编辑语义:PUT/编辑请求中 key 字段为空表示沿用原值。
- 日志禁记 API Key、完整上游请求体(logging-guidelines)。
