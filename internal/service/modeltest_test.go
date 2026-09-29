package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
	"github.com/acooler15/modelmeter/internal/service/llmclient"
)

// newModelTestDB 打开内存 SQLite 并迁移测试相关表,供各用例复用。
func newModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Provider{}, &model.TestRecord{}); err != nil {
		t.Fatalf("迁移测试相关表失败: %v", err)
	}
	return db
}

// newTestProvider 在库中创建指向假上游的接口配置,返回其 ID。
func newTestProvider(t *testing.T, db *gorm.DB, baseURL string) uint {
	t.Helper()
	p := model.Provider{Name: "测试配置", BaseURL: baseURL, APIKey: "sk-record-check"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("创建测试配置失败: %v", err)
	}
	return p.ID
}

// newChatFakeUpstream 返回一个 OpenAI Chat Completions 形态的假上游,
// replier 决定响应内容,便于分别构造成功与失败场景。
func newChatFakeUpstream(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// chatInput 构造一份合法的测试输入,指向指定配置。
func chatInput(providerID uint) TestInput {
	return TestInput{
		ProviderID: providerID,
		ChatRequest: llmclient.ChatRequest{
			Protocol: llmclient.ProtocolChatCompletions, Model: "gpt-test", System: "sys", User: "hi",
		},
	}
}

// TestRunTest_成功落库 全量测试成功后应留下带回复、计时与用量的成功记录。
func TestRunTest_成功落库(t *testing.T) {
	db := newModelTestDB(t)
	srv := newChatFakeUpstream(t, http.StatusOK,
		`{"choices":[{"message":{"content":"你好"}}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`)
	defer srv.Close()
	providerID := newTestProvider(t, db, srv.URL)

	result, record, err := RunTest(context.Background(), db, chatInput(providerID))
	if err != nil {
		t.Fatalf("RunTest 应成功,实际报错: %v", err)
	}
	if result.Reply != "你好" {
		t.Errorf("回复解析错误,实际 %q", result.Reply)
	}
	if record == nil || record.ID == 0 {
		t.Fatalf("期望记录已落库,实际 %+v", record)
	}
	if !record.Succeeded || record.Reply != "你好" || record.ErrMessage != "" {
		t.Errorf("成功记录内容错误,实际 %+v", record)
	}
	if record.ProviderName != "测试配置" || record.Protocol != "chat_completions" || record.Model != "gpt-test" {
		t.Errorf("记录配置快照错误,实际 %+v", record)
	}
	if record.PromptTokens != 3 || record.CompletionTokens != 1 || record.TotalTokens != 4 {
		t.Errorf("记录 token 用量错误,实际 %+v", record)
	}
	if record.FirstLatencyMs < 0 || record.TotalLatencyMs < record.FirstLatencyMs {
		t.Errorf("记录计时不合理: %+v", record)
	}
	// 请求参数应原样留档,且不出现任何 API Key
	if record.SystemPrompt != "sys" || record.UserMessage != "hi" || record.Stream {
		t.Errorf("记录参数快照错误,实际 %+v", record)
	}
	var count int64
	db.Model(&model.TestRecord{}).Count(&count)
	if count != 1 {
		t.Errorf("期望库中恰好 1 条记录,实际 %d", count)
	}
}

// TestRunTest_失败也落库 上游 401 时返回 1501,同时留下带中文失败原因的记录。
func TestRunTest_失败也落库(t *testing.T) {
	db := newModelTestDB(t)
	srv := newChatFakeUpstream(t, http.StatusUnauthorized, `{"error":{"message":"invalid key"}}`)
	defer srv.Close()
	providerID := newTestProvider(t, db, srv.URL)

	_, record, err := RunTest(context.Background(), db, chatInput(providerID))
	wantCode(t, err, apperr.CodeUpstreamAuth)
	if record == nil || record.Succeeded {
		t.Fatalf("期望失败记录已落库,实际 %+v", record)
	}
	if record.ErrMessage != "鉴权失败,请检查 API Key" {
		t.Errorf("失败原因应为中文文案,实际 %q", record.ErrMessage)
	}
	if record.Reply != "" || record.TotalTokens != 0 {
		t.Errorf("失败记录不应携带回复与用量,实际 %+v", record)
	}
}

// TestRunTest_校验失败不落库 参数缺失时报 2401,且不产生记录。
func TestRunTest_校验失败不落库(t *testing.T) {
	db := newModelTestDB(t)
	cases := map[string]TestInput{
		"未选配置": {ProviderID: 0, ChatRequest: llmclient.ChatRequest{Protocol: llmclient.ProtocolChatCompletions, Model: "m", User: "hi"}},
		"模型为空": {ProviderID: 1, ChatRequest: llmclient.ChatRequest{Protocol: llmclient.ProtocolChatCompletions, Model: " ", User: "hi"}},
		"消息为空": {ProviderID: 1, ChatRequest: llmclient.ChatRequest{Protocol: llmclient.ProtocolChatCompletions, Model: "m", User: ""}},
		"协议非法": {ProviderID: 1, ChatRequest: llmclient.ChatRequest{Protocol: "nope", Model: "m", User: "hi"}},
	}
	for name, in := range cases {
		_, record, err := RunTest(context.Background(), db, in)
		wantCode(t, err, apperr.CodeTestInvalid)
		if record != nil {
			t.Errorf("%s: 校验失败不应产生记录,实际 %+v", name, record)
		}
	}
	var count int64
	db.Model(&model.TestRecord{}).Count(&count)
	if count != 0 {
		t.Errorf("校验失败不应落库,实际 %d 条", count)
	}
}

// TestRunTest_配置不存在不落库 报 1404 且不产生记录。
func TestRunTest_配置不存在不落库(t *testing.T) {
	db := newModelTestDB(t)
	_, record, err := RunTest(context.Background(), db, chatInput(999))
	wantCode(t, err, apperr.CodeProviderNotFound)
	if record != nil {
		t.Errorf("配置不存在不应产生记录,实际 %+v", record)
	}
}

// TestStreamTest_流式成功落库 流式测试应转发增量、拼接回复并留下 Stream 标记记录。
func TestStreamTest_流式成功落库(t *testing.T) {
	db := newModelTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()
	providerID := newTestProvider(t, db, srv.URL)

	var deltas []string
	result, record, err := StreamTest(context.Background(), db, chatInput(providerID), func(text string) {
		deltas = append(deltas, text)
	})
	if err != nil {
		t.Fatalf("StreamTest 应成功,实际报错: %v", err)
	}
	if len(deltas) != 2 || deltas[0] != "你" || deltas[1] != "好" {
		t.Errorf("增量转发错误,实际 %v", deltas)
	}
	if result.Reply != "你好" {
		t.Errorf("流式回复应为增量拼接,实际 %q", result.Reply)
	}
	if record == nil || !record.Succeeded || !record.Stream || record.Reply != "你好" {
		t.Errorf("流式成功记录错误,实际 %+v", record)
	}
	if record.TotalTokens != 4 {
		t.Errorf("流式记录用量错误,实际 %+v", record)
	}
}

// TestStreamTest_推送中途失败落库 流中错误事件应返回 2500,并把失败记录落库。
func TestStreamTest_推送中途失败落库(t *testing.T) {
	db := newModelTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"error\":{\"message\":\"配额不足\"}}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()
	providerID := newTestProvider(t, db, srv.URL)

	var deltas []string
	_, record, err := StreamTest(context.Background(), db, chatInput(providerID), func(text string) {
		deltas = append(deltas, text)
	})
	wantCode(t, err, apperr.CodeUpstreamRequestFailed)
	if len(deltas) != 1 {
		t.Errorf("出错前的增量应已转发,实际 %v", deltas)
	}
	if record == nil || record.Succeeded {
		t.Fatalf("期望失败记录已落库,实际 %+v", record)
	}
	if record.ErrMessage != "上游返回错误:配额不足" {
		t.Errorf("失败原因应带上游文案,实际 %q", record.ErrMessage)
	}
}

// TestTrimTestRecords_只保留最近200条 超限后按时间倒序保留最新 200 条。
func TestTrimTestRecords_只保留最近200条(t *testing.T) {
	db := newModelTestDB(t)
	base := time.Now().UTC()
	for i := 0; i < 205; i++ {
		r := model.TestRecord{
			ProviderName: fmt.Sprintf("p%03d", i),
			Model:        "m",
			Succeeded:    true,
			// 显式递增时间,避免依赖插入顺序判断新旧
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := db.Create(&r).Error; err != nil {
			t.Fatalf("造数失败: %v", err)
		}
	}
	trimTestRecords(context.Background(), db)

	var count int64
	db.Model(&model.TestRecord{}).Count(&count)
	if count != 200 {
		t.Fatalf("清理后应剩 200 条,实际 %d", count)
	}
	// 最早的 5 条(时间最小)应被清理,最新的一条必须保留
	var oldest, newest model.TestRecord
	if err := db.First(&oldest, 1).Error; err == nil {
		t.Errorf("最早的记录应被清理,实际仍在: %+v", oldest)
	}
	if err := db.Order("created_at DESC").First(&newest).Error; err != nil {
		t.Fatalf("查询最新记录失败: %v", err)
	}
	if newest.ProviderName != "p204" {
		t.Errorf("最新记录应保留,实际 %+v", newest)
	}
}

// TestRunTest_触发清理 插满 200 条历史记录后再测一次,总量应仍为 200
// (新记录入库、最旧被清),且最新记录就是本次非流式测试。
func TestRunTest_触发清理(t *testing.T) {
	db := newModelTestDB(t)
	srv := newChatFakeUpstream(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	defer srv.Close()
	providerID := newTestProvider(t, db, srv.URL)
	// 种子时间全部放在过去(200~1 分钟前),保证本次测试记录是最新一条
	base := time.Now().UTC()
	for i := 0; i < 200; i++ {
		r := model.TestRecord{Model: "m", Succeeded: true, CreatedAt: base.Add(-time.Duration(200-i) * time.Minute)}
		if err := db.Create(&r).Error; err != nil {
			t.Fatalf("造数失败: %v", err)
		}
	}
	if _, _, err := RunTest(context.Background(), db, chatInput(providerID)); err != nil {
		t.Fatalf("RunTest 应成功,实际报错: %v", err)
	}
	var count int64
	db.Model(&model.TestRecord{}).Count(&count)
	if count != 200 {
		t.Errorf("清理后应仍为 200 条,实际 %d", count)
	}
	var newest model.TestRecord
	if err := db.Order("created_at DESC, id DESC").First(&newest).Error; err != nil {
		t.Fatalf("查询最新记录失败: %v", err)
	}
	if newest.Stream || newest.Reply != "ok" {
		t.Errorf("最新记录应为本次非流式测试(stream=false),实际 %+v", newest)
	}
}

// TestListTestRecords_倒序与上限 记录按创建时间倒序返回,limit 生效。
func TestListTestRecords_倒序与上限(t *testing.T) {
	db := newModelTestDB(t)
	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		r := model.TestRecord{ProviderName: fmt.Sprintf("p%d", i), Model: "m", Succeeded: true, CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		if err := db.Create(&r).Error; err != nil {
			t.Fatalf("造数失败: %v", err)
		}
	}
	records, err := ListTestRecords(context.Background(), db, 2)
	if err != nil {
		t.Fatalf("ListTestRecords 应成功,实际报错: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("limit=2 应返回 2 条,实际 %d", len(records))
	}
	if records[0].ProviderName != "p2" || records[1].ProviderName != "p1" {
		t.Errorf("应按创建时间倒序,实际 %s -> %s", records[0].ProviderName, records[1].ProviderName)
	}
}

// TestClearTestRecords_清空 清空后返回删除条数且表为空。
func TestClearTestRecords_清空(t *testing.T) {
	db := newModelTestDB(t)
	for i := 0; i < 2; i++ {
		if err := db.Create(&model.TestRecord{Model: "m", Succeeded: true}).Error; err != nil {
			t.Fatalf("造数失败: %v", err)
		}
	}
	deleted, err := ClearTestRecords(context.Background(), db)
	if err != nil {
		t.Fatalf("ClearTestRecords 应成功,实际报错: %v", err)
	}
	if deleted != 2 {
		t.Errorf("期望删除 2 条,实际 %d", deleted)
	}
	var count int64
	db.Model(&model.TestRecord{}).Count(&count)
	if count != 0 {
		t.Errorf("清空后应为 0 条,实际 %d", count)
	}
}

// TestRunTest_记录不含APIKey 成功与失败记录序列化结果都不应出现 API Key 明文。
func TestRunTest_记录不含APIKey(t *testing.T) {
	db := newModelTestDB(t)
	srv := newChatFakeUpstream(t, http.StatusUnauthorized, `{}`)
	defer srv.Close()
	providerID := newTestProvider(t, db, srv.URL)

	_, _, _ = RunTest(context.Background(), db, chatInput(providerID)) // 失败也落库,错误预期在下方断言
	var records []model.TestRecord
	if err := db.Find(&records).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	for _, r := range records {
		data, _ := json.Marshal(r)
		if bytes.Contains(data, []byte("sk-record-check")) {
			t.Errorf("记录泄露 API Key: %s", data)
		}
	}
}
