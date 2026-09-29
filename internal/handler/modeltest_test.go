package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/model"
)

// newHandlerTestEnv 内存 SQLite + 测试模式路由;假上游地址写入唯一的接口配置(ID=1)。
func newHandlerTestEnv(t *testing.T, upstreamURL string) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Provider{}, &model.TestRecord{}); err != nil {
		t.Fatalf("迁移测试相关表失败: %v", err)
	}
	if err := db.Create(&model.Provider{Name: "假配置", BaseURL: upstreamURL, APIKey: "sk-handler-test"}).Error; err != nil {
		t.Fatalf("创建测试配置失败: %v", err)
	}
	return NewRouter(db), db
}

// sseFrames 把 SSE 响应体按空行切帧,去掉 "data: " 前缀后反序列化为 map。
func sseFrames(t *testing.T, body string) []map[string]any {
	t.Helper()
	var frames []map[string]any
	for _, frame := range strings.Split(strings.TrimRight(body, "\n"), "\n\n") {
		if !strings.HasPrefix(frame, "data: ") {
			t.Fatalf("SSE 帧应以 \"data: \" 开头,实际 %q", frame)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &m); err != nil {
			t.Fatalf("SSE 帧数据不是合法 JSON: %v", err)
		}
		frames = append(frames, m)
	}
	return frames
}

// postJSON 向路由发起带 JSON 请求体的 POST。
func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestTestStream_流式SSE帧格式 校验 delta/done 两类事件的帧格式与内容,
// 以及响应头为 text/event-stream。
func TestTestStream_流式SSE帧格式(t *testing.T) {
	// 假上游:OpenAI Chat Completions 流式响应
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()
	router, db := newHandlerTestEnv(t, upstream.URL)

	rec := postJSON(router, "/api/test/stream",
		`{"provider_id":1,"protocol":"chat_completions","model":"gpt-test","user":"hi","stream":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200,实际 %d,body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("期望 text/event-stream,实际 %q", ct)
	}
	frames := sseFrames(t, rec.Body.String())
	if len(frames) != 3 {
		t.Fatalf("期望 3 帧(2 delta + 1 done),实际 %d 帧: %s", len(frames), rec.Body.String())
	}
	if frames[0]["type"] != "delta" || frames[0]["text"] != "你" {
		t.Errorf("首帧应为内容增量,实际 %v", frames[0])
	}
	if frames[1]["type"] != "delta" || frames[1]["text"] != "好" {
		t.Errorf("第二帧应为内容增量,实际 %v", frames[1])
	}
	done := frames[2]
	if done["type"] != "done" {
		t.Fatalf("末帧应为 done,实际 %v", done)
	}
	result, _ := done["result"].(map[string]any)
	if result["reply"] != "你好" {
		t.Errorf("done.result.reply 应为增量拼接,实际 %v", result["reply"])
	}
	record, _ := done["record"].(map[string]any)
	if record["succeeded"] != true || record["stream"] != true {
		t.Errorf("done.record 应为成功流式记录,实际 %v", record)
	}
	// 记录落库校验:流式测试应留下一条成功记录
	var count int64
	db.Model(&model.TestRecord{}).Where("succeeded = ? AND stream = ?", true, true).Count(&count)
	if count != 1 {
		t.Errorf("流式测试应落 1 条成功记录,实际 %d", count)
	}
}

// TestTestStream_开始推送前失败走信封 上游 401 时未推送任何增量,
// 响应应为统一错误信封(HTTP 400 + 业务码 1501)。
func TestTestStream_开始推送前失败走信封(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()
	router, _ := newHandlerTestEnv(t, upstream.URL)

	rec := postJSON(router, "/api/test/stream",
		`{"provider_id":1,"protocol":"chat_completions","model":"gpt-test","user":"hi","stream":true}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400,实际 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("失败时应返回 JSON 信封,实际 %q", ct)
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("信封解析失败: %v", err)
	}
	if env.Code != 1501 || env.Message != "鉴权失败,请检查 API Key" {
		t.Errorf("期望 1501 鉴权失败文案,实际 %+v", env)
	}
}

// TestTestRun_信封与记录落库 非流式成功返回 {result, record},记录可查询、可清空。
func TestTestRun_信封与记录落库(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()
	router, _ := newHandlerTestEnv(t, upstream.URL)

	rec := postJSON(router, "/api/test",
		`{"provider_id":1,"protocol":"chat_completions","model":"gpt-test","user":"hi"}`)
	var env struct {
		Code int `json:"code"`
		Data struct {
			Result struct {
				Reply string `json:"reply"`
			} `json:"result"`
			Record struct {
				ID        uint   `json:"id"`
				Succeeded bool   `json:"succeeded"`
				Err       string `json:"err_message"`
			} `json:"record"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("信封解析失败: %v", err)
	}
	if env.Code != 0 || env.Data.Result.Reply != "ok" || !env.Data.Record.Succeeded {
		t.Fatalf("非流式测试应成功并携带 result/record,实际 %s", rec.Body.String())
	}

	// 记录查询
	req := httptest.NewRequest(http.MethodGet, "/api/test/records", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var listEnv struct {
		Code int `json:"code"`
		Data []struct {
			ID     uint   `json:"id"`
			Reply  string `json:"reply"`
			Stream bool   `json:"stream"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("记录信封解析失败: %v", err)
	}
	if listEnv.Code != 0 || len(listEnv.Data) != 1 || listEnv.Data[0].Reply != "ok" {
		t.Fatalf("记录查询应返回 1 条成功记录,实际 %s", rec.Body.String())
	}

	// 参数缺失返回 2401
	rec = postJSON(router, "/api/test", `{"provider_id":1,"protocol":"chat_completions","model":" ","user":"hi"}`)
	var badEnv struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &badEnv); err != nil {
		t.Fatalf("错误信封解析失败: %v", err)
	}
	if badEnv.Code != 2401 {
		t.Errorf("模型为空应返回 2401,实际 %d", badEnv.Code)
	}

	// 清空记录
	delReq := httptest.NewRequest(http.MethodDelete, "/api/test/records", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, delReq)
	var delEnv struct {
		Code int `json:"code"`
		Data struct {
			Deleted int64 `json:"deleted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &delEnv); err != nil {
		t.Fatalf("清空信封解析失败: %v", err)
	}
	if delEnv.Code != 0 || delEnv.Data.Deleted != 1 {
		t.Errorf("清空应返回 deleted=1,实际 %s", rec.Body.String())
	}
}
