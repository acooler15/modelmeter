# New API 模型费率 — 执行清单

## 后端

1. [ ] `internal/model/setting.go` + main.go AutoMigrate 登记。
2. [ ] codes.go 登记 3401/3500/3501/3502(3xxx 段)。
3. [ ] llmclient:`GetJSONWithCodes` 参数化错误码,`GetJSON` 变薄封装(既有测试不动)。
4. [ ] `internal/service/newapi.go`:配置存取(脱敏/留空沿用)、ListRates(降级解析)、EstimateCost(两种计费口径)。
5. [ ] `internal/service/newapi_test.go`:design.md 列的全部场景。
6. [ ] `internal/handler/newapi.go` 四端点 + router 挂载。
7. [ ] 验证:`go vet ./internal/... ./cmd/... && go test ./internal/... ./cmd/...`。

## 前端

8. [ ] `web/src/types/newapi.ts` + `web/src/api/newapi.ts`。
9. [ ] `web/src/views/rate/RateView.vue`(配置卡片 + 费率表格 + 过滤 + 空态引导)。
10. [ ] ModelTestView.vue 接成本估算(成功测试且有用量时)。
11. [ ] 路由 `/rates` + 菜单「模型费率」。
12. [ ] 验证:`cd web && npm run lint && npm run build`。

## 冒烟

13. [ ] node/httptest 假 New API:配置保存→拉取费率表(含缺字段条目)→过滤;未配置 3401、错令牌 3501、拒连 3500 文案区分;测试页完成一次带用量测试后成本行出现估算值;令牌不进日志、GET 配置返回脱敏值;重启后配置仍在。

## 回滚点

- 步骤 1–7 与 8–12 各一个提交粒度;settings 表独立,回滚不影响其他功能。
