package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/service/agentconf"

	// 测试二进制内触发 Agent 配置实现的自注册(生产由 main.go 空导入触发)
	_ "github.com/acooler15/modelmeter/internal/service/agentconf/agents"
)

// newAgentTestRouter 仅装配路由;Agent 处理器不依赖数据库,db 传 nil 即可。
// 返回路由与 dataDir,供验证备份目录参数透传。
func newAgentTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	return NewRouter(nil, dataDir), dataDir
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

// putJSON 向路由发起带 JSON 请求体的 PUT。
func putJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// envelopeCode 解析响应信封中的业务码;解析失败返回 -1。
func envelopeCode(t *testing.T, rec *httptest.ResponseRecorder) int {
	t.Helper()
	var env struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("信封解析失败: %v", err)
	}
	return env.Code
}

// fakeAgent 假实现:验证 models 两端点的路由、信封与参数透传,
// 不触碰本机真实配置文件。命名为 zz-fake 保证排序在真实工具之后。
type fakeAgent struct {
	entries    []agentconf.ModelEntry
	err        error
	gotPatches []agentconf.ModelPatch
	gotDataDir string
}

func (f *fakeAgent) Name() string        { return "zz-fake" }
func (f *fakeAgent) DisplayName() string { return "假工具" }

func (f *fakeAgent) Snapshot(_ context.Context) agentconf.Snapshot {
	return agentconf.Snapshot{
		Name:        f.Name(),
		DisplayName: f.DisplayName(),
		Status:      agentconf.StatusFound,
		Columns:     []agentconf.FieldSpec{},
	}
}

func (f *fakeAgent) Models(_ context.Context) ([]agentconf.ModelEntry, error) {
	return f.entries, f.err
}

func (f *fakeAgent) ApplyModels(_ context.Context, patches []agentconf.ModelPatch, dataDir string) ([]agentconf.ModelEntry, error) {
	f.gotPatches = patches
	f.gotDataDir = dataDir
	return f.entries, f.err
}

// testFake 全局唯一的假实现实例,注册一次,用例各自改写其行为字段。
var (
	testFake = &fakeAgent{}
	fakeOnce sync.Once
)

// registerFakeAgent 把假实现注册进全局注册表(仅一次)。
func registerFakeAgent(t *testing.T) {
	t.Helper()
	fakeOnce.Do(func() { agentconf.Register(testFake) })
}

// TestAgentGet_未知名称_4401 未注册的 Agent 名称统一报 4401。
func TestAgentGet_未知名称_4401(t *testing.T) {
	router, _ := newAgentTestRouter(t)
	rec, env := getJSON(router, "/api/agents/nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if env["code"].(float64) != 4401 {
		t.Errorf("期望业务码 4401,实际 %v", env["code"])
	}
}

// TestAgentModelsGet_未知名称_4401 模型清单端点对未知名称同样报 4401。
func TestAgentModelsGet_未知名称_4401(t *testing.T) {
	router, _ := newAgentTestRouter(t)
	rec, env := getJSON(router, "/api/agents/nope/models")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if env["code"].(float64) != 4401 {
		t.Errorf("期望业务码 4401,实际 %v", env["code"])
	}
}

