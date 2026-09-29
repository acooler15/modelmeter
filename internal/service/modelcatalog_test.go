package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
)

// newListModelsServer 起一个返回指定状态码与响应体的假上游,测试结束自动关闭。
func newListModelsServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestListModels_成功解析与字段缺失降级 正常条目完整解析;缺字条目按零值展示;
// 缺 id 或非对象条目跳过;Raw 保留上游原始内容。
func TestListModels_成功解析与字段缺失降级(t *testing.T) {
	db := newProviderTestDB(t)
	body := `{"object":"list","data":[
		{"id":"gpt-4","object":"model","owned_by":"openai","created":1687882411,"custom":"x"},
		{"id":"bare"},
		{"object":"model","owned_by":"no-id"},
		"not-an-object"
	]}`
	srv := newListModelsServer(t, http.StatusOK, body)
	p := model.Provider{Name: "假上游", BaseURL: srv.URL, APIKey: "sk-abcdef123456"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("插入测试配置失败: %v", err)
	}

	models, err := ListModels(context.Background(), db, p.ID)
	if err != nil {
		t.Fatalf("拉取模型列表应成功,实际报错: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("期望 2 条可用模型(缺 id/非对象跳过),实际 %d 条: %+v", len(models), models)
	}
	first := models[0]
	if first.ID != "gpt-4" || first.Object != "model" || first.OwnedBy != "openai" || first.Created != 1687882411 {
		t.Fatalf("首条字段解析不符: %+v", first)
	}
	if !strings.Contains(string(first.Raw), `"custom":"x"`) {
		t.Fatalf("Raw 应保留上游原始条目,实际 %s", first.Raw)
	}
	if models[1].ID != "bare" || models[1].OwnedBy != "" || models[1].Created != 0 {
		t.Fatalf("缺字条目应按零值降级展示: %+v", models[1])
	}
}

// TestListModels_配置不存在报1404 查询不存在的配置 ID 返回 1404。
func TestListModels_配置不存在报1404(t *testing.T) {
	db := newProviderTestDB(t)
	_, err := ListModels(context.Background(), db, 9999)
	wantCode(t, err, apperr.CodeProviderNotFound)
}

// TestListModels_上游鉴权失败透传1501 上游 401 透传为 1501。
func TestListModels_上游鉴权失败透传1501(t *testing.T) {
	db := newProviderTestDB(t)
	srv := newListModelsServer(t, http.StatusUnauthorized, `{"error":"bad key"}`)
	p := model.Provider{Name: "假上游", BaseURL: srv.URL, APIKey: "sk-abcdef123456"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("插入测试配置失败: %v", err)
	}
	_, err := ListModels(context.Background(), db, p.ID)
	wantCode(t, err, apperr.CodeUpstreamAuth)
}

// TestListModels_连接失败报1500 上游不可达(拒连)归为 1500。
func TestListModels_连接失败报1500(t *testing.T) {
	db := newProviderTestDB(t)
	closed := newListModelsServer(t, http.StatusOK, "{}")
	closedURL := closed.URL
	closed.Close()
	p := model.Provider{Name: "失效上游", BaseURL: closedURL, APIKey: "sk-abcdef123456"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("插入测试配置失败: %v", err)
	}
	_, err := ListModels(context.Background(), db, p.ID)
	wantCode(t, err, apperr.CodeUpstreamNetwork)
}

// TestListModels_结构异常报1502 data 缺失与非数组分别触发 1502 格式异常。
func TestListModels_结构异常报1502(t *testing.T) {
	db := newProviderTestDB(t)
	cases := []struct {
		name string
		body string
	}{
		{"data 字段缺失", `{"object":"list"}`},
		{"data 不是数组", `{"data":"oops"}`},
	}
	for _, c := range cases {
		srv := newListModelsServer(t, http.StatusOK, c.body)
		p := model.Provider{Name: "假上游-" + c.name, BaseURL: srv.URL, APIKey: "sk-abcdef123456"}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("插入测试配置失败: %v", err)
		}
		_, err := ListModels(context.Background(), db, p.ID)
		wantCode(t, err, apperr.CodeUpstreamBadResponse)
	}
}

// TestListModels_空列表是合法结果 data 为空数组时返回空切片而非报错。
func TestListModels_空列表是合法结果(t *testing.T) {
	db := newProviderTestDB(t)
	srv := newListModelsServer(t, http.StatusOK, `{"object":"list","data":[]}`)
	p := model.Provider{Name: "空上游", BaseURL: srv.URL, APIKey: "sk-abcdef123456"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("插入测试配置失败: %v", err)
	}
	models, err := ListModels(context.Background(), db, p.ID)
	if err != nil {
		t.Fatalf("空列表不应报错,实际: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("期望空列表,实际 %d 条", len(models))
	}
}
