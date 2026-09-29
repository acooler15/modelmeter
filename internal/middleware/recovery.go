package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/handler/response"
)

// Recovery 兜底恢复 handler 链中的 panic:记录含堆栈的错误日志,
// 并返回统一的 500 信封,避免进程退出或向客户端泄露堆栈。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered",
					"panic", r,
					"stack", string(debug.Stack()),
					"request_id", c.GetString("request_id"),
				)
				// 兜底错误不携带内部细节,固定通用文案
				response.Fail(c, apperr.New(apperr.CodeInternal, "服务器内部错误"))
			}
		}()
		c.Next()
	}
}
