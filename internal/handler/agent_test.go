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
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
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

// fakeAgent 假实现:验证 models 相关端点的路由、信封与参数透传,
// 不触碰本机真实配置文件。命名为 zz-fake 保证排序在真实工具之后。
type fakeAgent struct {
	entries    []agentconf.ModelEntry
	err        error
	gotPatches []agentconf.ModelPatch
	gotRefs    []agentconf.ModelRef
	gotAddReq  agentconf.AddModelsRequest
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

func (f *fakeAgent) RemoveModels(_ context.Context, refs []agentconf.ModelRef, dataDir string) ([]agentconf.ModelEntry, error) {
	f.gotRefs = refs
	f.gotDataDir = dataDir
	return f.entries, f.err
}

// AddModels 除透传外按真实适配器契约校验 target.mode(路由级 4403 断言用,
// 适配器自身的完整校验由 agents 包单测覆盖)。
func (f *fakeAgent) AddModels(_ context.Context, req agentconf.AddModelsRequest, dataDir string) (agentconf.AddModelsResult, error) {
	f.gotAddReq = req
	f.gotDataDir = dataDir
	if req.Target.Mode != "" && req.Target.Mode != "existing" && req.Target.Mode != "new" {
		return agentconf.AddModelsResult{}, apperr.New(apperr.CodeAgentInvalid, "target.mode 非法,仅支持 existing 或 new")
	}
	return agentconf.AddModelsResult{Entries: f.entries, Added: req.ModelIDs}, f.err
}

// testFake 全局唯一的假实现实例,注册一次,用例各自改写其行为字段。
var (
	testFake = &fakeAgent{}
	fakeOnce sync.Once
)

// registerFakeAgent 把假实现注册进全局注册表(仅一次)。
func registerFakeAgent(t *testing.T) {
	t.Helper()
	fakeOnce.Do(func() {
		agentconf.Register(testFake)
		agentconf.Register(testFakeSetter)
	})
}

// fakeDefaultSetter 假实现变体:在 fakeAgent 之上额外实现 DefaultModelSetter,
// 验证默认模型端点的参数透传与响应信封。命名为 zz-fake-setter 保证排序靠后。
type fakeDefaultSetter struct {
	fakeAgent
	gotPatch   agentconf.DefaultModelPatch
	gotDataDir string
}

func (f *fakeDefaultSetter) Name() string        { return "zz-fake-setter" }
func (f *fakeDefaultSetter) DisplayName() string { return "假工具(默认模型)" }

func (f *fakeDefaultSetter) Snapshot(_ context.Context) agentconf.Snapshot {
	return agentconf.Snapshot{
		Name:                 f.Name(),
		DisplayName:          "假工具(默认模型)",
		Status:               agentconf.StatusFound,
		Columns:              []agentconf.FieldSpec{},
		SupportsDefaultModel: true,
	}
}

func (f *fakeDefaultSetter) ApplyDefaultModel(_ context.Context, patch agentconf.DefaultModelPatch, dataDir string) (agentconf.Snapshot, error) {
	f.gotPatch = patch
	f.gotDataDir = dataDir
	return f.Snapshot(context.Background()), f.err
}

// testFakeSetter 实现默认模型能力的假实现实例。
var testFakeSetter = &fakeDefaultSetter{}

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
	if len(data) < 3 {
		t.Fatalf("应包含 codebuddy、zcode 与 workbuddy 三个工具,实际 %d 个", len(data))
	}
	names := make(map[string]bool, len(data))
	for _, e := range data {
		if m, ok := e.(map[string]any); ok {
			names[m["name"].(string)] = true
		}
	}
	for _, want := range []string{"codebuddy", "workbuddy", "zcode"} {
		if !names[want] {
			t.Errorf("列表应包含工具 %s,实际 %v", want, names)
		}
	}
	first, _ := data[0].(map[string]any)
	if first["name"] != "codebuddy" {
		t.Errorf("列表应按名称排序(codebuddy 在前),实际 %v", first["name"])
	}
	// 快照已瘦身为列描述,不再携带 values
	if _, exists := first["values"]; exists {
		t.Error("快照不应再返回 values 字段")
	}
	if _, exists := first["columns"]; !exists {
		t.Error("快照应返回 columns 字段")
	}
}

