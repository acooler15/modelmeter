# 模型列表 — 执行清单

## 后端

1. [ ] codes.go 登记 1500/1501/1502(1xxx 段注释同步)。
2. [ ] `internal/service/llmclient/client.go`:NormalizeBaseURL/BuildURL/GetJSON + 错误归类。
3. [ ] `internal/service/llmclient/client_test.go`:URL 三形态 + 五类响应归类。
4. [ ] `internal/service/modelcatalog.go` ListModels + 降级解析。
5. [ ] `internal/service/modelcatalog_test.go`:httptest 四场景。
6. [ ] handler `ProviderModels` + router 挂载。
7. [ ] 验证:`go vet ./internal/... ./cmd/... && go test ./internal/... ./cmd/...`。

## 前端

8. [ ] `web/src/stores/providers.ts`(首个 Pinia store,遵循 state-management 规范)。
9. [ ] ProviderList.vue 数据源迁移到 store(保持行为不变)。
10. [ ] `web/src/types/model.ts` + `web/src/api/modelCatalog.ts`。
11. [ ] `web/src/views/model/ModelListView.vue`(选择/拉取/过滤/展开元信息/空态引导)。
12. [ ] 路由 `/models` + 菜单「模型列表」。
13. [ ] 验证:`cd web && npm run lint && npm run build`。

## 冒烟

14. [ ] 起 `go run ./cmd/server`:无配置引导 → 添加配置 → 拉取模型列表/刷新/过滤;用一个假上游(httptest 或错误地址)验证网络错误与 401 文案区分;日志无 Key。

## 回滚点

- 步骤 1–7、8–13 各一个提交粒度;llmclient 为新包可整体删除。
