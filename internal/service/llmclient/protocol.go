// protocol.go 在 client.go 的 URL 拼接与错误归类基座上实现三种对话协议
// (OpenAI Chat Completions / OpenAI Responses / Anthropic Messages)的
// 请求构造、全量解析与流式事件解析,对上暴露统一的 DoChat / StreamChat。
// 各协议报文差异逐字段对应任务 design.md 的协议差异表;API Key 只在
// applyAuth 中进入请求头,禁止写入任何日志。
package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// Protocol 上游对话协议标识。
type Protocol string

const (
	ProtocolChatCompletions Protocol = "chat_completions" // OpenAI Chat Completions
	ProtocolResponses       Protocol = "responses"        // OpenAI Responses
	ProtocolAnthropic       Protocol = "anthropic"        // Anthropic Messages
)

// defaultAnthropicMaxTokens anthropic 协议 max_tokens 必填,用户未填时补的默认值。
const defaultAnthropicMaxTokens = 1024

// Valid 判断协议取值是否为受支持的三种之一。
func (p Protocol) Valid() bool {
	switch p {
	case ProtocolChatCompletions, ProtocolResponses, ProtocolAnthropic:
		return true
	default:
		return false
	}
}

// path 返回协议的上游请求路径;以 /v1/ 开头,/v1 重复拼接由 BuildURL 处理。
func (p Protocol) path() string {
	switch p {
	case ProtocolResponses:
		return "/v1/responses"
	case ProtocolAnthropic:
		return "/v1/messages"
	default:
		return "/v1/chat/completions"
	}
}

// ChatRequest 对话测试请求参数。json 标签与前端请求体字段一致,供 service
// 层直接绑定。Temperature/MaxTokens 为 nil 表示不随请求发送,用上游默认值。
type ChatRequest struct {
	Protocol    Protocol `json:"protocol"`
	Model       string   `json:"model"`
	System      string   `json:"system"`
	User        string   `json:"user"`
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
	Stream      bool     `json:"stream"`
}

