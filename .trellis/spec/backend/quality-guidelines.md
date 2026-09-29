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
- **「编辑留空沿用原值」的判断必须用 `strings.TrimSpace(v) == ""`**,与新增
  校验口径一致;纯空白串当作留空,避免用户误输入空格清掉原凭据
  (接口配置与 New API 配置均按此实现)。
- **改写用户本地/落盘文件必须原子替换**:同目录写临时文件后 `os.Rename`,
  禁止 `os.WriteFile` 直接截断原文件(写回中途失败会把文件破坏成半截),
  参考实现见 agentconf 的 `WriteJSONFile`(同目录临时文件+改名替换)。
- **外部 JSON 配置整读整写必须「白名单树编辑」**:`json.Decoder` +
  `UseNumber()` 整读为 `map[string]any` 树,只改白名单键后整树写回;
  禁止声明完整 struct 直接 Unmarshal(未声明字段会被整段丢弃,连凭据与
  模板字段一起丢),未知键与凭据节点原样放回。读侧先校验文件内的版本
  标记(如 ZCode 的 `schemaVersion`),存在但不识别时拒绝读写,防版本
  漂移写坏文件;标记缺失则容忍(残缺文件仍可救)。参考实现见 agentconf
  的 `agents/treeutil.go`(读树+白名单改键+兄弟键保留)。
  > **例外**:「未知键原样保留」不适用于目标工具 schema 声明 `.strict()`
  > 的节点(如 ZCode 的 `defaultModelSelection`)——此类节点整体替换为
  > 规范形状,节点内未知键会让目标工具拒载整份配置,保留坏键与零丢失
  > 目标相悖;代码注释必须写明这是显式例外及原因。
- 上游凭据(API Key / 令牌)只在代理客户端(llmclient)内进入请求头;
  DTO 的 json tag 用 `-` 挡序列化,对外一律脱敏视图。
- **为注册表式接口增加可选能力时,用「能力接口 + 能力位」协商**:可选
  方法定义为独立接口(如 agentconf 的 `DefaultModelSetter`),状态视图
  (Snapshot)暴露布尔能力位,handler 用类型断言探测能力、未实现报专用
  错误码(4405);禁止在基础接口加全量桩方法强迫所有实现空实现。
  能力位与接口实现的一致性必须有测试兜底。

---

## 测试要求

- 测试框架:标准库 `testing`(表驱动测试),不引入断言库。
- 范围:`internal/service` 的业务规则必须覆盖;model 查询辅助建议配合
  内存 SQLite(`:memory:`)覆盖;handler 以 service 测试为主,不强制。
- 命名:`被测函数_test.go`,`TestXxx_场景` 形式,注释用中文说明场景。
- 命令:`go test ./internal/... ./cmd/...`;提交前必须全绿。
  (不要用 `go test ./...`:会扫到 `web/node_modules` 内的第三方 Go 文件,
  产生大量无意义输出。)

---

## 代码评审清单

- [ ] 分层是否正确(handler 无业务逻辑、service 无 HTTP 依赖)?
- [ ] 错误是否走 `apperr`、消息是否为中文?
- [ ] SQL 是否有注入风险(是否用了占位符)?
- [ ] 上下文是否透传、事务是否用 `tx`?
- [ ] 注释是否为中文、是否解释了"为什么"?
- [ ] `go vet ./internal/... ./cmd/...` 与 `go test ./internal/... ./cmd/...` 是否通过?
