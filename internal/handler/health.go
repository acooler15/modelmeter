// Package handler 存放 gin 处理器,按资源一文件;只做解析输入、调用
// service、输出统一信封三件事,不放业务逻辑、不直接访问 gorm.DB 之外
// 的数据操作(见 .trellis/spec/backend/directory-structure.md)。
package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/handler/response"
	"github.com/acooler15/modelmeter/internal/service"
)

// Health GET /api/health 健康检查:验证进程、路由与数据库连通性。
func Health(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := service.CheckHealth(c.Request.Context(), db)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, status)
	}
}
