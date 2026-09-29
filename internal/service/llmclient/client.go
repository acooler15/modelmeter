// Package llmclient 提供访问上游 LLM 服务的 HTTP 客户端封装:URL 归一化、
// Bearer 鉴权、超时控制与错误归类(网络/鉴权/响应异常)。它是模型列表、
// 模型测试、费率查询任务的共用基座;service 其余部分不得直接发起上游请求。
// 约定:API Key 只在此包内进入 Authorization header,禁止写入任何日志。
package llmclient

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// upstreamTimeout 单次上游请求的整体超时;超时归入网络错误(1500)。
const upstreamTimeout = 30 * time.Second

// httpClient 共享的只读客户端:http.Client 并发安全且创建后不再变更,
// 不属于包级可变状态。
var httpClient = &http.Client{Timeout: upstreamTimeout}

// UpstreamConfig 上游访问配置,来源于已保存的接口配置。
type UpstreamConfig struct {
	BaseURL string
	APIKey  string
}

// NormalizeBaseURL 去除 Base URL 首尾空白与尾部 '/',统一后续拼接行为,
// 兼容 "https://x.com" 与 "https://x.com/" 两种填写习惯。
func NormalizeBaseURL(base string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/")
}

// BuildURL 拼接上游请求地址:path 必须以 "/v1/" 开头;base 归一化后
// 已以 "/v1" 结尾时去掉 path 的 "/v1" 前缀,避免拼出重复的 "/v1/v1"。
func BuildURL(base, path string) string {
	b := NormalizeBaseURL(base)
	if strings.HasSuffix(b, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return b + path
}

// ErrCodes 上游错误归类的参数化配置:不同上游(模型站点、New API 网关)
// 使用各自资源分段的业务错误码,中文文案模板共用。
type ErrCodes struct {
	Network     int    // 网络类错误(连接失败/超时/DNS/TLS)的业务码
	Auth        int    // 鉴权失败(HTTP 401/403)的业务码
	BadResponse int    // 其余非 2xx 状态码或 JSON 解析失败的业务码
	ServiceName string // 文案中的服务名,如「上游服务」「New API 服务」
	AuthHint    string // 鉴权失败文案中的凭据名称,如「API Key」「New API 访问令牌」
}

// UpstreamCodes 1xxx 段上游访问类错误码组:模型列表与模型测试共用,
// 也是 GetJSON 的默认行为(文案与历史版本保持一致)。
var UpstreamCodes = ErrCodes{
	Network:     apperr.CodeUpstreamNetwork,
	Auth:        apperr.CodeUpstreamAuth,
	BadResponse: apperr.CodeUpstreamBadResponse,
	ServiceName: "上游服务",
	AuthHint:    "API Key",
}

// GetJSON 以 Bearer 鉴权请求上游 GET 接口,并把 JSON 响应解析到 out。
// 错误按 1xxx 段上游错误码归类(等价于 GetJSONWithCodes 的默认码组):
//   - 网络类(连接失败/超时/DNS/TLS)→ CodeUpstreamNetwork;
//   - HTTP 401/403 → CodeUpstreamAuth;
//   - 其余非 2xx 状态码或 JSON 解析失败 → CodeUpstreamBadResponse。
func GetJSON(ctx context.Context, cfg UpstreamConfig, path string, out any) error {
	return GetJSONWithCodes(ctx, cfg, path, out, UpstreamCodes)
}

// GetJSONWithCodes 是 GetJSON 的参数化版本:错误码与文案由 codes 决定,
// 供不同上游分段(如 New API 3xxx 段)复用同一套请求与归类逻辑。
func GetJSONWithCodes(ctx context.Context, cfg UpstreamConfig, path string, out any, codes ErrCodes) error {
	// 服务名缺省回退为通用「上游服务」,保证零值码组也能给出可读文案
	name := codes.ServiceName
	if name == "" {
		name = "上游服务"
	}
	hint := codes.AuthHint
	if hint == "" {
		hint = "API Key"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BuildURL(cfg.BaseURL, path), nil)
	if err != nil {
		// 配置创建时已校验 URL 格式,走到这里说明存量数据异常,按网络类处理
		return apperr.Wrap(codes.Network, "无法连接"+name+":上游地址不合法", err)
	}
	// API Key 只进入请求头,不进任何日志与错误消息
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		// 请求被业务方取消(如客户端断开)不属于上游故障,原样返回交由上层判断
		if ctx.Err() != nil {
			return err
		}
		return apperr.Wrap(codes.Network, "无法连接"+name+":"+describeNetErr(err), err)
	}
	defer func() {
		// 读取失败时也需要关闭连接;关闭错误不影响业务结果,可安全忽略
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return apperr.New(codes.Auth, "鉴权失败,请检查 "+hint)
		}
		return apperr.New(codes.BadResponse, fmt.Sprintf("%s返回异常(HTTP %d)", name, resp.StatusCode))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apperr.Wrap(codes.BadResponse, name+"响应解析失败,结构不符合预期", err)
	}
	return nil
}

// maxReasonLen 限制网络错误根因摘要的长度,保证用户提示简短可读。
const maxReasonLen = 120

// describeNetErr 提取网络错误的简短根因用于中文提示;完整错误链仍保留在
// apperr.Err 中供日志排查。无法识别的类型回退为原文摘要。
func describeNetErr(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "域名解析失败"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "请求超时"
	}
	var unknownAuth x509.UnknownAuthorityError
	var certInvalid x509.CertificateInvalidError
	var hostnameErr x509.HostnameError
	if errors.As(err, &unknownAuth) || errors.As(err, &certInvalid) || errors.As(err, &hostnameErr) {
		return "TLS 证书验证失败"
	}
	// 截断按 rune 处理,避免把中文提示切出非法字节序列
	reason := []rune(err.Error())
	if len(reason) > maxReasonLen {
		reason = append(reason[:maxReasonLen], []rune("...")...)
	}
	return string(reason)
}
