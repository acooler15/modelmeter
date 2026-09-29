package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/middleware"
)

// NewRouter 装配中间件与 API 路由并返回 gin 引擎。
// 横切中间件统一在此注册;SPA 托管由 internal/web 在 main.go 中另行挂载。
func NewRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.RequestLog(),
		middleware.Recovery(),
	)

	api := r.Group("/api")
	api.GET("/health", Health(db))

	return r
}
