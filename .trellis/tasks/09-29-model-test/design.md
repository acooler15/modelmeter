# 模型测试 — 技术设计

共享决策见父任务 design.md;本文写本任务落点。这是五个子任务中最复杂的,重点在三协议适配与 SSE 双层流转。

## 后端

### internal/model/test_record.go

```go
// TestRecord 模型测试记录:成功与失败都落库,保留最近 200 条。
type TestRecord struct {
    ID               uint      `gorm:"primaryKey" json:"id"`
    ProviderID       uint      `json:"provider_id"`
    ProviderName     string    `json:"provider_name"`
    Protocol         string    `json:"protocol"`      // chat_completions / responses / anthropic
    Model            string    `json:"model"`
    SystemPrompt     string    `json:"system_prompt"`
    UserMessage      string    `json:"user_message"`
    Temperature      *float64  `json:"temperature"`
    MaxTokens        *int      `json:"max_tokens"`
    Stream           bool      `json:"stream"`
    Succeeded        bool      `json:"succeeded"`
    Reply            string    `json:"reply"`
    ErrMessage       string    `json:"err_message"`
    FirstLatencyMs   int64     `json:"first_latency_ms"`
    TotalLatencyMs   int64     `json:"total_latency_ms"`
    PromptTokens     int       `json:"prompt_tokens"`
    CompletionTokens int       `json:"completion_tokens"`
    TotalTokens      int       `json:"total_tokens"`
    CreatedAt        time.Time `json:"created_at"`
}
```

- 显式 `TableName() = "test_records"`;APIKey 等凭据不落记录。main.go AutoMigrate 登记。

### internal/service/llmclient 扩展(在既有 GetJSON 基础上)

**protocol.go — 统一抽象**:

```go
type Protocol string // "chat_completions" | "responses" | "anthropic"

type ChatRequest struct {
    Protocol    Protocol
    Model       string
    System      string
    User        string
    Temperature *float64
    MaxTokens   *int
    Stream      bool
}

type Usage  struct{ Prompt, Completion, Total int }
type Result struct{ Reply string; Usage Usage; FirstLatencyMs, TotalLatencyMs int64 }

// DoChat 全量请求;StreamChat 流式请求,onDelta 收到每个内容增量。
// 两者都返回完整 Result(流式的 Reply 为增量拼接)。
func DoChat(ctx, cfg, req) (Result, error)
func StreamChat(ctx, cfg, req, onDelta func(string)) (Result, error)
```

**各协议差异**(实现要点,报文字段以此为准):

| | chat_completions | responses | anthropic |
|---|---|---|---|
| 路径 | /v1/chat/completions | /v1/responses | /v1/messages |
| 鉴权 | Bearer | Bearer | x-api-key + anthropic-version: 2023-06-01 |
| system | messages[0] role=system | 顶层 `instructions` | 顶层 `system` |
| user | messages[{role:user,content}] | `input:[{role:user,content}]` | messages[{role:user,content}] |
| max_tokens | max_tokens(可选) | max_output_tokens(可选) | max_tokens **必填**,未传补 1024 |
| 全量回复 | choices[0].message.content | 优先顶层 output_text;否则汇总 output[] 中 message 的 content[type=output_text].text | content[] 中 type=text 的 text 拼接 |
| usage | prompt/completion/total_tokens | input/output/total_tokens | input_tokens/output_tokens(Total=相加) |
| 流式增量 | choices[].delta.content | 事件 response.output_text.delta(data.delta) | content_block_delta(delta.type=text_delta → delta.text) |
| 流式 usage | stream_options.include_usage=true,末尾 chunk 的 usage(choices 为空数组) | response.completed 事件 response.usage | message_start(input)+message_delta(output) |
| 结束 | data: [DONE] | response.completed / response.incomplete | message_stop |