// Usage token 用量;上游未返回时各字段为 0,由展示层显示「未返回」。
type Usage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// Result 一次对话测试的统一结果;流式时 Reply 为全部内容增量拼接而成。
type Result struct {
	Reply          string `json:"reply"`
	Usage          Usage  `json:"usage"`
	FirstLatencyMs int64  `json:"first_latency_ms"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
}

// chatClient 全量对话专用客户端:生成式回复耗时可能远超模型列表类请求,
// 整体超时放宽到 120 秒。独立于共享 httpClient,GetJSON 行为保持不变。
var chatClient = &http.Client{Timeout: 120 * time.Second}

// streamClient 流式对话专用客户端:不设整体超时,流式生命周期由请求的
// context 控制(用户中断、服务端关闭都会取消 ctx)。
var streamClient = &http.Client{}

// chatMessage OpenAI 系协议(含 anthropic)的对话消息条目。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DoChat 发起一次非流式对话请求并解析完整回复。
// 首字延迟:非流式下首个内容随完整响应体一起到达,取响应体读完的时刻。
func DoChat(ctx context.Context, cfg UpstreamConfig, req ChatRequest) (Result, error) {
	req.Stream = false
	start := time.Now()
	resp, err := sendChatRequest(ctx, cfg, req)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		// 读取失败时也需要关闭连接;关闭错误不影响业务结果,可安全忽略
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, err // 主动取消返回原始错误,不归类
		}
		return Result{}, apperr.Wrap(apperr.CodeUpstreamNetwork, "读取上游响应失败:"+describeNetErr(err), err)
	}
	reply, usage, err := parseFullReply(req.Protocol, body)
	return Result{
		Reply:          reply,
		Usage:          usage,
		FirstLatencyMs: time.Since(start).Milliseconds(),
		TotalLatencyMs: time.Since(start).Milliseconds(),
	}, err
}

// StreamChat 发起流式对话请求,onDelta 在收到每个内容增量时被同步调用。
// 返回的 Result 含拼接后的回复、token 用量与计时;首字延迟 = 收到第一个
// 内容增量的时刻,总耗时 = 流结束时刻(均从请求发出起算,单位毫秒)。
func StreamChat(ctx context.Context, cfg UpstreamConfig, req ChatRequest, onDelta func(string)) (Result, error) {
	req.Stream = true
	start := time.Now()
	resp, err := sendChatRequest(ctx, cfg, req)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 2xx 但 Content-Type 明确不是 event-stream:上游忽略了 stream 参数,
	// 返回的是完整 JSON 而非事件流,按解析失败处理(文案与 2502 对齐)
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "text/event-stream") {
		return Result{}, apperr.New(apperr.CodeUpstreamParseFailed, "上游响应解析失败,流式响应格式不符合预期")
	}

	var res Result
	firstSeen := false
	emit := func(text string) {
		if text == "" {
			return // 空增量不推进首字延迟,也不向下游转发
		}
		res.Reply += text
		if !firstSeen {
			res.FirstLatencyMs = time.Since(start).Milliseconds()
			firstSeen = true
		}
		if onDelta != nil {
			onDelta(text)
		}
	}
	consume := streamConsumer(req.Protocol, &res, emit)
	if consume == nil {
		return Result{}, apperr.New(apperr.CodeTestInvalid, "不支持的对话协议: "+string(req.Protocol))
	}
	if err := consumeSSEStream(ctx, resp.Body, consume); err != nil {
		return res, err
	}
	res.TotalLatencyMs = time.Since(start).Milliseconds()
	if !firstSeen {
		// 全程无内容增量(如空回复):首字延迟与总耗时取同一时刻
		res.FirstLatencyMs = res.TotalLatencyMs
	}
	return res, nil
}

// sendChatRequest 构造协议请求并发送,统一处理 URL 拼接、鉴权头与响应前
// 错误(网络/鉴权/状态码);响应体交由调用方按全量或流式分别解析。
func sendChatRequest(ctx context.Context, cfg UpstreamConfig, req ChatRequest) (*http.Response, error) {
	payload, err := buildPayload(req)
	if err != nil {
		// 参数合法性由 service 层前置校验,走到这里属程序性异常
		return nil, apperr.Wrap(apperr.CodeTestInvalid, "构造测试请求失败,参数不合法", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, BuildURL(cfg.BaseURL, req.Protocol.path()), bytes.NewReader(payload))
	if err != nil {
		// 配置创建时已校验 URL 格式,走到这里说明存量数据异常,按网络类处理
		return nil, apperr.Wrap(apperr.CodeUpstreamNetwork, "无法连接上游服务:上游地址不合法", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	accept := "application/json"
	if req.Stream {
		accept = "text/event-stream"
	}
	httpReq.Header.Set("Accept", accept)
	applyAuth(httpReq.Header, cfg, req.Protocol)

	client := chatClient
	if req.Stream {
		client = streamClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err // 主动取消返回原始错误,不归类
		}
		return nil, apperr.Wrap(apperr.CodeUpstreamNetwork, "无法连接上游服务:"+describeNetErr(err), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, apperr.New(apperr.CodeUpstreamAuth, "鉴权失败,请检查 API Key")
		}
		return nil, apperr.New(apperr.CodeUpstreamRequestFailed, fmt.Sprintf("上游请求失败(HTTP %d)", resp.StatusCode))
	}
	return resp, nil
}

// applyAuth 按协议设置鉴权头:anthropic 用 x-api-key + 版本头,其余用 Bearer。
func applyAuth(h http.Header, cfg UpstreamConfig, p Protocol) {
	if p == ProtocolAnthropic {
		h.Set("x-api-key", cfg.APIKey)
		h.Set("anthropic-version", "2023-06-01")
		return
	}
	h.Set("Authorization", "Bearer "+cfg.APIKey)
}

// buildPayload 按协议构造请求体;字段命名与可省略性逐项对应协议差异表:
// system 分别落在 messages[0]/顶层 instructions/顶层 system,temperature 与
// max_tokens 为 nil 时不发送,anthropic 的 max_tokens 必填(缺省补 1024)。
func buildPayload(req ChatRequest) ([]byte, error) {
	switch req.Protocol {
	case ProtocolChatCompletions:
		messages := make([]chatMessage, 0, 2)
		if req.System != "" {
			messages = append(messages, chatMessage{Role: "system", Content: req.System})
		}
		messages = append(messages, chatMessage{Role: "user", Content: req.User})
		payload := struct {
			Model         string        `json:"model"`
			Messages      []chatMessage `json:"messages"`
			Temperature   *float64      `json:"temperature,omitempty"`
			MaxTokens     *int          `json:"max_tokens,omitempty"`
			Stream        bool          `json:"stream"`
			StreamOptions *struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options,omitempty"`
		}{
			Model:       req.Model,
			Messages:    messages,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			Stream:      req.Stream,
		}
		if req.Stream {
			// 请求上游在末尾 chunk 下发 usage(此时 choices 为空数组);
			// 上游不支持该参数时用量记为未返回,不影响回复内容
			payload.StreamOptions = &struct {
				IncludeUsage bool `json:"include_usage"`
			}{IncludeUsage: true}
		}
		return json.Marshal(payload)

	case ProtocolResponses:
		payload := struct {
			Model           string        `json:"model"`
			Instructions    string        `json:"instructions,omitempty"`
			Input           []chatMessage `json:"input"`
			Temperature     *float64      `json:"temperature,omitempty"`
			MaxOutputTokens *int          `json:"max_output_tokens,omitempty"`
			Stream          bool          `json:"stream"`
		}{
			Model:           req.Model,
			Input:           []chatMessage{{Role: "user", Content: req.User}},
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
			Stream:          req.Stream,
		}
		if req.System != "" {
			payload.Instructions = req.System
		}
		return json.Marshal(payload)

	case ProtocolAnthropic:
		maxTokens := defaultAnthropicMaxTokens
		if req.MaxTokens != nil {
			maxTokens = *req.MaxTokens
		}
		payload := struct {
			Model       string        `json:"model"`
			System      string        `json:"system,omitempty"`
			Messages    []chatMessage `json:"messages"`
			Temperature *float64      `json:"temperature,omitempty"`
			MaxTokens   int           `json:"max_tokens"`
			Stream      bool          `json:"stream"`
		}{
			Model:       req.Model,
			Messages:    []chatMessage{{Role: "user", Content: req.User}},
			Temperature: req.Temperature,
			MaxTokens:   maxTokens,
			Stream:      req.Stream,
		}
		if req.System != "" {
			payload.System = req.System
		}
		return json.Marshal(payload)

	default:
		return nil, fmt.Errorf("不支持的对话协议: %s", req.Protocol)
	}
}

