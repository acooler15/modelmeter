// Package response 提供统一 JSON 信封的输出辅助函数。
// 信封格式与前端 src/types/api.ts 的 ApiResponse 对应,规范见
// .trellis/spec/backend/error-handling.md。
package response

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// envelope 统一响应信封:成功 code=0;失败 code 为业务错误码、message 为中文提示。
type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 输出成功信封,业务数据放 data。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Code: 0, Message: "ok", Data: data})
}

// Fail 输出失败信封:apperr 使用其业务码与中文提示;其余错误一律按
// 服务器内部错误处理,细节只进日志、不暴露给客户端。
func Fail(c *gin.Context, err error) {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		// 内部类错误需要落日志便于排查;4xx 类业务校验错误不打日志
		if ae.Code == apperr.CodeInternal {
			slog.Error("internal error",
				"code", ae.Code,
				"err", err.Error(),
				"request_id", c.GetString("request_id"),
			)
		}
		c.JSON(httpStatusFor(ae.Code), envelope{Code: ae.Code, Message: ae.Message})
		return
	}

	// 预期外的程序错误:记录根因,对外只给通用文案
	slog.Error("unexpected handler error",
		"err", err.Error(),
		"request_id", c.GetString("request_id"),
	)
	c.JSON(http.StatusInternalServerError, envelope{
		Code:    apperr.CodeInternal,
		Message: "服务器内部错误",
	})
}

// httpStatusFor 把业务错误码映射为 HTTP 状态码;前端主要依据 code 判断,
// 状态码仅作辅助。未登记的资源类错误码按参数错误(400)处理。
func httpStatusFor(code int) int {
	switch code {
	case apperr.CodeNotFound, apperr.CodeProviderNotFound:
		return http.StatusNotFound
	case apperr.CodeInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}
