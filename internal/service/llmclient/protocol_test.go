package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/acooler15/modelmeter/internal/apperr"
)

const testAPIKey = "sk-test-key-1234"

// readBody 读全假上游收到的请求体。
func readBody(r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	return body
}

// writeSSE 向响应写入一条 SSE 事件并立即 Flush,模拟上游逐块推送。
func writeSSE(t *testing.T, w http.ResponseWriter, event, data string) {
	t.Helper()
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// decodeBody 把假上游收到的请求体解析为 map,便于逐字段断言。
func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("假上游收到的请求体不是合法 JSON: %v", err)
	}
	return m
}

// TestDoChat_ChatCompletions_全量解析 覆盖路径、Bearer 鉴权、system/user
// 报文位置、可选参数缺省不发送、回复与 usage 解析。
func TestDoChat_ChatCompletions_全量解析(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody = decodeBody(t, readBody(r))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"你好,世界"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer srv.Close()

	temp := 0.7
	res, err := DoChat(context.Background(), UpstreamConfig{BaseURL: srv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolChatCompletions, Model: "gpt-test", System: "你是助手", User: "打个招呼", Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("DoChat 应成功,实际报错: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("期望路径 /v1/chat/completions,实际 %q", gotPath)
	}
	if gotAuth != "Bearer "+testAPIKey {
		t.Errorf("期望 Bearer 鉴权,实际 %q", gotAuth)
	}
	messages, _ := gotBody["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("期望 system+user 两条消息,实际 %v", gotBody["messages"])
	}
	first, _ := messages[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "你是助手" {
		t.Errorf("messages[0] 应为 system 消息,实际 %v", first)
	}
	if gotBody["temperature"] != 0.7 {
		t.Errorf("temperature 应随请求发送,实际 %v", gotBody["temperature"])
	}
	if _, ok := gotBody["max_tokens"]; ok {
		t.Errorf("max_tokens 未传时不应发送")
	}
	if _, ok := gotBody["stream_options"]; ok {
		t.Errorf("非流式请求不应携带 stream_options")
	}
	if res.Reply != "你好,世界" {
		t.Errorf("回复解析错误,实际 %q", res.Reply)
	}
	if res.Usage.Prompt != 10 || res.Usage.Completion != 5 || res.Usage.Total != 15 {
		t.Errorf("usage 解析错误,实际 %+v", res.Usage)
	}
	if res.FirstLatencyMs < 0 || res.TotalLatencyMs < res.FirstLatencyMs {
		t.Errorf("计时不合理: first=%d total=%d", res.FirstLatencyMs, res.TotalLatencyMs)
	}
}

// TestDoChat_Responses_全量解析 覆盖顶层 output_text 优先与 output[] 汇总
// 两条解析路径,以及 instructions/max_output_tokens 的报文位置。
func TestDoChat_Responses_全量解析(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("期望路径 /v1/responses,实际 %q", r.URL.Path)
		}
		gotBody = decodeBody(t, readBody(r))
		_, _ = w.Write([]byte(`{"output_text":"聚合回复","usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11}}`))
	}))
	defer srv.Close()

	maxTokens := 256
	res, err := DoChat(context.Background(), UpstreamConfig{BaseURL: srv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolResponses, Model: "gpt-test", System: "系统指令", User: "问题", MaxTokens: &maxTokens,
	})
	if err != nil {
		t.Fatalf("DoChat 应成功,实际报错: %v", err)
	}
	if gotBody["instructions"] != "系统指令" {
		t.Errorf("system 应落在顶层 instructions,实际 %v", gotBody["instructions"])
	}
	if gotBody["max_output_tokens"] != float64(256) {
		t.Errorf("max_tokens 应映射为 max_output_tokens,实际 %v", gotBody["max_output_tokens"])
	}
	input, _ := gotBody["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("期望 input 仅一条 user 消息,实际 %v", gotBody["input"])
	}
	if res.Reply != "聚合回复" {
		t.Errorf("应优先取顶层 output_text,实际 %q", res.Reply)
	}
	if res.Usage.Prompt != 8 || res.Usage.Completion != 3 || res.Usage.Total != 11 {
		t.Errorf("usage 解析错误,实际 %+v", res.Usage)
	}

	// 无 output_text 时汇总 output[] 中 output_text 文本块
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"第一段"},{"type":"output_text","text":"第二段"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer fallbackSrv.Close()
	res, err = DoChat(context.Background(), UpstreamConfig{BaseURL: fallbackSrv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolResponses, Model: "gpt-test", User: "问题",
	})
	if err != nil {
		t.Fatalf("DoChat 应成功,实际报错: %v", err)
	}
	if res.Reply != "第一段第二段" {
		t.Errorf("output[] 汇总解析错误,实际 %q", res.Reply)
	}
}

