// Package middleware 存放横切关注点中间件(请求 ID、访问日志、panic 兜底),
// 由 internal/handler/router.go 在装配路由时统一注册。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDHeader = "X-Request-ID"

// RequestID 为每个请求生成唯一 ID:优先沿用客户端带来的 ID,否则生成新的;
// 写入响应头与 gin Context,供访问日志串联一次请求的全部日志。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		c.Set("request_id", id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// newRequestID 生成 16 位十六进制随机 ID。
func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 读取失败极罕见;退化为纳秒时间戳保证 ID 仍可用
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}
