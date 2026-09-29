# Agent 模型配置管理 — 执行清单

## 后端

1. [x] codes.go 登记 4401/4402/4403/4404(4xxx 段)。
2. [x] `internal/service/agentconf/agent.go`:Agent 接口、FieldSpec/Snapshot、registry(Register/Get/List)。
3. [x] `internal/service/agentconf/backup.go`:备份(时间戳 .bak)、还原最近、保留 10 份清理。
4. [x] `internal/service/agentconf/zcode.go`:homeDir 注入;Snapshot(provider 选项解析)/Apply(只改 selection,未知键保留)/校验。
5. [x] `internal/service/agentconf/workbuddy.go`:候选路径探测与 not_found 占位。
6. [x] `internal/service/agentconf/{zcode,workbuddy,backup}_test.go`:design.md 列的全部场景(t.TempDir 伪造 home)。
7. [x] `internal/handler/agent.go` 四端点 + router 挂载(dataDir 注入方式最小改动)。
8. [x] 验证:`go vet ./internal/... ./cmd/... && go test ./internal/... ./cmd/...`。

## 前端

9. [x] `web/src/types/agent.ts` + `web/src/api/agent.ts`。
10. [x] `web/src/views/agent/AgentView.vue`(卡片/动态表单对话框/还原/未找到态)。
11. [x] 路由 `/agents` + 菜单「Agent 配置」。
12. [x] 验证:`cd web && npm run lint && npm run build`。

## 冒烟

13. [x] 真实服务(临时 DATA_DIR):ZCode 卡片显示当前 provider/model(值来自真实 model-selection.json 的复制场景);修改写回后文件变化正确(options 保留);备份文件生成且还原可回滚;WorkBuddy 显示「未找到」;日志无凭据。

## 回滚点

- 步骤 1–8 与 9–12 各一个提交粒度;agentconf 包独立,可整体删除。
