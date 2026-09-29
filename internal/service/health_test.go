package service

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// TestCheckHealth_数据库可用时返回ok 验证探活正常路径。
func TestCheckHealth_数据库可用时返回ok(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}

	status, err := CheckHealth(context.Background(), db)
	if err != nil {
		t.Fatalf("期望健康检查通过,实际报错: %v", err)
	}
	if status.Status != "ok" {
		t.Fatalf("期望 status=ok,实际 %q", status.Status)
	}
}

// TestCheckHealth_上下文取消时返回业务错误 验证底层错误被包装为 apperr
// 且不把数据库原始报错泄露给调用方。
func TestCheckHealth_上下文取消时返回业务错误(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}

	// 预先取消的上下文应使查询立即失败
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = CheckHealth(ctx, db)
	if err == nil {
		t.Fatal("期望健康检查报错,实际通过")
	}
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		t.Fatalf("期望错误被包装为 apperr.Error,实际 %T", err)
	}
	if ae.Code != apperr.CodeInternal {
		t.Fatalf("期望业务码 %d,实际 %d", apperr.CodeInternal, ae.Code)
	}
}
