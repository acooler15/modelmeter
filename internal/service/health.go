// Package service 承载业务逻辑,只依赖 model 层与类型化参数,
// 禁止 import gin / net/http(分层约束见 .trellis/spec/backend/directory-structure.md)。
package service

import (
	"context"

	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// HealthStatus 服务健康状态。
type HealthStatus struct {
	Status string `json:"status"` // 固定为 "ok"
}

// CheckHealth 检查服务与数据库连通性,供 /api/health 使用。
// 用一次最小 SQL 探活,把 SQLite 驱动问题在启动初期就暴露出来。
func CheckHealth(ctx context.Context, db *gorm.DB) (HealthStatus, error) {
	if err := db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		return HealthStatus{}, apperr.Wrap(apperr.CodeInternal, "数据库查询失败", err)
	}
	return HealthStatus{Status: "ok"}, nil
}