// parseFullReply 按协议解析非流式响应体,返回回复文本与 token 用量。
func parseFullReply(p Protocol, body []byte) (string, Usage, error) {
	switch p {
	case ProtocolChatCompletions:
		return parseChatCompletionsFull(body)
	case ProtocolResponses:
		return parseResponsesFull(body)
	case ProtocolAnthropic:
		return parseAnthropicFull(body)
	default:
		return "", Usage{}, apperr.New(apperr.CodeTestInvalid, "不支持的对话协议: "+string(p))
	}
}

// parseChatCompletionsFull 全量回复取 choices[0].message.content,
// 用量为 prompt/completion/total_tokens。
func parseChatCompletionsFull(body []byte) (string, Usage, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", Usage{}, apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游响应解析失败,不符合 OpenAI Chat Completions 结构", err)
	}
	if resp.Error != nil {
		return "", Usage{}, apperr.New(apperr.CodeUpstreamRequestFailed, upstreamErrMsg(resp.Error.Message))
	}
	if len(resp.Choices) == 0 {
		return "", Usage{}, apperr.New(apperr.CodeUpstreamParseFailed, "上游响应解析失败,缺少回复内容")
	}
	reply := ""
	if resp.Choices[0].Message.Content != nil {
		reply = *resp.Choices[0].Message.Content
	}
	usage := Usage{}
	if resp.Usage != nil {
		usage = Usage{Prompt: resp.Usage.PromptTokens, Completion: resp.Usage.CompletionTokens, Total: resp.Usage.TotalTokens}
	}
	return reply, usage, nil
}

