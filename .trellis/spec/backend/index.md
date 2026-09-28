# 后端开发规范

> ModelMeter 后端(Go)开发约定。

---

## 概述

技术栈基线:**Go 1.22+ / Gin / GORM / SQLite(纯 Go 驱动)**,前端构建
产物经 `go:embed` 嵌入,部署形态为单个静态二进制。目录分层为
handler / service / model,详见各规范文件。

> 语言约定:所有规范文档使用中文;代码注释使用中文;标识符使用英文。

---

## 规范索引

| 文档 | 内容 | 状态 |
|------|------|------|
| [目录结构](./directory-structure.md) | Go module 布局、handler/service/model 分层、命名 | 已填写 |
| [数据库规范](./database-guidelines.md) | GORM + SQLite、AutoMigrate、查询与事务模式 | 已填写 |
| [错误处理](./error-handling.md) | apperr 业务错误、统一 JSON 信封 | 已填写 |
| [质量规范](./quality-guidelines.md) | 禁止/必须模式、测试要求、评审清单 | 已填写 |
| [日志规范](./logging-guidelines.md) | slog 结构化日志、级别、应记/禁记内容 | 已填写 |

---

## 维护说明

规范记录的是项目实际约定。代码演进导致约定变化时,同步更新对应文件,
保持规范与现实一致。
