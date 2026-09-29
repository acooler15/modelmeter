package service

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
)

// newNewAPITestDB 打开内存 SQLite 并迁移 settings 表,供各用例复用。
func newNewAPITestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("迁移 settings 表失败: %v", err)
	}
	return db
}

// saveTestConfig 直接写入一条 New API 配置(settings JSON),绕过保存校验,
// 供拉取/估算类用例快速构造「已配置」状态。
func saveTestConfig(t *testing.T, db *gorm.DB, baseURL, token string) {
	t.Helper()
	data, err := json.Marshal(storedConfig{BaseURL: baseURL, Token: token})
	if err != nil {
		t.Fatalf("序列化测试配置失败: %v", err)
	}
	if err := db.Create(&model.Setting{Key: newAPISettingKey, Value: string(data)}).Error; err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
}

// newRatesServer 起一个返回指定状态码与响应体的假 New API,测试结束自动关闭。
func newRatesServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// assertFloatEqual 断言浮点数在容差内相等,避免除法结果的表示误差误报。
func assertFloatEqual(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v,期望 %v", name, got, want)
	}
}

// TestSaveNewAPIConfig_校验与首次必填令牌 覆盖地址为空、地址非法与
// 首次保存令牌留空三类 3401 校验。
func TestSaveNewAPIConfig_校验与首次必填令牌(t *testing.T) {
	db := newNewAPITestDB(t)
	cases := []struct {
		name string
		in   NewAPIConfigInput
	}{
		{"地址为空", NewAPIConfigInput{BaseURL: "  ", Token: "tok"}},
		{"地址缺少 scheme", NewAPIConfigInput{BaseURL: "newapi.example.com", Token: "tok"}},
		{"地址协议不是 http(s)", NewAPIConfigInput{BaseURL: "ftp://newapi.example.com", Token: "tok"}},
		{"地址缺少主机", NewAPIConfigInput{BaseURL: "http://", Token: "tok"}},
		{"首次保存令牌为空", NewAPIConfigInput{BaseURL: "https://newapi.example.com", Token: " "}},
	}
	for _, c := range cases {
		_, err := SaveNewAPIConfig(context.Background(), db, c.in)
		wantCode(t, err, apperr.CodeNewAPINotConfigured)
	}
}

// TestSaveNewAPIConfig_留空沿用原令牌 已有配置时令牌留空沿用原值、填写则替换;
// 地址允许单独更新。
func TestSaveNewAPIConfig_留空沿用原令牌(t *testing.T) {
	db := newNewAPITestDB(t)
	ctx := context.Background()

	saved, err := SaveNewAPIConfig(ctx, db, NewAPIConfigInput{
		BaseURL: "https://old.example.com/", Token: "sk-original-token-1234",
	})
	if err != nil {
		t.Fatalf("首次保存应成功,实际报错: %v", err)
	}
	if saved.TokenMasked != MaskKey("sk-original-token-1234") {
		t.Errorf("保存视图应携带脱敏令牌,实际 %q", saved.TokenMasked)
	}

	// 令牌留空:沿用原令牌,仅更新地址(地址含尾斜杠时归一化交给保存值本身)
	if _, err := SaveNewAPIConfig(ctx, db, NewAPIConfigInput{
		BaseURL: "https://new.example.com", Token: "",
	}); err != nil {
		t.Fatalf("留空令牌保存应成功,实际报错: %v", err)
	}
	stored, found, err := readStoredConfig(ctx, db)
	if err != nil || !found {
		t.Fatalf("配置应存在且可读,found=%v err=%v", found, err)
	}
	if stored.BaseURL != "https://new.example.com" {
		t.Errorf("地址应更新为新值,实际 %q", stored.BaseURL)
	}
	if stored.Token != "sk-original-token-1234" {
		t.Errorf("令牌应沿用原值,实际被改为 %q", stored.Token)
	}

	// 令牌填写:替换原令牌
	if _, err := SaveNewAPIConfig(ctx, db, NewAPIConfigInput{
		BaseURL: "https://new.example.com", Token: "sk-replaced-9876",
	}); err != nil {
		t.Fatalf("替换令牌保存应成功,实际报错: %v", err)
	}
	stored, _, _ = readStoredConfig(ctx, db)
	if stored.Token != "sk-replaced-9876" {
		t.Errorf("令牌应替换为新值,实际 %q", stored.Token)
	}
}