// parseResponsesFull 优先取顶层 output_text(官方聚合字段);缺失时汇总
// output[] 各条目 content[type=output_text] 的文本(只有 message 条目会
// 携带 output_text 块,故按块类型过滤即可)。用量为 input/output/total_tokens。
func parseResponsesFull(body []byte) (string, Usage, error) {
	var resp struct {
		OutputText *string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", Usage{}, apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游响应解析失败,不符合 OpenAI Responses 结构", err)
	}
	if resp.Error != nil {
		return "", Usage{}, apperr.New(apperr.CodeUpstreamRequestFailed, upstreamErrMsg(resp.Error.Message))
	}
	var reply string
	haveText := false
	if resp.OutputText != nil {
		reply, haveText = *resp.OutputText, true
	} else {
		parts := make([]string, 0, len(resp.Output))
		for _, item := range resp.Output {
			for _, block := range item.Content {
				if block.Type == "output_text" {
					parts = append(parts, block.Text)
				}
			}
		}
		reply, haveText = strings.Join(parts, ""), len(resp.Output) > 0
	}
	if !haveText {
		return "", Usage{}, apperr.New(apperr.CodeUpstreamParseFailed, "上游响应解析失败,缺少回复内容")
	}
	usage := Usage{}
	if resp.Usage != nil {
		usage = Usage{Prompt: resp.Usage.InputTokens, Completion: resp.Usage.OutputTokens, Total: resp.Usage.TotalTokens}
	}
	return reply, usage, nil
}

