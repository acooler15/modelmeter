// Package handler 存放 gin 处理器,按资源一文件;只做解析输入、调用
// service、输出统一信封三件事,不放业务逻辑、不直接访问 gorm.DB 之外
// 的数据操作(见 .trellis/spec/backend/directory-structure.md)。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/handler/response"
	"github.com/acooler15/modelmeter/internal/service"
)

// parseProviderID 解析路径参数中的配置 ID;非数字或为 0 视为非法(1401)。
func parseProviderID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, apperr.New(apperr.CodeProviderInvalid, "接口配置 ID 不合法")
	}
	return uint(id), nil
}

// bindProviderInput 解析请求体为 ProviderInput;JSON 格式错误返回 1401。
func bindProviderInput(c *gin.Context) (service.ProviderInput, error) {
	var in service.ProviderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		return service.ProviderInput{}, apperr.New(apperr.CodeProviderInvalid, "请求参数格式错误")
	}
	return in, nil
}

// ProviderList GET /api/providers 接口配置列表:按创建时间正序,Key 只返回脱敏值。
func ProviderList(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		views, err := service.ListProviders(c.Request.Context(), db)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, views)
	}
}

// ProviderCreate POST /api/providers 新增接口配置:名称、Base URL、API Key 均必填。
func ProviderCreate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		in, err := bindProviderInput(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		view, err := service.CreateProvider(c.Request.Context(), db, in)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, view)
	}
}

// ProviderUpdate PUT /api/providers/:id 编辑接口配置:api_key 留空表示沿用原值。
func ProviderUpdate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseProviderID(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		in, err := bindProviderInput(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		view, err := service.UpdateProvider(c.Request.Context(), db, id, in)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, view)
	}
}

// ProviderDelete DELETE /api/providers/:id 删除接口配置。
// data 返回被删 ID:信封约定成功必须携带非空 data,同时便于前端核对。
func ProviderDelete(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseProviderID(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		if err := service.DeleteProvider(c.Request.Context(), db, id); err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, gin.H{"id": id})
	}
}

// ProviderModels GET /api/providers/:id/models 经后端代理拉取上游模型列表,
// 实时透传不落库;网络/鉴权/响应异常由 service 层归类为 1500/1501/1502。
func ProviderModels(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseProviderID(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		models, err := service.ListModels(c.Request.Context(), db, id)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, models)
	}
}
