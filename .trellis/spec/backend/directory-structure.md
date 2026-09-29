# 目录结构

> 后端(Go)代码的组织方式。

---

## 概述

ModelMeter 后端是一个单一 Go module,同时提供 REST API 和内嵌的前端页面。
技术栈基线(2026-09-28 项目引导时确定):**Go 1.22+ / Gin / GORM / SQLite**。
部署形态是单个静态二进制:`web/dist` 下的前端构建产物通过 `go:embed`
嵌入,由同一进程对外服务。

> 约定:代码标识符(变量、函数、包名)用英文,**注释一律用中文**。

---

## 目录布局

```
ModelMeter/
├── cmd/
│   └── server/
│       └── main.go          # 入口:加载配置、初始化 DB、装配路由、托管 embed.FS
├── internal/
│   ├── config/              # 环境变量/启动参数解析为类型化的 Config 结构体
│   ├── handler/             # gin 处理器,按资源一文件(meter.go ...)
│   ├── service/             # 业务逻辑;禁止 import gin
│   ├── model/               # GORM 模型结构体 + 按表的查询辅助函数
│   ├── middleware/          # 请求日志、recover、request ID 等中间件
│   ├── apperr/              # 类型化业务错误(见 error-handling.md)
│   └── web/                 # embed.go: //go:embed web/dist + SPA 回退路由
├── web/                     # Vue 前端(见 ../frontend/directory-structure.md)
│   └── dist/                # 前端构建产物,嵌入二进制(已 gitignore)
├── data/                    # 运行期 SQLite 数据库文件(已 gitignore)
├── go.mod
└── Makefile                 # dev / build / test 等常用目标
```

新代码一律放在 `internal/` 下:module 根目录下的包会被视为对外公开 API,
容易被误引用。

---

## 模块划分

- **handler** — 只做三件事:解析 HTTP 输入(path/query/body)、调用一个
  service 函数、写出统一 JSON 响应。不放业务逻辑,不许直接访问 `gorm.DB`。
- **service** — 全部业务规则和校验。接收类型化参数和 `context.Context`,
  返回 `(结果, error)`。禁止 import `gin` / `net/http`。
- **model** — GORM 结构体、`TableName()`、小型查询辅助
  (如 `ListMeters(db, filter)`)。复杂查询写在这里,service 负责编排。
- 横切关注点(鉴权、日志、recover)放 `internal/middleware`,在
  `main.go` 统一注册。

新增一个资源(以 `meter` 为例)时,纵向切片顺序是:
`model/meter.go` → `service/meter.go` → `handler/meter.go` → 在
`cmd/server/main.go` 注册路由(main.go 变大后再拆到
`internal/handler/router.go`)。

---

## 扩展点包布局(agentconf 型)

读取/写回外部工具本地配置的扩展点(如 agentconf,后续接入新工具照此):

- 接口、注册表、共享文件工具放根包 `internal/service/agentconf`;
- 各工具实现放子包 `internal/service/agentconf/agents/`,子包 `init()`
  调 `agentconf.Register` 自注册;
- `cmd/server/main.go` 以空导入 `_ ".../agentconf/agents"` 触发注册;
- **根包禁止 import 子包**(子包需引用根包类型,反向依赖会成环);
  handler 只 import 根包,不感知具体实现。

---

## 命名约定

- 包名:小写、单个单词,禁止 `utils` / `common` 这类大杂烩包。
- 文件名:`snake_case.go`,按所服务的资源命名(`meter.go`、`meter_test.go`)。
- 构造函数:`NewXxx`;接口在消费方声明,保持小(1–3 个方法)。
- HTTP 服务默认监听 `:8422`(刻意避开 8080 等常用端口),可用环境变量 `PORT` 覆盖。

---

## 示例

第一个纵向切片(`meter` 资源)落地后即为参考模式;写第二个资源时严格
照搬它的分层方式,不要另起炉灶。