// TestDoChat_Anthropic_全量解析与默认MaxTokens 覆盖 x-api-key/版本头、
// 顶层 system、max_tokens 必填补 1024、content[] 拼接与 usage 相加。
func TestDoChat_Anthropic_全量解析与默认MaxTokens(t *testing.T) {
	var gotPath, gotAPIKey, gotVersion string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotBody = decodeBody(t, readBody(r))
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"第一段"},{"type":"text","text":"第二段"}],"usage":{"input_tokens":6,"output_tokens":4}}`))
	}))
	defer srv.Close()

	res, err := DoChat(context.Background(), UpstreamConfig{BaseURL: srv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolAnthropic, Model: "claude-test", System: "系统提示", User: "问题",
	})
	if err != nil {
		t.Fatalf("DoChat 应成功,实际报错: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("期望路径 /v1/messages,实际 %q", gotPath)
	}
	if gotAPIKey != testAPIKey {
		t.Errorf("期望 x-api-key 鉴权,实际 %q", gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Errorf("期望 anthropic-version 2023-06-01,实际 %q", gotVersion)
	}
	if gotBody["system"] != "系统提示" {
		t.Errorf("system 应为顶层字段,实际 %v", gotBody["system"])
	}
	if gotBody["max_tokens"] != float64(defaultAnthropicMaxTokens) {
		t.Errorf("max_tokens 未传时应补默认值 1024,实际 %v", gotBody["max_tokens"])
	}
	if res.Reply != "第一段第二段" {
		t.Errorf("content[] 拼接错误,实际 %q", res.Reply)
	}
	if res.Usage.Prompt != 6 || res.Usage.Completion != 4 || res.Usage.Total != 10 {
		t.Errorf("anthropic usage Total 应为 input+output,实际 %+v", res.Usage)
	}
}

// TestStreamChat_ChatCompletions_流式拼接 覆盖增量转发顺序、回复拼接、
// 末尾 chunk 用量与 stream_options.include_usage 请求参数。
func TestStreamChat_ChatCompletions_流式拼接(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, readBody(r))
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "", `{"choices":[{"delta":{"content":"你"}}]}`)
		writeSSE(t, w, "", `{"choices":[{"delta":{"content":"好"}}]}`)
		writeSSE(t, w, "", `{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`)
		writeSSE(t, w, "", "[DONE]")
	}))
	defer srv.Close()

	var deltas []string
	res, err := StreamChat(context.Background(), UpstreamConfig{BaseURL: srv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolChatCompletions, Model: "gpt-test", User: "打个招呼",
	}, func(text string) { deltas = append(deltas, text) })
	if err != nil {
		t.Fatalf("StreamChat 应成功,实际报错: %v", err)
	}
	if gotBody["stream"] != true {
		t.Errorf("流式请求应携带 stream:true")
	}
	opts, ok := gotBody["stream_options"].(map[string]any)
	if !ok || opts["include_usage"] != true {
		t.Errorf("流式请求应携带 stream_options.include_usage=true,实际 %v", gotBody["stream_options"])
	}
	if strings.Join(deltas, "") != "你好" || len(deltas) != 2 {
		t.Errorf("增量回调顺序/内容错误,实际 %v", deltas)
	}
	if res.Reply != "你好" {
		t.Errorf("流式回复应为增量拼接,实际 %q", res.Reply)
	}
	if res.Usage.Prompt != 5 || res.Usage.Completion != 2 || res.Usage.Total != 7 {
		t.Errorf("usage 解析错误,实际 %+v", res.Usage)
	}
	if res.FirstLatencyMs < 0 || res.TotalLatencyMs < res.FirstLatencyMs {
		t.Errorf("计时不合理: first=%d total=%d", res.FirstLatencyMs, res.TotalLatencyMs)
	}
}

// TestStreamChat_Responses_流式拼接 覆盖 event 名与 data.type 双通道识别、
// response.completed 携带的 usage 与结束语义。
func TestStreamChat_Responses_流式拼接(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("期望路径 /v1/responses,实际 %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// 完整事件:带 event: 行
		writeSSE(t, w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"你"}`)
		// 省略 event: 行,靠 data.type 兜底识别
		writeSSE(t, w, "", `{"type":"response.output_text.delta","delta":"好"}`)
		writeSSE(t, w, "response.completed", `{"response":{"usage":{"input_tokens":9,"output_tokens":2,"total_tokens":11}}}`)
	}))
	defer srv.Close()

	var deltas []string
	res, err := StreamChat(context.Background(), UpstreamConfig{BaseURL: srv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolResponses, Model: "gpt-test", User: "问题",
	}, func(text string) { deltas = append(deltas, text) })
	if err != nil {
		t.Fatalf("StreamChat 应成功,实际报错: %v", err)
	}
	if strings.Join(deltas, "") != "你好" {
		t.Errorf("增量解析错误,实际 %v", deltas)
	}
	if res.Reply != "你好" {
		t.Errorf("流式回复应为增量拼接,实际 %q", res.Reply)
	}
	if res.Usage.Prompt != 9 || res.Usage.Completion != 2 || res.Usage.Total != 11 {
		t.Errorf("usage 应取自 response.completed,实际 %+v", res.Usage)
	}
}

