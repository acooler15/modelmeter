package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/middleware"
)

// NewRouter 装配中间件与 API 路由并返回 gin 引擎。
// 横切中间件统一在此注册;SPA 托管由 internal/web 在 main.go 中另行挂载。
// dataDir 为运行数据目录,供 Agent 配置的备份/还原使用。
func NewRouter(db *gorm.DB, dataDir string) *gin.Engine {
	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.RequestLog(),
		middleware.Recovery(),
	)

	api := r.Group("/api")
	api.GET("/health", Health(db))

	// 接口配置:列表/新增挂集合根,编辑/删除/模型列表挂 :id
	p := api.Group("/providers")
	p.GET("", ProviderList(db))
	p.POST("", ProviderCreate(db))
	p.PUT("/:id", ProviderUpdate(db))
	p.DELETE("/:id", ProviderDelete(db))
	p.GET("/:id/models", ProviderModels(db))

	// 模型测试:非流式/流式入口 + 记录查询/清空
	t := api.Group("/test")
	t.POST("", TestRun(db))
	t.POST("/stream", TestStream(db))
	t.GET("/records", TestRecords(db))
	t.DELETE("/records", TestRecordsClear(db))

	// New API 模型费率:配置查询/保存、费率拉取与成本估算
	n := api.Group("/newapi")
	n.GET("/config", NewAPIConfigGet(db))
	n.PUT("/config", NewAPIConfigSave(db))
	n.GET("/rates", NewAPIRates(db))
	n.POST("/estimate", NewAPIEstimate(db))

	// Agent 模型配置:列表/详情/模型清单/批量写回/还原(写回前自动备份,
	// 备份目录由 dataDir 决定)
	a := api.Group("/agents")
	a.GET("", AgentList())
	a.GET("/:name", AgentGet())
	a.GET("/:name/models", AgentModelsGet())
	a.PUT("/:name/models", AgentModelsApply(dataDir))
	a.POST("/:name/restore", AgentRestore(dataDir))

	return r
}
