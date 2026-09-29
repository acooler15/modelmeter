// newapi.go New API 模型费率处理器:配置查询/保存、费率拉取与成本估算。
package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/handler/response"
	"github.com/acooler15/modelmeter/internal/service"
)

// bindNewAPIConfigInput 解析请求体为 NewAPIConfigInput;JSON 格式错误返回 3401
// (3xxx 段没有独立的参数错误码,统一归入配置类错误)。
func bindNewAPIConfigInput(c *gin.Context) (service.NewAPIConfigInput, error) {
	var in service.NewAPIConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		return service.NewAPIConfigInput{}, apperr.New(apperr.CodeNewAPINotConfigured, "请求参数格式错误")
	}
	return in, nil
}

// NewAPIConfigGet GET /api/newapi/config 配置视图:令牌只回脱敏值,不回明文。
func NewAPIConfigGet(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := service.GetNewAPIConfig(c.Request.Context(), db)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, view)
	}
}

// NewAPIConfigSave PUT /api/newapi/config 保存配置;token 留空表示沿用原令牌。
func NewAPIConfigSave(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		in, err := bindNewAPIConfigInput(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		view, err := service.SaveNewAPIConfig(c.Request.Context(), db, in)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, view)
	}
}

// NewAPIRates GET /api/newapi/rates 实时拉取上游费率表,不落库;
// 未配置/网络/鉴权/响应异常分别报 3401/3500/3501/3502。
func NewAPIRates(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		entries, err := service.ListRates(c.Request.Context(), db)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, entries)
	}
}

// NewAPIEstimate POST /api/newapi/estimate 单次调用成本估算。
// 展示层增强接口:任何失败都降级为 available=false(code=0),不返回错误信封,
// 满足「估算不可用时前端不报错、不显示」的要求。
func NewAPIEstimate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.EstimateInput
		if err := c.ShouldBindJSON(&in); err != nil {
			// 请求体不合法按未命中处理:对前端表现为「无成本行」而非报错
			response.OK(c, gin.H{"available": false, "estimate": nil})
			return
		}
		estimate, ok := service.EstimateCost(c.Request.Context(), db, in.Model, in.PromptTokens, in.CompletionTokens)
		data := gin.H{"available": ok, "estimate": nil}
		if ok {
			data["estimate"] = estimate
		}
		response.OK(c, data)
	}
}