// TestGetNewAPIConfig_脱敏视图 未保存时 configured=false;保存后只回脱敏令牌,
// 视图任何字段不包含明文。
func TestGetNewAPIConfig_脱敏视图(t *testing.T) {
	db := newNewAPITestDB(t)
	ctx := context.Background()

	view, err := GetNewAPIConfig(ctx, db)
	if err != nil {
		t.Fatalf("未保存时查询应成功,实际报错: %v", err)
	}
	if view.Configured {
		t.Error("未保存配置时 configured 应为 false")
	}

	token := "sk-secret-token-8888"
	if _, err := SaveNewAPIConfig(ctx, db, NewAPIConfigInput{
		BaseURL: "https://newapi.example.com", Token: token,
	}); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}
	view, err = GetNewAPIConfig(ctx, db)
	if err != nil {
		t.Fatalf("查询配置失败: %v", err)
	}
	if !view.Configured || view.BaseURL != "https://newapi.example.com" {
		t.Errorf("视图字段不符: %+v", view)
	}
	if view.TokenMasked != MaskKey(token) {
		t.Errorf("令牌应为脱敏值,实际 %q", view.TokenMasked)
	}
	if strings.Contains(view.TokenMasked, token) {
		t.Errorf("视图泄露明文令牌: %q", view.TokenMasked)
	}
}

// TestListRates_成功解析与缺字段降级 正常条目完整解析;缺字条目按零值保留;
// model_name 为空与非对象条目跳过;未知字段被容忍;结果按模型名排序。
func TestListRates_成功解析与缺字段降级(t *testing.T) {
	db := newNewAPITestDB(t)
	body := `{"data":[
		{"model_name":"b-model","quota_type":0,"model_ratio":0.002,"completion_ratio":3,"cache_ratio":0.5,"group":"default"},
		{"model_name":"a-model","quota_type":1,"model_price":0.02},
		{"quota_type":0,"model_ratio":1},
		"not-an-object"
	]}`
	srv := newRatesServer(t, http.StatusOK, body)
	saveTestConfig(t, db, srv.URL, "sk-rates-token-1234")

	entries, err := ListRates(context.Background(), db)
	if err != nil {
		t.Fatalf("拉取费率表应成功,实际报错: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("期望 2 条可用条目(空名/非对象跳过),实际 %d 条: %+v", len(entries), entries)
	}
	// 结果按 model_name 排序:a-model 在前
	first, second := entries[0], entries[1]
	if first.ModelName != "a-model" || first.QuotaType != 1 {
		t.Errorf("首条应为 a-model(按次),实际 %+v", first)
	}
	assertFloatEqual(t, "a-model.model_price", first.ModelPrice, 0.02)
	if second.ModelName != "b-model" || second.QuotaType != 0 {
		t.Errorf("次条应为 b-model(倍率),实际 %+v", second)
	}
	assertFloatEqual(t, "b-model.model_ratio", second.ModelRatio, 0.002)
	assertFloatEqual(t, "b-model.completion_ratio", second.CompletionRatio, 3)
}

// TestListRates_未配置或配置不完整报3401 覆盖无配置、有地址无令牌两种情况。
func TestListRates_未配置或配置不完整报3401(t *testing.T) {
	db := newNewAPITestDB(t)
	_, err := ListRates(context.Background(), db)
	wantCode(t, err, apperr.CodeNewAPINotConfigured)

	// 只有地址没有令牌:视为配置不完整
	saveTestConfig(t, db, "https://newapi.example.com", "")
	_, err = ListRates(context.Background(), db)
	wantCode(t, err, apperr.CodeNewAPINotConfigured)
}

// TestListRates_鉴权失败报3501 上游 401 归类为 3501,文案不回显令牌。
func TestListRates_鉴权失败报3501(t *testing.T) {
	db := newNewAPITestDB(t)
	srv := newRatesServer(t, http.StatusUnauthorized, `{"error":"bad token"}`)
	saveTestConfig(t, db, srv.URL, "sk-rates-leak-check")

	_, err := ListRates(context.Background(), db)
	wantCode(t, err, apperr.CodeNewAPIAuth)
	// 令牌不允许出现在错误消息里
	if strings.Contains(err.Error(), "sk-rates-leak-check") {
		t.Errorf("错误消息泄露令牌: %v", err)
	}
}

// TestListRates_响应异常报3502 覆盖坏 JSON、data 缺失与 data 非数组三类。
func TestListRates_响应异常报3502(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"响应体不是 JSON", `<html>gateway</html>`},
		{"data 字段缺失", `{"object":"list"}`},
		{"data 不是数组", `{"data":"oops"}`},
	}
	for _, c := range cases {
		db := newNewAPITestDB(t)
		srv := newRatesServer(t, http.StatusOK, c.body)
		saveTestConfig(t, db, srv.URL, "sk-rates-token-1234")
		_, err := ListRates(context.Background(), db)
		wantCode(t, err, apperr.CodeNewAPIBadResponse)
	}
}

