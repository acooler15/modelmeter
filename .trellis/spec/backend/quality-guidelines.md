# 质量规范

> 后端代码质量标准。

---

## 概述

- 格式化与静态检查:`gofmt`(强制)、`go vet`、`golangci-lint`(建议)。
- 命名与分层见 [directory-structure.md](./directory-structure.md)。
- **注释一律用中文**,写"为什么"而不是复述代码;导出符号必须有中文注释。
  标识符本身仍用英文。

---

## 禁止模式

- `panic` 用于业务流程(仅限程序初始化阶段的致命错误)。
- `fmt.Sprintf` 拼接 SQL。
- `_ = err` 吞错;不处理的 error 必须显式注释原因。
- 包级全局可变状态(全局 `db` 句柄、全局配置变量),一律通过依赖注入。
- `internal/service` 里 import `gin` / `net/http`(分层约束)。
- 在 `main.go` 之外调用 `gorm.Open` / `slog.SetDefault`。

---

## 必须模式

- 所有 I/O 函数第一个参数收 `context.Context` 并透传。
- 写数据库的多语句操作包在事务里。
- 返回给前端的错误走 `internal/apperr`,消息为中文。
- 新增依赖前先确认标准库/已有依赖无法胜任;引入新第三方库需在任务记录
  中说明理由。

---

## 测试要求

- 测试框架:标准库 `testing`(表驱动测试),不引入断言库。
- 范围:`internal/service` 的业务规则必须覆盖;model 查询辅助建议配合
  内存 SQLite(`:memory:`)覆盖;handler 以 service 测试为主,不强制。
- 命名:`被测函数_test.go`,`TestXxx_场景` 形式,注释用中文说明场景。
- 命令:`go test ./...`;提交前必须全绿。

---

## 代码评审清单

- [ ] 分层是否正确(handler 无业务逻辑、service 无 HTTP 依赖)?
- [ ] 错误是否走 `apperr`、消息是否为中文?
- [ ] SQL 是否有注入风险(是否用了占位符)?
- [ ] 上下文是否透传、事务是否用 `tx`?
- [ ] 注释是否为中文、是否解释了"为什么"?
- [ ] `go vet ./...` 与 `go test ./...` 是否通过?
