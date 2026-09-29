// Package model 存放 GORM 模型结构体与小型查询辅助。
package model

import "time"

// Provider 接口配置:用于访问某个 LLM 服务站点。
// 不嵌入 gorm.Model(即不含 DeletedAt):API Key 属敏感凭据,
// 删除必须物理删除、不在库里残留软删数据。
type Provider struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:100" json:"name"`
	BaseURL   string    `gorm:"size:500" json:"base_url"`
	APIKey    string    `gorm:"size:500" json:"-"` // 永不序列化输出,防止明文外泄
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名,不依赖 GORM 的表名推断。
func (Provider) TableName() string { return "providers" }