// TestAgentModelsApply_未知名称_4401 批量写回端点对未知名称同样报 4401。
func TestAgentModelsApply_未知名称_4401(t *testing.T) {
	router, _ := newAgentTestRouter(t)
	rec := putJSON(router, "/api/agents/nope/models", `{"patches":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4401 {
		t.Errorf("期望业务码 4401,实际 %d", code)
	}
}

// TestAgentModelsGet_信封与脱敏数据 成功信封按 data 返回清单数组。
func TestAgentModelsGet_信封与脱敏数据(t *testing.T) {
	registerFakeAgent(t)
	testFake.err = nil
	testFake.entries = []agentconf.ModelEntry{{
		ProviderID: "p1", ProviderName: "供应商一", ModelID: "m-1", DisplayName: "m-1",
		Fields: map[string]any{"enabled": true},
	}}
	defer func() { testFake.entries = nil }()

	router, _ := newAgentTestRouter(t)
	rec, env := getJSON(router, "/api/agents/zz-fake/models")
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200,实际 %d", rec.Code)
	}
	if env["code"].(float64) != 0 {
		t.Fatalf("期望成功信封,实际 %v", env["code"])
	}
	data, _ := env["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("应返回 1 个条目,实际 %d 个", len(data))
	}
	first, _ := data[0].(map[string]any)
	if first["model_id"] != "m-1" || first["provider_id"] != "p1" {
		t.Errorf("条目字段解析错误,实际 %v", first)
	}
}

// TestAgentModelsApply_信封与参数透传 body 的 patches 原样传给实现,
// dataDir 使用路由装配时注入的目录,响应为最新清单。
func TestAgentModelsApply_信封与参数透传(t *testing.T) {
	registerFakeAgent(t)
	testFake.err = nil
	testFake.entries = []agentconf.ModelEntry{{ModelID: "m-1", Fields: map[string]any{"enabled": false}}}

	router, dataDir := newAgentTestRouter(t)
	body := `{"patches":[{"provider_id":"p1","model_id":"m-1","fields":{"enabled":true}}]}`
	rec := putJSON(router, "/api/agents/zz-fake/models", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 0 {
		t.Fatalf("期望成功信封,实际 %d", code)
	}
	if len(testFake.gotPatches) != 1 {
		t.Fatalf("应透传 1 个 patch,实际 %d 个", len(testFake.gotPatches))
	}
	p := testFake.gotPatches[0]
	if p.ProviderID != "p1" || p.ModelID != "m-1" || p.Fields["enabled"] != true {
		t.Errorf("patch 内容透传错误,实际 %+v", p)
	}
	if testFake.gotDataDir != dataDir {
		t.Errorf("dataDir 应为路由注入的目录,实际 %q", testFake.gotDataDir)
	}
	// 响应 data 为实现返回的最新清单
	var env struct {
		Data []agentconf.ModelEntry `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0].ModelID != "m-1" {
		t.Errorf("应返回最新清单,实际 %+v", env.Data)
	}
}

// TestAgentModelsApply_非法body_4403 请求体不是合法 JSON 时报 4403。
func TestAgentModelsApply_非法body_4403(t *testing.T) {
	registerFakeAgent(t)
	router, _ := newAgentTestRouter(t)
	rec := putJSON(router, "/api/agents/zz-fake/models", "{bad")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4403 {
		t.Errorf("期望业务码 4403,实际 %d", code)
	}
}

// TestAgentModels_实现错误透传 实现返回的业务错误按码透传(如 4402)。
func TestAgentModels_实现错误透传(t *testing.T) {
	registerFakeAgent(t)
	testFake.err = apperr.New(apperr.CodeAgentFileIO, "配置文件不是合法 JSON")
	defer func() { testFake.err = nil }()

	router, _ := newAgentTestRouter(t)
	rec := putJSON(router, "/api/agents/zz-fake/models", `{"patches":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4402 {
		t.Errorf("期望业务码 4402,实际 %d", code)
	}
	rec, env := getJSON(router, "/api/agents/zz-fake/models")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if env["code"].(float64) != 4402 {
		t.Errorf("GET models 期望业务码 4402,实际 %v", env["code"])
	}
}

// TestAgentRestore_语义 还原已注册但无备份的工具报 4404(配置缺失或无备份
// 两个分支均为 4404);还原未知名称报 4401。
func TestAgentRestore_语义(t *testing.T) {
	router, _ := newAgentTestRouter(t)
	rec := postJSON(router, "/api/agents/workbuddy/restore", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 HTTP 404,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4404 {
		t.Errorf("还原无备份的已注册工具应报 4404,实际 %d", code)
	}

	rec = postJSON(router, "/api/agents/nope/restore", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4401 {
		t.Errorf("还原未知名称应报 4401,实际 %d", code)
	}
}

// TestAgentList_包含全部注册工具 列表按名称排序返回且信封为成功。
func TestAgentList_包含全部注册工具(t *testing.T) {
	registerFakeAgent(t)
	router, _ := newAgentTestRouter(t)
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
	// 快照已瘦身为列描述,不再携带 values
	if _, exists := first["values"]; exists {
		t.Error("快照不应再返回 values 字段")
	}
	if _, exists := first["columns"]; !exists {
		t.Error("快照应返回 columns 字段")
	}
}