// parseAnthropicFull 回复取 content[] 中 type=text 的 text 拼接;
// 用量为 input_tokens/output_tokens,Total 取两者相加。
func parseAnthropicFull(body []byte) (string, Usage, error) {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", Usage{}, apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游响应解析失败,不符合 Anthropic Messages 结构", err)
	}
	if resp.Error != nil {
		return "", Usage{}, apperr.New(apperr.CodeUpstreamRequestFailed, upstreamErrMsg(resp.Error.Message))
	}
	if len(resp.Content) == 0 {
		return "", Usage{}, apperr.New(apperr.CodeUpstreamParseFailed, "上游响应解析失败,缺少回复内容")
	}
	parts := make([]string, 0, len(resp.Content))
	for _, block := range resp.Content {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	usage := Usage{}
	if resp.Usage != nil {
		usage = Usage{
			Prompt:     resp.Usage.InputTokens,
			Completion: resp.Usage.OutputTokens,
			Total:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
	}
	return strings.Join(parts, ""), usage, nil
}

// streamConsumer 按协议返回流式事件消费函数;协议不支持时返回 nil。
func streamConsumer(p Protocol, res *Result, emit func(string)) func(SSEEvent) error {
	switch p {
	case ProtocolChatCompletions:
		return consumeChatCompletionsStream(res, emit)
	case ProtocolResponses:
		return consumeResponsesStream(res, emit)
	case ProtocolAnthropic:
		return consumeAnthropicStream(res, emit)
	default:
		return nil
	}
}

// consumeChatCompletionsStream 解析 chat_completions 流式分块:增量取
// choices[].delta.content;末尾 chunk 的 usage 记录用量;data: [DONE] 结束。
func consumeChatCompletionsStream(res *Result, emit func(string)) func(SSEEvent) error {
	return func(ev SSEEvent) error {
		if strings.TrimSpace(ev.Data) == "[DONE]" {
			return errStreamDone
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content *string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &chunk); err != nil {
			return apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游流式响应解析失败,不符合 OpenAI Chat Completions 事件结构", err)
		}
		if chunk.Error != nil {
			return apperr.New(apperr.CodeUpstreamRequestFailed, upstreamErrMsg(chunk.Error.Message))
		}
		if chunk.Usage != nil {
			res.Usage = Usage{Prompt: chunk.Usage.PromptTokens, Completion: chunk.Usage.CompletionTokens, Total: chunk.Usage.TotalTokens}
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != nil {
				emit(*ch.Delta.Content)
			}
		}
		return nil
	}
}

// consumeResponsesStream 解析 responses 流式事件:response.output_text.delta
// 携带内容增量;response.completed / response.incomplete 携带 usage 并结束。
func consumeResponsesStream(res *Result, emit func(string)) func(SSEEvent) error {
	return func(ev SSEEvent) error {
		switch eventName(ev) {
		case "response.output_text.delta":
			var d struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &d); err != nil {
				return apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游流式响应解析失败,不符合 OpenAI Responses 事件结构", err)
			}
			emit(d.Delta)
		case "response.completed", "response.incomplete":
			var d struct {
				Response struct {
					Usage struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
						TotalTokens  int `json:"total_tokens"`
					} `json:"usage"`
				} `json:"response"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &d); err != nil {
				return apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游流式响应解析失败,不符合 OpenAI Responses 事件结构", err)
			}
			res.Usage = Usage{
				Prompt:     d.Response.Usage.InputTokens,
				Completion: d.Response.Usage.OutputTokens,
				Total:      d.Response.Usage.TotalTokens,
			}
			return errStreamDone
		case "response.failed", "error":
			return apperr.New(apperr.CodeUpstreamRequestFailed, upstreamErrMsg(streamErrorMessage(ev.Data)))
		}
		// 其余事件(response.created、ping 等)与回复内容无关,忽略
		return nil
	}
}

// consumeAnthropicStream 解析 anthropic 流式事件:message_start 携带输入
// 用量,content_block_delta(delta.type=text_delta)携带增量,message_delta
// 携带输出用量,message_stop 结束。
func consumeAnthropicStream(res *Result, emit func(string)) func(SSEEvent) error {
	return func(ev SSEEvent) error {
		switch eventName(ev) {
		case "message_start":
			var d struct {
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &d); err != nil {
				return apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游流式响应解析失败,不符合 Anthropic Messages 事件结构", err)
			}
			// anthropic 的用量分两处下发:此处记输入,message_delta 记输出
			res.Usage.Prompt = d.Message.Usage.InputTokens
			res.Usage.Total = res.Usage.Prompt + res.Usage.Completion
		case "content_block_delta":
			var d struct {
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &d); err != nil {
				return apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游流式响应解析失败,不符合 Anthropic Messages 事件结构", err)
			}
			if d.Delta.Type == "text_delta" {
				emit(d.Delta.Text)
			}
		case "message_delta":
			var d struct {
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &d); err != nil {
				return apperr.Wrap(apperr.CodeUpstreamParseFailed, "上游流式响应解析失败,不符合 Anthropic Messages 事件结构", err)
			}
			res.Usage.Completion = d.Usage.OutputTokens
			res.Usage.Total = res.Usage.Prompt + res.Usage.Completion
		case "message_stop":
			return errStreamDone
		case "error":
			return apperr.New(apperr.CodeUpstreamRequestFailed, upstreamErrMsg(streamErrorMessage(ev.Data)))
		}
		return nil
	}
}

// eventName 返回事件的生效名称:优先 event: 行;部分代理会省略该行,此时
// 退回 data JSON 中的 type 字段(三种协议的事件 data 均携带 type)。
func eventName(ev SSEEvent) string {
	if ev.Event != "" {
		return ev.Event
	}
	var d struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(ev.Data), &d); err != nil {
		return ""
	}
	return d.Type
}

// streamErrorMessage 从流中错误事件提取上游错误消息;三种协议的错误载体
// 不同(chat 在 data.error,responses 在 data.response.error,anthropic 在
// data.error),这里统一兜底解析,取不到时返回空串。
func streamErrorMessage(data string) string {
	var e struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Response *struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return ""
	}
	if e.Error != nil {
		return e.Error.Message
	}
	if e.Response != nil && e.Response.Error != nil {
		return e.Response.Error.Message
	}
	return ""
}

// upstreamErrMsg 组装上游业务错误的中文提示;上游未提供消息时退回通用文案。
func upstreamErrMsg(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "上游请求失败"
	}
	return "上游返回错误:" + msg
}
