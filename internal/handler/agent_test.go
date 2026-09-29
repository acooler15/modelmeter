package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newAgentTestRouter 仅装配路由;Agent 处理器不依赖数据库,db 传 nil 即可。
func newAgentTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return NewRouter(nil, t.TempDir())
}

// getJSON 向路由发起 GET 并解析统一信封。
func getJSON(router *gin.Engine, path string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env) // 失败时由调用方按空信封判失败
	return rec, env
}

// TestAgentGet_未知名称_4401 未注册的 Agent 名称统一报 4401。
func TestAgentGet_未知名称_4401(t *testing.T) {
	router := newAgentTestRouter(t)
	rec, env := getJSON(router, "/api/agents/nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if env["code"].(float64) != 4401 {
		t.Errorf("期望业务码 4401,实际 %v", env["code"])
	}
}

// TestAgentApply_未知名称_4401 写回未知名称同样报 4401。
func TestAgentApply_未知名称_4401(t *testing.T) {
	router := newAgentTestRouter(t)
	req := httptest.NewRequest(http.MethodPut, "/api/agents/nope", strings.NewReader(`{"values":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var env struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("信封解析失败: %v", err)
	}
	if env.Code != 4401 {
		t.Errorf("期望业务码 4401,实际 %d", env.Code)
	}
}

// TestAgentRestore_语义 还原已注册但无备份的工具报 4404(配置缺失或无备份
// 两个分支均为 4404);还原未知名称报 4401。
func TestAgentRestore_语义(t *testing.T) {
	router := newAgentTestRouter(t)
	rec := postJSON(router, "/api/agents/workbuddy/restore", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 HTTP 404,实际 %d", rec.Code)
	}
	var env struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("信封解析失败: %v", err)
	}
	if env.Code != 4404 {
		t.Errorf("还原无备份的已注册工具应报 4404,实际 %d", env.Code)
	}

	rec = postJSON(router, "/api/agents/nope/restore", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("信封解析失败: %v", err)
	}
	if env.Code != 4401 {
		t.Errorf("还原未知名称应报 4401,实际 %d", env.Code)
	}
}

// TestAgentList_包含全部注册工具 列表按名称排序返回且信封为成功。
func TestAgentList_包含全部注册工具(t *testing.T) {
	router := newAgentTestRouter(t)
	rec, env := getJSON(router, "/api/agents")
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200,实际 %d", rec.Code)
	}
	if env["code"].(float64) != 0 {
		t.Fatalf("期望成功信封,实际 %v", env["code"])
	}
	data, _ := env["data"].([]any)
	if len(data) < 2 {
		t.Fatalf("应包含 zcode 与 workbuddy 两个工具,实际 %d 个", len(data))
	}
	first, _ := data[0].(map[string]any)
	if first["name"] != "workbuddy" {
		t.Errorf("列表应按名称排序(workbuddy 在前),实际 %v", first["name"])
	}
}
