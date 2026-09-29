# 执行清单

验证命令:后端 `go build ./... && go vet ./internal/... ./cmd/... && go test ./internal/... ./cmd/...`(遵守 spec:禁止 `go test ./...`,会扫进 web/node_modules);前端 `cd web && npm run lint && npm run build`。每步末尾跑对应命令,绿了再进下一步;每步一个 commit 作为回滚点。

1. [x] **接口与错误码**:agentconf.go 增 DefaultModel / DefaultModelPatch / DefaultModelSetter 类型,Snapshot 增 supports_default_model / default_model 字段,FieldSpec.Type 注释加 "list";apperr/codes.go 4xxx 段增 CodeAgentUnsupported=4405;response 包状态码映射核对。→ `go build ./...` 绿(handler 测试若断言 Snapshot payload 需同步)
2. [x] **ZCode 读侧**:zcode.go 的 readTree 增 schemaVersion 校验(存在且非数字 1 → 4402);zcodeEntryFields 增 supports_image / supports_json_schema_output / reasoning_levels 三键读取;zcodeColumns 增三列(reasoning_levels 的 Options 取全部规则 values 保序并集);Snapshot 组装解析 defaultModelSelection 与能力位。→ `go test ./internal/service/agentconf/...` 绿(先补读侧用例)
3. [x] **ZCode 写侧**:applyZCodePatch 白名单扩展三键(缺失节点创建、兄弟键保留)、manual 冲突守护(追加前查 manualProviderModelRules)、新增 ApplyDefaultModel(设置/清除/校验/defaultModelSelection 整体替换例外);fixture schemaVersion 2→1,zcode_test.go 按 design 第 6 节补全。→ `go test ./internal/service/agentconf/...` 绿
4. [x] **WorkBuddy**:workBuddyColumns 补 supported_efforts(list 列);workbuddy_test.go 补列与写回断言、能力位/接口反向断言。→ `go test ./internal/service/agentconf/...` 绿
5. [x] **handler/router**:router.go 增 PUT /api/agents/:name/default-model,agent.go 增 AgentDefaultModelSet(能力断言 → 4405),agent_test.go 用例增补(fakeAgent 两变体)。→ `go test ./internal/handler/...` 绿
6. [x] **前端**:types/agent.ts、api/agent.ts、AgentModelCard.vue(list 列渲染 + 默认模型区 + 事件)、AgentView.vue 监听刷新。→ `npm run lint && npm run build` 绿
7. [x] **文档同步**:README 第 6 节勘误、包注释更新、`.trellis/spec/backend/quality-guidelines.md` 相关表述核对;确认无"当前选中模型不在管理范围"残留。
8. [x] **全量检查**(2.2 最后一轮):后端与前端全部验证命令;逐条核对 prd 验收标准。

## 回滚

每步一个 commit,失败时 `git reset` 回上一 commit。风险点:第 3 步树编辑(整体替换 defaultModelSelection 节点)与 manual 守护——fixture 断言 apiKey/未知键逐字节保留兜底;第 6 步前端仅展示层,可独立回退。

## 边界备忘

- 不动:backup.go、WriteJSONFile、treeutil.go 既有辅助、apperr 既有码段(仅新增 4405)、provider 层字段(enabled/apiType/baseUrl)与 access/apiKey。
- 不做:新增/删除供应商或模型条目、apiKey 相关任何读写、credentials.json、reasoningLevel.map / maxOutputTokens.map / 未观测能力键(supportsNativeWebSearch、supportsMidConversationSystem、inputFormat 其余键)、manualProviderModelRules 的清单展示(仅作冲突守护定位)。
- 测试命令一律 `./internal/... ./cmd/...` 范围,遵守 spec。
