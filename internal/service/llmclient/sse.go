// sse.go 提供通用的 SSE(Server-Sent Events)行解析器,供三种协议的流式
// 响应解析复用。按行喂入,event:/data: 字段累积,空行派发一次完整事件;
// 多行 data 以 \n 拼接(SSE 规范)。跨网络包的行边界由 bufio.Scanner 处理,
// 本解析器只关心完整的行。
package llmclient

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// SSEEvent 一次派发的 SSE 事件:Event 为 event: 行声明的名称(未声明时为
// 空串,消费方可用 data 中的 type 字段兜底),Data 为拼接后的 data 内容。
type SSEEvent struct {
	Event string
	Data  string
}

// errStreamDone 供协议事件回调提前终止读取的哨兵错误,表示已收到协议约定
// 的结束事件,不是真正的失败。
var errStreamDone = errors.New("sse stream done")

// SSEParser 通用 SSE 解析器:通过 Feed 逐行喂入,凑齐一个完整事件
// (遇到空行)时回调 onEvent。非线程安全,须由单个 goroutine 顺序喂入。
type SSEParser struct {
	event   string
	data    []string
	onEvent func(SSEEvent)
}

// NewSSEParser 创建解析器;onEvent 在每次完整事件就绪时被调用。
func NewSSEParser(onEvent func(SSEEvent)) *SSEParser {
	return &SSEParser{onEvent: onEvent}
}

// Feed 喂入一行(不含行终止符)。识别 event:/data: 字段、忽略注释行
// 与 id:/retry: 等无关字段;空行触发派发。
func (p *SSEParser) Feed(line string) {
	// 兜底去掉行尾 \r:部分上游以 \r\n 结尾,避免破坏字段值
	line = strings.TrimSuffix(line, "\r")
	switch {
	case line == "":
		p.dispatch()
	case strings.HasPrefix(line, ":"):
		// 注释行(常用于心跳保活),忽略
	case strings.HasPrefix(line, "event:"):
		p.event = trimFieldSpace(line[len("event:"):])
	case strings.HasPrefix(line, "data:"):
		// 只去掉一个可选前导空格,保留正文本身的缩进
		p.data = append(p.data, trimFieldSpace(line[len("data:"):]))
	default:
		// id:/retry: 等字段与本任务无关,忽略
	}
}

// Finish 在流结束时派发尚未遇到空行的悬挂事件(容错部分上游不写终止空行)。
func (p *SSEParser) Finish() { p.dispatch() }

// dispatch 派发当前累积的事件并复位状态;data 内容为空时不派发(SSE 规范)。
func (p *SSEParser) dispatch() {
	event, data := p.event, strings.Join(p.data, "\n")
	p.event, p.data = "", p.data[:0]
	if data == "" {
		return
	}
	if p.onEvent != nil {
		p.onEvent(SSEEvent{Event: event, Data: data})
	}
}

// consumeSSEStream 用 SSEParser 逐行读取 body 并回调 handle;
// handle 返回 errStreamDone 时视为正常结束,返回其他错误时原样上抛。
// 上游读取失败(连接中断)归类为网络错误 1500;ctx 已取消时返回原始错误。
func consumeSSEStream(ctx context.Context, body io.Reader, handle func(SSEEvent) error) error {
	var handleErr error
	parser := NewSSEParser(func(ev SSEEvent) {
		// 只保留第一个错误;收到后由下方循环停止喂入
		if handleErr == nil {
			handleErr = handle(ev)
		}
	})
	// SSE 单行(长回复分片)可能超过 Scanner 默认 64KB 行上限,放大缓冲
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		parser.Feed(scanner.Text())
		if handleErr != nil {
			break
		}
	}
	if handleErr != nil {
		if errors.Is(handleErr, errStreamDone) {
			return nil
		}
		return handleErr
	}
	if err := scanner.Err(); err != nil {
		// 请求被业务方取消(如用户中断)不属于上游故障,原样返回
		if ctx.Err() != nil {
			return err
		}
		return apperr.Wrap(apperr.CodeUpstreamNetwork, "读取上游流式响应失败:"+describeNetErr(err), err)
	}
	parser.Finish()
	return nil
}

// trimFieldSpace 去掉字段值前的单个可选空格("data: xxx"),避免误删正文缩进。
func trimFieldSpace(v string) string {
	return strings.TrimPrefix(v, " ")
}
