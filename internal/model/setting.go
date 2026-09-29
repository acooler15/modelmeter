package model

// Setting 通用键值配置:值存 JSON 文本,首用于 New API 配置(键 newapi)。
// 不嵌入 gorm.Model:配置以键为主键整体覆盖,无需创建/更新时间与软删除。
type Setting struct {
	Key   string `gorm:"primaryKey;size:100" json:"key"`
	Value string `gorm:"type:text" json:"-"` // 内容可能含令牌,永不序列化输出
}

// TableName 显式指定表名,不依赖 GORM 的表名推断。
func (Setting) TableName() string { return "settings" }
