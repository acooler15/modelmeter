package llmclient

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// TestBuildURL_三种BaseURL形态 覆盖根地址、带尾斜杠、带 /v1 与空白字符的写法,
// 结果都应只拼出一段 /v1。
func TestBuildURL_三种BaseURL形态(t *testing.T) {
	cases := []struct {
		name string
		base string
		path string
		want string
	}{
		{"根地址", "https://api.example.com", "/v1/models", "https://api.example.com/v1/models"},
		{"带尾斜杠", "https://api.example.com/", "/v1/models", "https://api.example.com/v1/models"},
		{"带v1", "https://api.example.com/v1", "/v1/models", "https://api.example.com/v1/models"},
		{"带v1和尾斜杠", "https://api.example.com/v1/", "/v1/models", "https://api.example.com/v1/models"},
		{"带首尾空白", "  https://api.example.com/  ", "/v1/models", "https://api.example.com/v1/models"},
	}
	for _, c := range cases {
		if got := BuildURL(c.base, c.path); got != c.want {
			t.Errorf("%s: BuildURL(%q, %q) = %q,期望 %q", c.name, c.base, c.path, got, c.want)
		}
	}
}

// TestGetJSON_成功解析响应 2xx 响应应解析进 out,且请求头携带 Bearer 凭据。
func TestGetJSON_成功解析响应(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Errorf("期望请求路径 /v1/models,实际 %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4"}]}`))
	}))
	defer srv.Close()

	var out struct {
		Data []json.RawMessage `json:"data"`
	}
	cfg := UpstreamConfig{BaseURL: srv.URL, APIKey: "sk-test-key-1234"}
	if err := GetJSON(context.Background(), cfg, "/v1/models", &out); err != nil {
		t.Fatalf("GetJSON 应成功,实际报错: %v", err)
	}
	if gotAuth != "Bearer sk-test-key-1234" {
		t.Errorf("期望 Authorization 头为 Bearer 凭据,实际 %q", gotAuth)
	}
	if len(out.Data) != 1 {
		t.Fatalf("期望解析出 1 条 data,实际 %d 条", len(out.Data))
	}
}

// TestGetJSON_响应归类 覆盖坏 JSON、401/403、其他非 2xx 与拒连四类错误归类。
func TestGetJSON_响应归类(t *testing.T) {
	ctx := context.Background()

	// 坏 JSON:200 但响应体不是 JSON → 1502
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()
	err := GetJSON(ctx, UpstreamConfig{BaseURL: srv.URL}, "/v1/models", &struct{}{})
	wantCode(t, err, apperr.CodeUpstreamBadResponse)

	// 401/403 → 1501,文案固定为鉴权提示
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		err := GetJSON(ctx, UpstreamConfig{BaseURL: authSrv.URL, APIKey: "sk-leak-check"}, "/v1/models", &struct{}{})
		authSrv.Close()
		wantCode(t, err, apperr.CodeUpstreamAuth)
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Message != "鉴权失败,请检查 API Key" {
			t.Errorf("HTTP %d 期望鉴权中文文案,实际 %v", status, err)
		}
		// API Key 不允许出现在错误消息里
		if strings.Contains(err.Error(), "sk-leak-check") {
			t.Errorf("错误消息泄露 API Key: %v", err)
		}
	}

	// 500 → 1502,消息附上游状态码
	serverErrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer serverErrSrv.Close()
	err = GetJSON(ctx, UpstreamConfig{BaseURL: serverErrSrv.URL}, "/v1/models", &struct{}{})
	wantCode(t, err, apperr.CodeUpstreamBadResponse)
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("期望消息包含上游状态码 500,实际 %v", err)
	}

	// 拒连 → 1500:复用一个已关闭的假上游地址
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	err = GetJSON(ctx, UpstreamConfig{BaseURL: closedURL}, "/v1/models", &struct{}{})
	wantCode(t, err, apperr.CodeUpstreamNetwork)
}

// timeoutError 用于构造带 Timeout 标记的 net.Error,避免真实等待超时。
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

// TestDescribeNetErr_根因分类 覆盖 DNS、超时、TLS 与未知错误的摘要输出。
func TestDescribeNetErr_根因分类(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"DNS 解析失败", fmt.Errorf("dial: %w", &net.DNSError{Err: "no such host", Name: "a.b.c"}), "域名解析失败"},
		{"请求超时", fmt.Errorf("wrap: %w", timeoutError{}), "请求超时"},
		{"TLS 证书错误", fmt.Errorf("wrap: %w", x509.UnknownAuthorityError{}), "TLS 证书验证失败"},
		{"未知错误回退原文", errors.New("dial tcp 127.0.0.1:1: connect: connection refused"), "dial tcp 127.0.0.1:1: connect: connection refused"},
	}
	for _, c := range cases {
		if got := describeNetErr(c.err); got != c.want {
			t.Errorf("%s: describeNetErr = %q,期望 %q", c.name, got, c.want)
		}
	}
}

// TestDescribeNetErr_超长摘要截断 超长错误文本应被截断且不超过上限。
func TestDescribeNetErr_超长摘要截断(t *testing.T) {
	long := errors.New(strings.Repeat("x", 500))
	got := describeNetErr(long)
	if len([]rune(got)) > maxReasonLen+len("...") {
		t.Errorf("期望摘要被截断,实际长度 %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("期望截断后以省略号结尾,实际 %q", got)
	}
}

// wantCode 断言 err 为业务错误且业务码一致。
func wantCode(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望业务错误码 %d,实际成功", code)
	}
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		t.Fatalf("期望错误被包装为 apperr.Error,实际 %T: %v", err, err)
	}
	if ae.Code != code {
		t.Fatalf("期望业务码 %d,实际 %d(%v)", code, ae.Code, err)
	}
}
