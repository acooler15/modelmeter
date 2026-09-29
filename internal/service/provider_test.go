package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
)

// newProviderTestDB 打开内存 SQLite 并迁移 providers 表,供各用例复用。
func newProviderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Provider{}); err != nil {
		t.Fatalf("迁移 providers 表失败: %v", err)
	}
	return db
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

// TestMaskKey_脱敏格式 覆盖普通长度、边界长度与空串。
func TestMaskKey_脱敏格式(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"sk-abcdefghijklmn", "sk-****klmn"}, // 普通长度:前 3 + **** + 后 4
		{"123456789", "123****6789"},         // 最短可部分脱敏的长度(9 位)
		{"12345678", "********"},             // 恰好 8 位:全部脱敏
		{"abc", "***"},                       // 短 key:全部脱敏
		{"", ""},                             // 空串:输出空串
	}
	for _, c := range cases {
		if got := MaskKey(c.key); got != c.want {
			t.Errorf("MaskKey(%q) = %q,期望 %q", c.key, got, c.want)
		}
	}
}

// TestCreateProvider_校验失败返回1401 覆盖名称、URL、Key 三类必填/格式错误。
func TestCreateProvider_校验失败返回1401(t *testing.T) {
	db := newProviderTestDB(t)
	cases := []struct {
		name string
		in   ProviderInput
	}{
		{"名称为空", ProviderInput{Name: "", BaseURL: "https://api.example.com", APIKey: "sk-test"}},
		{"URL 缺少 scheme", ProviderInput{Name: "a", BaseURL: "api.example.com", APIKey: "sk-test"}},
		{"URL 协议不是 http(s)", ProviderInput{Name: "a", BaseURL: "ftp://api.example.com", APIKey: "sk-test"}},
		{"URL 缺少主机", ProviderInput{Name: "a", BaseURL: "http://", APIKey: "sk-test"}},
		{"Key 为空", ProviderInput{Name: "a", BaseURL: "https://api.example.com", APIKey: " "}},
	}
	for _, c := range cases {
		_, err := CreateProvider(context.Background(), db, c.in)
		wantCode(t, err, apperr.CodeProviderInvalid)
	}
}

// TestCreateProvider_名称重复报1402 同名配置不允许重复创建。
func TestCreateProvider_名称重复报1402(t *testing.T) {
	db := newProviderTestDB(t)
	in := ProviderInput{Name: "官方站", BaseURL: "https://api.example.com", APIKey: "sk-abcdef123456"}
	if _, err := CreateProvider(context.Background(), db, in); err != nil {
		t.Fatalf("首次创建应成功,实际报错: %v", err)
	}
	_, err := CreateProvider(context.Background(), db, in)
	wantCode(t, err, apperr.CodeProviderDuplicate)
}

// TestCreateProvider_成功返回脱敏视图 验证视图字段与"响应不含明文 Key"。
func TestCreateProvider_成功返回脱敏视图(t *testing.T) {
	db := newProviderTestDB(t)
	view, err := CreateProvider(context.Background(), db, ProviderInput{
		Name: "官方站", BaseURL: "https://api.example.com", APIKey: "sk-abcdef123456",
	})
	if err != nil {
		t.Fatalf("创建应成功,实际报错: %v", err)
	}
	if view.ID == 0 || view.Name != "官方站" || view.BaseURL != "https://api.example.com" {
		t.Fatalf("视图字段不符: %+v", view)
	}
	if view.APIKeyMasked != "sk-****3456" {
		t.Fatalf("期望脱敏值 sk-****3456,实际 %q", view.APIKeyMasked)
	}
	// 序列化结果中不允许出现明文 Key
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if strings.Contains(string(data), "sk-abcdef123456") {
		t.Fatalf("序列化结果泄露明文 Key: %s", data)
	}
}

// TestUpdateProvider_空Key保留原值 编辑时 Key 留空应沿用原值,新 Key 则覆盖。
func TestUpdateProvider_空Key保留原值(t *testing.T) {
	db := newProviderTestDB(t)
	ctx := context.Background()
	view, err := CreateProvider(ctx, db, ProviderInput{
		Name: "官方站", BaseURL: "https://api.example.com", APIKey: "sk-abcdef123456",
	})
	if err != nil {
		t.Fatalf("创建应成功,实际报错: %v", err)
	}

	// Key 留空:名称与 URL 更新,原 Key 保留
	kept, err := UpdateProvider(ctx, db, view.ID, ProviderInput{
		Name: "官方站改名", BaseURL: "https://api2.example.com", APIKey: "",
	})
	if err != nil {
		t.Fatalf("编辑应成功,实际报错: %v", err)
	}
	if kept.Name != "官方站改名" || kept.BaseURL != "https://api2.example.com" {
		t.Fatalf("编辑结果不符: %+v", kept)
	}
	if kept.APIKeyMasked != "sk-****3456" {
		t.Fatalf("期望沿用原 Key 的脱敏值,实际 %q", kept.APIKeyMasked)
	}
	// 落库值必须仍是原 Key
	var stored model.Provider
	if err := db.First(&stored, view.ID).Error; err != nil {
		t.Fatalf("查询落库记录失败: %v", err)
	}
	if stored.APIKey != "sk-abcdef123456" {
		t.Fatalf("期望原 Key 保留,实际 %q", stored.APIKey)
	}

	// 提供新 Key:覆盖旧值
	replaced, err := UpdateProvider(ctx, db, view.ID, ProviderInput{
		Name: "官方站改名", BaseURL: "https://api2.example.com", APIKey: "sk-zzzzzzzz9999",
	})
	if err != nil {
		t.Fatalf("编辑应成功,实际报错: %v", err)
	}
	if replaced.APIKeyMasked != "sk-****9999" {
		t.Fatalf("期望新 Key 的脱敏值,实际 %q", replaced.APIKeyMasked)
	}
}

