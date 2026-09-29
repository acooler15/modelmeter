# 执行清单

验证命令:后端 `go build ./... && go vet ./... && go test ./...`;前端 `cd web && npm run lint && npm run build`。每步末尾跑对应命令,绿了再进下一步;每步一个 commit 作为回滚点。

1. [ ] **纯移动(行为不变)**:建 `internal/service/agentconf/agents/`,git mv `zcode*.go`、`workbuddy*.go` 进去,包名改 `agents`;fileExists/writeJSONFile/marshalPlainString 留根包并导出(FileExists/WriteJSONFile),backup.go 改用导出名;新增 agents/register.go 承接 init 注册;main.go 空导入触发;删根包 init。→ `go test ./...` 全绿(旧测试随迁,仍测 cli-bin 旧行为)
2. [ ] **接口演化 + ZCode 重写**:agentconf.go 按 design 改造(Snapshot 瘦身+Columns、ModelEntry/ModelPatch、删 Apply 加 Models/ApplyModels、FieldSpec 扩 number/bool/Readonly);agents/zcode.go 重写(目标 v2/provider_config.json,UseNumber 树编辑,删 cli-bin 逻辑),zcode_test.go 按 design 测试要点重写。→ `go test ./...` 绿
3. [ ] **WorkBuddy 实装**:agents/workbuddy.go 真实实现(models.json 读写+白名单+定位),workbuddy_test.go 重写。→ 同上绿
4. [ ] **handler/router**:agent.go 改造(新增 GET/PUT /:name/models,删 PUT /:name 与 values 绑定),router.go 更新,agent_test.go 用例增删。→ `go test ./...` 绿
5. [ ] **前端**:types/agent.ts、api/agent.ts 重写,AgentView.vue 重做(卡片+通用列驱动表格+批量保存+还原+重启提示)。→ `npm run lint && npm run build` 绿
6. [ ] **文档同步**:README 第 6 节、agentconf.go/agents 包注释、`.trellis/spec/backend/quality-guidelines.md` 旧表述;确认无"切换当前模型""cli-bin"残留。
7. [ ] **全量检查**(2.2 最后一轮):后端与前端全部验证命令;核对验收标准逐条打勾。

## 回滚

每步一个 commit,失败时 `git reset` 回上一 commit。风险集中在第 2/3 步的树编辑写回——fixture 测试兜底(apiKey/未知键逐字节保留),backup.go 机制不动。

## 边界备忘

- 不动:backup.go 备份/还原行为、apperr 错误码定义、其余 service 包。
- 不做:新增/删除供应商或模型条目、apiKey 相关任何读写、"当前选中模型"。
- 中间态说明:第 2 步后端合入时前端仍调旧 PUT /:name(TS 类型未变可编译,运行时表单为空),第 5 步修复——两 commit 间 web 运行态短暂不一致可接受,勿部署。
