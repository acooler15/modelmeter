# 数据库规范

> 本项目的数据库使用约定。

---

## 概述

- **ORM**:GORM v2(`gorm.io/gorm`)。
- **驱动**:`github.com/glebarez/sqlite` — 纯 Go 实现的 SQLite,无 CGO,
  二进制保持静态、可交叉编译。
- **数据库文件**:`data/modelmeter.db`,启动时自动创建;`data/` 已
  gitignore。所有时间一律存 UTC。
- **连接**:`cmd/server/main.go` 中 `gorm.Open` 一次,向下注入
  (service → model)。禁止在 `main.go` 之外再调用 `gorm.Open`。

---

## 表结构管理

当前规模下,表结构变更通过启动时 `AutoMigrate` 完成:

```go
db.AutoMigrate(&model.Meter{}, &model.Reading{})
```

规则:

- 新增模型必须加进 `main.go` 的 `AutoMigrate` 列表。
- `AutoMigrate` 只会加列/加表 —— 破坏性变更(删列、改列名、改类型)需要在
  任务记录里写明手工迁移步骤后才能合并。
- 如果破坏性变更变得频繁,再作为独立决策引入版本化迁移工具(如
  `golang-migrate`),不要悄悄引入。

---

## 模型约定

```go
package model

// Meter 计量表示例模型。
type Meter struct {
    gorm.Model              // ID、CreatedAt、UpdatedAt、DeletedAt(软删除)
    Name   string `gorm:"size:128;not null;uniqueIndex"`
    Unit   string `gorm:"size:32;not null;default:''"`
}

// TableName 显式指定表名。
func (Meter) TableName() string { return "meters" }
```

- 统一嵌入 `gorm.Model` 获得标准四字段;软删除保持开启。
  **豁免**:承载敏感凭据的模型(如 `Provider`)显式定义字段、不嵌
  `gorm.Model`——API Key 必须物理删除,不允许软删数据残留库中。
- 必须显式定义 `TableName()`,返回小写复数 snake_case
  (`meters`、`readings`)。不要依赖 GORM 的表名推断。
- 列名使用字段名的隐式 snake_case;`gorm:` 标签只用于
  size/null/index/default,不用于改列名。
- 时间一律存 UTC:在 `main.go` 唯一允许的 `gorm.Open` 处统一注入
  `gorm.Config{ NowFunc: time.Now().UTC }`,业务代码不再各自转换。

---

## 查询模式

- 始终携带上下文:`db.WithContext(ctx)`。
- 查询辅助函数放 `internal/model`,签名形如 `(db *gorm.DB, ...)`;
  条件多于一个时用过滤结构体:

```go
// ListMeters 按过滤条件分页查询计量表,返回数据与总数。
func ListMeters(db *gorm.DB, f MeterFilter) ([]Meter, int64, error)
```

- 多语句写操作用 `db.Transaction(func(tx *gorm.DB) error { ... })`;
  闭包内必须用 `tx`,禁止使用外层 `db`。
- 关联加载用 `Preload` 按查询显式声明,不做全局预加载。
- 原生 SQL 只能走 `db.Raw(...).Scan(...)` / `db.Exec(...)` 且必须用占位符。
  用 `fmt.Sprintf` 拼 SQL 是禁止项。
- 分页:`Limit`/`Offset` 来自经过校验的过滤条件(单页上限 200 条)。

---

## 常见错误

- 忘写 `WithContext` —— 请求取消后查询不会随之中断。
- 事务闭包里误用外层 `db` 句柄。
- 指望 `AutoMigrate` 做破坏性变更(它会静默不做任何事)。
- 把 SQLite 原始报错字符串直接返回给 API 客户端(见 error-handling.md)。