// TestUpdateProvider_改名冲突与不存在报对应错误码 改名撞名报 1402,
// 编辑不存在的配置报 1404,自身沿用原名不算冲突。
func TestUpdateProvider_改名冲突与不存在报对应错误码(t *testing.T) {
	db := newProviderTestDB(t)
	ctx := context.Background()
	first, err := CreateProvider(ctx, db, ProviderInput{Name: "A站", BaseURL: "https://a.example.com", APIKey: "sk-aaaa11112222"})
	if err != nil {
		t.Fatalf("创建 A站 失败: %v", err)
	}
	second, err := CreateProvider(ctx, db, ProviderInput{Name: "B站", BaseURL: "https://b.example.com", APIKey: "sk-bbbb11112222"})
	if err != nil {
		t.Fatalf("创建 B站 失败: %v", err)
	}

	// 改成已有名称:1402
	_, err = UpdateProvider(ctx, db, second.ID, ProviderInput{Name: "A站", BaseURL: "https://b.example.com", APIKey: ""})
	wantCode(t, err, apperr.CodeProviderDuplicate)

	// 名称不变(自身)不算冲突
	if _, err := UpdateProvider(ctx, db, second.ID, ProviderInput{Name: "B站", BaseURL: "https://b2.example.com", APIKey: ""}); err != nil {
		t.Fatalf("沿用自身名称不应报冲突: %v", err)
	}

	// 不存在的 ID:1404
	_, err = UpdateProvider(ctx, db, 9999, ProviderInput{Name: "C站", BaseURL: "https://c.example.com", APIKey: "sk-cccc11112222"})
	wantCode(t, err, apperr.CodeProviderNotFound)

	// 编辑时校验仍然生效:非法 URL 报 1401
	_, err = UpdateProvider(ctx, db, first.ID, ProviderInput{Name: "A站", BaseURL: "not-a-url", APIKey: ""})
	wantCode(t, err, apperr.CodeProviderInvalid)
}

// TestDeleteProvider_删除成功与不存在报1404 删除后列表为空,重复删除报 1404。
func TestDeleteProvider_删除成功与不存在报1404(t *testing.T) {
	db := newProviderTestDB(t)
	ctx := context.Background()
	view, err := CreateProvider(ctx, db, ProviderInput{Name: "官方站", BaseURL: "https://api.example.com", APIKey: "sk-abcdef123456"})
	if err != nil {
		t.Fatalf("创建应成功,实际报错: %v", err)
	}

	if err := DeleteProvider(ctx, db, view.ID); err != nil {
		t.Fatalf("删除应成功,实际报错: %v", err)
	}
	views, err := ListProviders(ctx, db)
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("删除后列表应为空,实际 %d 条", len(views))
	}

	// 不存在/已删除的 ID:1404
	err = DeleteProvider(ctx, db, view.ID)
	wantCode(t, err, apperr.CodeProviderNotFound)
}

// TestListProviders_按创建时间正序 显式写入不同创建时间验证排序。
func TestListProviders_按创建时间正序(t *testing.T) {
	db := newProviderTestDB(t)
	base := time.Now().UTC().Truncate(time.Second)
	rows := []model.Provider{
		{Name: "旧站", BaseURL: "https://old.example.com", APIKey: "sk-old11112222", CreatedAt: base, UpdatedAt: base},
		{Name: "新站", BaseURL: "https://new.example.com", APIKey: "sk-new11112222", CreatedAt: base.Add(time.Hour), UpdatedAt: base.Add(time.Hour)},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("插入测试数据失败: %v", err)
		}
	}
	views, err := ListProviders(context.Background(), db)
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	if len(views) != 2 || views[0].Name != "旧站" || views[1].Name != "新站" {
		t.Fatalf("期望按创建时间正序 [旧站 新站],实际 %+v", views)
	}
}