**sse.go — 通用 SSE 行解析器**(三协议复用):按行扫描 `event:` / `data:`,空行派发;多行 data 以 `\n` 拼接;提供 `Feed(line) ` 与事件回调,供 bufio.Scanner 驱动。注意跨网络包的行边界由 bufio 处理。

**错误码(codes.go 新增 2xxx)**:`CodeTestInvalid=2401`(协议非法/模型/user 为空)、`CodeUpstreamRequestFailed=2500`(非 2xx,含状态码)、`CodeUpstreamParseFailed=2502`(响应体不符合协议结构)。网络/鉴权沿用 1500/1501。ctx 主动取消返回原始错误不归类。

### internal/service/modeltest.go

- `TestInput{ProviderID uint; llmclient.ChatRequest}`;validate:ProviderID>0、Model/User 非空、Protocol 合法 → 2401 中文提示。
- `RunTest(ctx, db, in) (llmclient.Result, *model.TestRecord, error)`:查 provider(1404)→ DoChat → **无论成败都落 TestRecord**(失败 Succeeded=false、ErrMessage=中文文案)→ 清理超限 → 返回。
- `StreamTest(ctx, db, in, onDelta func(string)) (llmclient.Result, *model.TestRecord, error)`:同上,Stream=true;错误发生在推送过程中时同样落失败记录。
- 清理:`DELETE ... WHERE id NOT IN (SELECT id ... ORDER BY created_at DESC, id DESC LIMIT 200)`,失败不影响主流程(记日志)。

### handler/modeltest.go + router

- `POST /api/test`:非流式,信封 `data:{result, record}`。
- `POST /api/test/stream`:参数校验失败/配置不存在 → 走信封错误(4xx);校验通过后设 `text/event-stream`,事件统一 `data: {"type":"delta","text":...}` / `{"type":"done","result":{...},"record":{...}}` / `{"type":"error","message":...}`,每事件 Flush;循环内每次 onDelta 转发。handler 不落业务逻辑,StreamTest 回调直接转发。
- `GET /api/test/records?limit=100`:created_at 倒序。
- `DELETE /api/test/records`:清空,返回 `{deleted:n}`。

### 测试

- `llmclient/protocol_test.go`:三协议 × (全量解析+usage / 流式拼接+usage+首字延迟) httptest 假上游;错误分类 401→1501、500→2500、坏结构→2502;anthropic 未传 max_tokens 补 1024。
- `modeltest_test.go`:成功/失败落库、只保留 200 条、records 清空。

## 前端

- `types/test.ts`:Protocol、TestRequest、TestResult、Usage、TestRecord(与后端 json tag 一致)。
- `api/modelTest.ts`:`runTest / fetchRecords / clearRecords`(非流式走 http 封装)。
- `composables/useTestStream.ts`:`fetch POST /api/test/stream` + AbortController;ReadableStream + TextDecoder 手动缓冲分帧,按 `data: {...}\n\n` 切事件;`onDelta` 逐字回调,done 返回 result/record,error 抛中文 message;开始前响应非 200/非 event-stream → 解析信封报错;`abort()` 支持中断。
- `views/test/ModelTestView.vue`(单页上下两区):
  - 表单区:配置选择(用 providers store)、模型 ID(手输 + 「从配置拉取」按钮填充下拉)、协议 el-radio-group(三项中文标签)、system/user 文本域、temperature/max_tokens 数字输入(可空)、流式 el-switch、「发送测试」;发送中禁用表单。
  - 结果区:回复文本、指标行(首字延迟/总耗时/tokens 三项,未返回显示「未返回」)、成本行占位(费率任务接入)。
  - 记录区:表格(时间/配置/模型/协议/状态/耗时/tokens)+ 「清空记录」(二次确认)+ 行点击详情对话框(完整回复与参数)。
- 路由 `/test` + 菜单「模型测试」。

## 兼容与回滚

- 新增为主;codes.go/router/main.go 挂载点小改;llmclient 新增文件不改动既有 GetJSON 行为(其单测保证)。