// TestAgentDefaultModelSet_信封与参数透传 body 的 patch 原样传给实现,
// dataDir 使用路由装配时注入的目录,响应为实现返回的最新 Snapshot。
func TestAgentDefaultModelSet_信封与参数透传(t *testing.T) {
	registerFakeAgent(t)
	testFakeSetter.err = nil
	defer func() { testFakeSetter.err = nil }()

	router, dataDir := newAgentTestRouter(t)
	body := `{"provider_id":"p1","model_id":"m-1","reasoning_level":"high"}`
	rec := putJSON(router, "/api/agents/zz-fake-setter/default-model", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 0 {
		t.Fatalf("期望成功信封,实际 %d", code)
	}
	if testFakeSetter.gotPatch.ProviderID != "p1" || testFakeSetter.gotPatch.ModelID != "m-1" ||
		testFakeSetter.gotPatch.ReasoningLevel != "high" {
		t.Errorf("patch 内容透传错误,实际 %+v", testFakeSetter.gotPatch)
	}
	if testFakeSetter.gotDataDir != dataDir {
		t.Errorf("dataDir 应为路由注入的目录,实际 %q", testFakeSetter.gotDataDir)
	}
	// 响应 data 为实现返回的最新 Snapshot
	var env struct {
		Data agentconf.Snapshot `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	if env.Data.Name != "zz-fake-setter" || !env.Data.SupportsDefaultModel {
		t.Errorf("应返回最新 Snapshot,实际 %+v", env.Data)
	}
}

// TestAgentDefaultModelSet_未实现能力_4405 未实现 DefaultModelSetter 的工具
// (如 WorkBuddy 与无能力假实现)报 4405。
func TestAgentDefaultModelSet_未实现能力_4405(t *testing.T) {
	registerFakeAgent(t)
	router, _ := newAgentTestRouter(t)
	rec := putJSON(router, "/api/agents/zz-fake/default-model", `{"provider_id":"p1","model_id":"m-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4405 {
		t.Errorf("期望业务码 4405,实际 %d", code)
	}
	// 真实注册的 WorkBuddy 同样未实现该能力
	rec = putJSON(router, "/api/agents/workbuddy/default-model", `{"provider_id":"p1","model_id":"m-1"}`)
	if code := envelopeCode(t, rec); code != 4405 {
		t.Errorf("WorkBuddy 期望业务码 4405,实际 %d", code)
	}
}

// TestAgentDefaultModelSet_未知名称_4401 默认模型端点对未知名称报 4401。
func TestAgentDefaultModelSet_未知名称_4401(t *testing.T) {
	router, _ := newAgentTestRouter(t)
	rec := putJSON(router, "/api/agents/nope/default-model", `{"provider_id":"p1","model_id":"m-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4401 {
		t.Errorf("期望业务码 4401,实际 %d", code)
	}
}

// TestAgentDefaultModelSet_非法body_4403 请求体不是合法 JSON 时报 4403。
func TestAgentDefaultModelSet_非法body_4403(t *testing.T) {
	registerFakeAgent(t)
	router, _ := newAgentTestRouter(t)
	rec := putJSON(router, "/api/agents/zz-fake-setter/default-model", "{bad")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4403 {
		t.Errorf("期望业务码 4403,实际 %d", code)
	}
}

// newAgentAddTestEnv 内存 SQLite(预置一条接口记录 ID=1)+ 测试模式路由,
// 供 models/add 端点的装配断言使用。返回路由、库与 dataDir。
func newAgentAddTestEnv(t *testing.T, baseURL string) (*gin.Engine, *gorm.DB, string) {
	t.Helper()
	registerFakeAgent(t)
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Provider{}); err != nil {
		t.Fatalf("迁移测试相关表失败: %v", err)
	}
	if err := db.Create(&model.Provider{Name: "来源接口", BaseURL: baseURL, APIKey: "sk-agent-add-test"}).Error; err != nil {
		t.Fatalf("创建测试配置失败: %v", err)
	}
	dataDir := t.TempDir()
	return NewRouter(db, dataDir), db, dataDir
}

// TestAgentModelsRemove_信封与参数透传 body 的 targets 原样传给实现,
// dataDir 使用路由装配时注入的目录,响应为最新清单;空 targets 数组按
// 空切片透传(交由实现只读返回)。
func TestAgentModelsRemove_信封与参数透传(t *testing.T) {
	registerFakeAgent(t)
	testFake.err = nil
	testFake.entries = []agentconf.ModelEntry{{ProviderID: "p1", ModelID: "m-2", Fields: map[string]any{"enabled": true}}}
	defer func() { testFake.entries = nil; testFake.gotRefs = nil }()

	router, dataDir := newAgentTestRouter(t)
	body := `{"targets":[{"provider_id":"p1","model_id":"m-1"},{"model_id":"m-2"}]}`
	rec := postJSON(router, "/api/agents/zz-fake/models/remove", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 0 {
		t.Fatalf("期望成功信封,实际 %d", code)
	}
	if len(testFake.gotRefs) != 2 {
		t.Fatalf("应透传 2 个定位,实际 %d 个", len(testFake.gotRefs))
	}
	if testFake.gotRefs[0].ProviderID != "p1" || testFake.gotRefs[0].ModelID != "m-1" ||
		testFake.gotRefs[1].ProviderID != "" || testFake.gotRefs[1].ModelID != "m-2" {
		t.Errorf("定位内容透传错误,实际 %+v", testFake.gotRefs)
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
	if len(env.Data) != 1 || env.Data[0].ModelID != "m-2" {
		t.Errorf("应返回最新清单,实际 %+v", env.Data)
	}

	// 空 targets 数组:按空切片透传(空删除只读返回)
	rec = postJSON(router, "/api/agents/zz-fake/models/remove", `{"targets":[]}`)
	if code := envelopeCode(t, rec); code != 0 {
		t.Fatalf("空删除期望成功信封,实际 %d", code)
	}
	if testFake.gotRefs == nil || len(testFake.gotRefs) != 0 {
		t.Errorf("空 targets 应按空切片透传,实际 %+v", testFake.gotRefs)
	}
}

// TestAgentModelsRemove_未知名称与非法body 未知 agent 报 4401,请求体非法报 4403,
// 实现的业务错误按码透传。
func TestAgentModelsRemove_未知名称与非法body(t *testing.T) {
	registerFakeAgent(t)
	router, _ := newAgentTestRouter(t)

	rec := postJSON(router, "/api/agents/nope/models/remove", `{"targets":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4401 {
		t.Errorf("未知名称期望业务码 4401,实际 %d", code)
	}

	rec = postJSON(router, "/api/agents/zz-fake/models/remove", "{bad")
	if code := envelopeCode(t, rec); code != 4403 {
		t.Errorf("非法 body 期望业务码 4403,实际 %d", code)
	}

	testFake.err = apperr.New(apperr.CodeAgentFileIO, "配置文件不是合法 JSON")
	defer func() { testFake.err = nil }()
	rec = postJSON(router, "/api/agents/zz-fake/models/remove", `{"targets":[]}`)
	if code := envelopeCode(t, rec); code != 4402 {
		t.Errorf("实现错误期望按码透传 4402,实际 %d", code)
	}
}

// TestAgentModelsAdd_信封与参数透传 provider 记录装配 ModelSource:base_url
// 覆盖(trim 后非空才覆盖)、EndpointURL 由 BuildURL 派生(/v1 不重复)、
// APIKey 取自记录;target 缺省为零值;凭据不出现在响应中。
func TestAgentModelsAdd_信封与参数透传(t *testing.T) {
	router, db, dataDir := newAgentAddTestEnv(t, "https://record.example.com/v1")
	testFake.err = nil
	testFake.entries = []agentconf.ModelEntry{{ModelID: "m-1", Fields: map[string]any{}}}
	defer func() { testFake.gotAddReq = agentconf.AddModelsRequest{} }()

	// 带覆盖地址与完整 target
	body := `{"provider_id":1,"model_ids":["m-1","m-2"],"base_url":"https://override.example.com/",` +
		`"target":{"mode":"new","provider_name":"新供应商","api_type":"openai-responses"}}`
	rec := postJSON(router, "/api/agents/zz-fake/models/add", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200,实际 %d,body: %s", rec.Code, rec.Body.String())
	}
	if code := envelopeCode(t, rec); code != 0 {
		t.Fatalf("期望成功信封,实际 %d", code)
	}
	req := testFake.gotAddReq
	if len(req.ModelIDs) != 2 || req.ModelIDs[0] != "m-1" || req.ModelIDs[1] != "m-2" {
		t.Errorf("model_ids 透传错误,实际 %v", req.ModelIDs)
	}
	if req.Source.BaseURL != "https://override.example.com/" {
		t.Errorf("base_url 覆盖后应为请求值(trim 去空白),实际 %q", req.Source.BaseURL)
	}
	// BuildURL 归一:根地址拼出一段 /v1,不重复
	if req.Source.EndpointURL != "https://override.example.com/v1/chat/completions" {
		t.Errorf("EndpointURL 派生错误,实际 %q", req.Source.EndpointURL)
	}
	if req.Source.ProviderName != "来源接口" {
		t.Errorf("ProviderName 应取接口记录名,实际 %q", req.Source.ProviderName)
	}
	if req.Source.APIKey != "sk-agent-add-test" {
		t.Errorf("APIKey 应取自接口记录,实际 %q", req.Source.APIKey)
	}
	if req.Target.Mode != "new" || req.Target.ProviderName != "新供应商" || req.Target.APIType != "openai-responses" {
		t.Errorf("target 透传错误,实际 %+v", req.Target)
	}
	if testFake.gotDataDir != dataDir {
		t.Errorf("dataDir 应为路由注入的目录,实际 %q", testFake.gotDataDir)
	}
	// 响应 data 为 AddModelsResult(最新清单 + added)
	var env struct {
		Data agentconf.AddModelsResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	if len(env.Data.Entries) != 1 || len(env.Data.Added) != 2 {
		t.Errorf("应返回最新清单与 added,实际 %+v", env.Data)
	}
	if strings.Contains(rec.Body.String(), "sk-agent-add-test") {
		t.Error("响应体不得包含凭据内容")
	}

	// 不带 base_url 与 target:回退记录值,target 为零值
	rec = postJSON(router, "/api/agents/zz-fake/models/add", `{"provider_id":1,"model_ids":["m-9"]}`)
	if code := envelopeCode(t, rec); code != 0 {
		t.Fatalf("缺省 base_url/target 期望成功信封,实际 %d", code)
	}
	req = testFake.gotAddReq
	if req.Source.BaseURL != "https://record.example.com/v1" {
		t.Errorf("未覆盖时 BaseURL 应取记录值,实际 %q", req.Source.BaseURL)
	}
	if req.Source.EndpointURL != "https://record.example.com/v1/chat/completions" {
		t.Errorf("记录值含 /v1 时 EndpointURL 不应重复拼接,实际 %q", req.Source.EndpointURL)
	}
	if req.Target != (agentconf.AddTargetSpec{}) {
		t.Errorf("target 缺省应为零值,实际 %+v", req.Target)
	}
	_ = db
}

// TestAgentModelsAdd_provider不存在_1404 记录不存在(含 provider_id 缺省 0)
// 时报 1404,适配器不得被调用。
func TestAgentModelsAdd_provider不存在_1404(t *testing.T) {
	router, _, _ := newAgentAddTestEnv(t, "https://record.example.com")

	for name, body := range map[string]string{
		"ID 不存在": `{"provider_id":999,"model_ids":["m-1"]}`,
		"ID 为 0": `{"model_ids":["m-1"]}`,
	} {
		rec := postJSON(router, "/api/agents/zz-fake/models/add", body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: 期望 HTTP 404,实际 %d", name, rec.Code)
		}
		if code := envelopeCode(t, rec); code != 1404 {
			t.Errorf("%s: 期望业务码 1404,实际 %d", name, code)
		}
	}
	if testFake.gotAddReq.Source.ProviderName != "" {
		t.Errorf("装配失败时不得调用适配器,实际 %+v", testFake.gotAddReq)
	}
}

// TestAgentModelsAdd_model_ids空_4403 model_ids 为空报 4403。
func TestAgentModelsAdd_model_ids空_4403(t *testing.T) {
	router, _, _ := newAgentAddTestEnv(t, "https://record.example.com")

	rec := postJSON(router, "/api/agents/zz-fake/models/add", `{"provider_id":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if code := envelopeCode(t, rec); code != 4403 {
		t.Errorf("期望业务码 4403,实际 %d", code)
	}
}

// TestAgentModelsAdd_未知agent与非法target 未知 agent 报 4401;
// target.mode 非法由实现报 4403 透传。
func TestAgentModelsAdd_未知agent与非法target(t *testing.T) {
	router, _, _ := newAgentAddTestEnv(t, "https://record.example.com")

	rec := postJSON(router, "/api/agents/nope/models/add", `{"provider_id":1,"model_ids":["m-1"]}`)
	if code := envelopeCode(t, rec); code != 4401 {
		t.Errorf("未知名称期望业务码 4401,实际 %d", code)
	}

	rec = postJSON(router, "/api/agents/zz-fake/models/add",
		`{"provider_id":1,"model_ids":["m-1"],"target":{"mode":"clone"}}`)
	if code := envelopeCode(t, rec); code != 4403 {
		t.Errorf("非法 target 期望业务码 4403,实际 %d", code)
	}

	rec = postJSON(router, "/api/agents/zz-fake/models/add", "{bad")
	if code := envelopeCode(t, rec); code != 4403 {
		t.Errorf("非法 body 期望业务码 4403,实际 %d", code)
	}
}
