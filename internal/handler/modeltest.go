// modeltest.go 模型测试处理器:非流式/流式测试入口与记录查询/清空。
// 流式端点在开始推送前失败走统一错误信封;一旦开始推送则改按 SSE 事件
// 输出(data: {"type":"delta"|"done"|"error", ...}\n\n),不再套信封。
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/handler/response"
	"github.com/acooler15/modelmeter/internal/service"
)

// bindTestInput 解析请求体为 service.TestInput;JSON 格式错误返回 2401。
func bindTestInput(c *gin.Context) (service.TestInput, error) {
	var in service.TestInput
	if err := c.ShouldBindJSON(&in); err != nil {
		return service.TestInput{}, apperr.New(apperr.CodeTestInvalid, "请求参数格式错误")
	}
	return in, nil
}

// TestRun POST /api/test 非流式测试:信封 data 携带 {result, record}。
func TestRun(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		in, err := bindTestInput(c)
		if err != nil {
			response.Fail(c, err)
			return
		}
		result, record, err := service.RunTest(c.Request.Context(), db, in)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, gin.H{"result": result, "record": record})
	}
}

// TestStream POST /api/test/stream 流式测试:校验通过后逐事件转发上游增量,
// 每个事件独立 Flush;结束时补发 done 事件携带完整结果与记录。
func TestStream(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		in, err := bindTestInput(c)
		if err != nil {
			response.Fail(c, err)
			return
		}

		pushed := false // 是否已开始推送 SSE 事件
		startSSE := func() {
			header := c.Writer.Header()
			header.Set("Content-Type", "text/event-stream; charset=utf-8")
			header.Set("Cache-Control", "no-cache")
			// 关闭 Nginx 等反向代理的缓冲,保证逐字推送
			header.Set("X-Accel-Buffering", "no")
			c.Writer.WriteHeader(http.StatusOK)
		}
		sendEvent := func(payload any) {
			data, err := json.Marshal(payload)
			if err != nil {
				return // 事件为固定结构,序列化失败按程序性异常跳过
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		}

		result, record, err := service.StreamTest(c.Request.Context(), db, in, func(text string) {
			if !pushed {
				// 首个增量到达才切换为 SSE 输出,此前的失败仍可走错误信封
				startSSE()
				pushed = true
			}
			sendEvent(gin.H{"type": "delta", "text": text})
		})

		if err != nil {
			if !pushed {
				// 尚未推送任何增量:回统一错误信封,由前端按普通请求处理
				response.Fail(c, err)
				return
			}
			// 推送已开始:以错误事件收尾,消息为面向用户的中文文案
			sendEvent(gin.H{"type": "error", "message": service.TestErrMessage(err)})
			return
		}
		if !pushed {
			// 成功但零增量(空回复):同样以 SSE 事件交付结果
			startSSE()
			pushed = true
		}
		sendEvent(gin.H{"type": "done", "result": result, "record": record})
	}
}

// TestRecords GET /api/test/records?limit=100 最近测试记录,创建时间倒序。
func TestRecords(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 100
		if v := c.Query("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				response.Fail(c, apperr.New(apperr.CodeTestInvalid, "limit 参数不合法"))
				return
			}
			limit = n
		}
		records, err := service.ListTestRecords(c.Request.Context(), db, limit)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, records)
	}
}

// TestRecordsClear DELETE /api/test/records 清空测试记录,返回删除条数。
func TestRecordsClear(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		deleted, err := service.ClearTestRecords(c.Request.Context(), db)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, gin.H{"deleted": deleted})
	}
}