// TestListRates_连接失败报3500 上游不可达(拒连)归类为 3500。
func TestListRates_连接失败报3500(t *testing.T) {
	db := newNewAPITestDB(t)
	closed := newRatesServer(t, http.StatusOK, "{}")
	closedURL := closed.URL
	closed.Close()
	saveTestConfig(t, db, closedURL, "sk-rates-token-1234")

	_, err := ListRates(context.Background(), db)
	wantCode(t, err, apperr.CodeNewAPINetwork)
}

// TestEstimateCost_倍率型 按公式计算 quota 与 USD:quota = 0.002×100 + 0.002×3×50。
func TestEstimateCost_倍率型(t *testing.T) {
	db := newNewAPITestDB(t)
	srv := newRatesServer(t, http.StatusOK,
		`{"data":[{"model_name":"gpt-x","quota_type":0,"model_ratio":0.002,"completion_ratio":3}]}`)
	saveTestConfig(t, db, srv.URL, "sk-rates-token-1234")

	est, ok := EstimateCost(context.Background(), db, "gpt-x", 100, 50)
	if !ok {
		t.Fatal("命中倍率型费率应返回估算结果")
	}
	// quota = 0.002*100 + 0.002*3*50 = 0.2 + 0.3 = 0.5;USD = 0.5/500000
	assertFloatEqual(t, "quota", est.Quota, 0.5)
	assertFloatEqual(t, "usd", est.USD, 0.5/500000)
	if est.QuotaType != 0 || est.Model != "gpt-x" {
		t.Errorf("估算结果字段不符: %+v", est)
	}
	if !strings.Contains(est.Formula, "倍率") || !strings.Contains(est.Formula, "500000") {
		t.Errorf("倍率型口径说明应包含倍率与换算比,实际 %q", est.Formula)
	}
}

// TestEstimateCost_按次型 USD 取 model_price,quota 为 0。
func TestEstimateCost_按次型(t *testing.T) {
	db := newNewAPITestDB(t)
	srv := newRatesServer(t, http.StatusOK,
		`{"data":[{"model_name":"flat-model","quota_type":1,"model_price":0.02}]}`)
	saveTestConfig(t, db, srv.URL, "sk-rates-token-1234")

	est, ok := EstimateCost(context.Background(), db, "flat-model", 10, 20)
	if !ok {
		t.Fatal("命中按次型费率应返回估算结果")
	}
	assertFloatEqual(t, "usd", est.USD, 0.02)
	assertFloatEqual(t, "quota", est.Quota, 0)
	if !strings.Contains(est.Formula, "按次") {
		t.Errorf("按次型口径说明应包含「按次」,实际 %q", est.Formula)
	}
}

// TestEstimateCost_降级不外抛 覆盖未命中、未配置、拉取失败与零用量四类
// 降级场景:一律返回 false 且不返回错误。
func TestEstimateCost_降级不外抛(t *testing.T) {
	ctx := context.Background()

	// 未配置:直接降级
	emptyDB := newNewAPITestDB(t)
	if _, ok := EstimateCost(ctx, emptyDB, "gpt-x", 10, 10); ok {
		t.Error("未配置时应返回 available=false")
	}

	// 拉取失败(鉴权失败):降级且不外抛
	failDB := newNewAPITestDB(t)
	srv := newRatesServer(t, http.StatusUnauthorized, `{"error":"bad token"}`)
	saveTestConfig(t, failDB, srv.URL, "sk-rates-token-1234")
	estimate, ok := EstimateCost(ctx, failDB, "gpt-x", 10, 10)
	if ok || estimate != (CostEstimate{}) {
		t.Errorf("拉取失败应返回零值估算,实际 ok=%v estimate=%+v", ok, estimate)
	}

	// 未命中费率表
	missDB := newNewAPITestDB(t)
	missSrv := newRatesServer(t, http.StatusOK,
		`{"data":[{"model_name":"other-model","quota_type":0,"model_ratio":1}]}`)
	saveTestConfig(t, missDB, missSrv.URL, "sk-rates-token-1234")
	if _, ok := EstimateCost(ctx, missDB, "gpt-x", 10, 10); ok {
		t.Error("未命中费率表应返回 available=false")
	}

	// 零用量(上游未返回 usage)
	if _, ok := EstimateCost(ctx, missDB, "other-model", 0, 0); ok {
		t.Error("零用量应返回 available=false")
	}
}