// TestStreamChat_Anthropic_流式拼接 覆盖 message_start/content_block_delta/
// message_delta/message_stop 四类事件与 usage 两处相加。
func TestStreamChat_Anthropic_流式拼接(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("期望路径 /v1/messages,实际 %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "message_start", `{"type":"message_start","message":{"usage":{"input_tokens":5}}}`)
		writeSSE(t, w, "content_block_delta", `{"type":"content_block_delta","delta":{"type":"text_delta","text":"你"}}`)
		writeSSE(t, w, "content_block_delta", `{"type":"content_block_delta","delta":{"type":"text_delta","text":"好"}}`)
		writeSSE(t, w, "message_delta", `{"type":"message_delta","usage":{"output_tokens":2}}`)
		writeSSE(t, w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	var deltas []string
	res, err := StreamChat(context.Background(), UpstreamConfig{BaseURL: srv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolAnthropic, Model: "claude-test", User: "问题",
	}, func(text string) { deltas = append(deltas, text) })
	if err != nil {
		t.Fatalf("StreamChat 应成功,实际报错: %v", err)
	}
	if strings.Join(deltas, "") != "你好" {
		t.Errorf("增量解析错误,实际 %v", deltas)
	}
	if res.Usage.Prompt != 5 || res.Usage.Completion != 2 || res.Usage.Total != 7 {
		t.Errorf("usage 应为 message_start(input)+message_delta(output) 相加,实际 %+v", res.Usage)
	}
}

// TestSSEParser_分帧规则 直接验证共享解析器:多行 data 拼接、注释与
// 无关字段忽略、字段值前导空格处理。
func TestSSEParser_分帧规则(t *testing.T) {
	var events []SSEEvent
	p := NewSSEParser(func(ev SSEEvent) { events = append(events, ev) })
	for _, line := range []string{
		": heartbeat", // 注释行,忽略
		"event: response.output_text.delta",
		"data: {\"a\":1}",
		"data: {\"b\":2}", // 同一事件的多行 data
		"",                // 空行派发第一个事件
		"event: response.completed",
		"data:{\"c\":3}", // 无空格写法
		"id: 42",         // 无关字段,忽略
		"",               // 空行派发第二个事件
	} {
		p.Feed(line)
	}
	p.Finish()
	if len(events) != 2 {
		t.Fatalf("期望派发 2 个事件,实际 %d 个: %+v", len(events), events)
	}
	if events[0].Event != "response.output_text.delta" {
		t.Errorf("事件名解析错误,实际 %q", events[0].Event)
	}
	if events[0].Data != "{\"a\":1}\n{\"b\":2}" {
		t.Errorf("多行 data 应以换行拼接,实际 %q", events[0].Data)
	}
	if events[1].Data != "{\"c\":3}" {
		t.Errorf("无空格 data 解析错误,实际 %q", events[1].Data)
	}
}

// TestDoChat_错误归类 覆盖 401→1501、500→2500、坏结构→2502、拒连→1500,
// 并确保错误消息不泄露 API Key。
func TestDoChat_错误归类(t *testing.T) {
	ctx := context.Background()

	// 401 → 1501,文案与模型列表一致
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer authSrv.Close()
	_, err := DoChat(ctx, UpstreamConfig{BaseURL: authSrv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolChatCompletions, Model: "m", User: "hi",
	})
	wantCode(t, err, apperr.CodeUpstreamAuth)
	if !strings.Contains(err.Error(), "鉴权失败") {
		t.Errorf("期望鉴权中文文案,实际 %v", err)
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("错误消息泄露 API Key: %v", err)
	}

	// 500 → 2500,消息含状态码
	serverErrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer serverErrSrv.Close()
	_, err = DoChat(ctx, UpstreamConfig{BaseURL: serverErrSrv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolResponses, Model: "m", User: "hi",
	})
	wantCode(t, err, apperr.CodeUpstreamRequestFailed)
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("期望消息包含上游状态码 500,实际 %v", err)
	}

	// 200 但结构非法 → 2502(三种协议同一归类逻辑,抽一种验证)
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"foo":"bar"}`))
	}))
	defer badSrv.Close()
	for _, p := range []Protocol{ProtocolChatCompletions, ProtocolResponses, ProtocolAnthropic} {
		_, err := DoChat(ctx, UpstreamConfig{BaseURL: badSrv.URL, APIKey: testAPIKey}, ChatRequest{Protocol: p, Model: "m", User: "hi"})
		wantCode(t, err, apperr.CodeUpstreamParseFailed)
	}

	// 拒连 → 1500:复用一个已关闭的假上游地址
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	_, err = DoChat(ctx, UpstreamConfig{BaseURL: closedURL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolAnthropic, Model: "m", User: "hi",
	})
	wantCode(t, err, apperr.CodeUpstreamNetwork)
}

// TestStreamChat_错误归类 覆盖流式场景:401、坏事件结构与流中错误事件。
func TestStreamChat_错误归类(t *testing.T) {
	ctx := context.Background()

	// 401 → 1501(推送未开始即失败)
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer authSrv.Close()
	_, err := StreamChat(ctx, UpstreamConfig{BaseURL: authSrv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolChatCompletions, Model: "m", User: "hi",
	}, func(string) {})
	wantCode(t, err, apperr.CodeUpstreamAuth)

	// 200 但事件数据不是合法 JSON → 2502
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "", `not-json`)
	}))
	defer badSrv.Close()
	_, err = StreamChat(ctx, UpstreamConfig{BaseURL: badSrv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolChatCompletions, Model: "m", User: "hi",
	}, func(string) {})
	wantCode(t, err, apperr.CodeUpstreamParseFailed)

	// 2xx 但 Content-Type 不是 event-stream → 2502
	jsonSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer jsonSrv.Close()
	_, err = StreamChat(ctx, UpstreamConfig{BaseURL: jsonSrv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolChatCompletions, Model: "m", User: "hi",
	}, func(string) {})
	wantCode(t, err, apperr.CodeUpstreamParseFailed)

	// 流中错误事件 → 2500,消息带上游错误文案
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "error", `{"type":"error","error":{"message":"配额不足"}}`)
	}))
	defer errSrv.Close()
	_, err = StreamChat(ctx, UpstreamConfig{BaseURL: errSrv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolAnthropic, Model: "m", User: "hi",
	}, func(string) {})
	wantCode(t, err, apperr.CodeUpstreamRequestFailed)
	if !strings.Contains(err.Error(), "配额不足") {
		t.Errorf("期望消息带上游错误文案,实际 %v", err)
	}
}

// TestStreamChat_主动取消返回原始错误 ctx 已取消时不做错误归类,
// 错误应保持 context.Canceled 可被 errors.Is 识别。
func TestStreamChat_主动取消返回原始错误(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "", `{"choices":[{"delta":{"content":"你"}}]}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 发起前即取消
	_, err := StreamChat(ctx, UpstreamConfig{BaseURL: srv.URL, APIKey: testAPIKey}, ChatRequest{
		Protocol: ProtocolChatCompletions, Model: "m", User: "hi",
	}, func(string) {})
	if err == nil {
		t.Fatal("ctx 已取消,期望返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望原始 context.Canceled,实际 %v", err)
	}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		t.Errorf("主动取消的错误不应被归类为业务错误,实际 %+v", ae)
	}
}
