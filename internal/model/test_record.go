// Package model 存放 GORM 模型结构体与小型查询辅助。
package model

import "time"

// TestRecord 模型测试记录:成功与失败都落库,服务层只保留最近 200 条。
// 不含 API Key 等任何凭据字段;回复原文仅用于页面回看。
// 不嵌入 gorm.Model(无软删除):记录随清理策略滚动删除,物理删除即可。
type TestRecord struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ProviderID       uint      `json:"provider_id"`
	ProviderName     string    `json:"provider_name"`
	Protocol         string    `json:"protocol"` // chat_completions / responses / anthropic
	Model            string    `json:"model"`
	SystemPrompt     string    `json:"system_prompt"`
	UserMessage      string    `json:"user_message"`
	Temperature      *float64  `json:"temperature"`
	MaxTokens        *int      `json:"max_tokens"`
	Stream           bool      `json:"stream"`
	Succeeded        bool      `json:"succeeded"`
	Reply            string    `json:"reply"`
	ErrMessage       string    `json:"err_message"`
	FirstLatencyMs   int64     `json:"first_latency_ms"`
	TotalLatencyMs   int64     `json:"total_latency_ms"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CreatedAt        time.Time `json:"created_at"`
}

// TableName 显式指定表名,不依赖 GORM 的表名推断。
func (TestRecord) TableName() string { return "test_records" }
