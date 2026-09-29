# 接口配置管理 — 执行清单

按序执行,每步可独立验证;遵循 .trellis/spec/backend 与 frontend 规范(已在 implement.jsonl 登记)。

## 后端

1. [ ] `internal/apperr/codes.go` 登记 1401/1402/1404(注释保持分段约定)。
2. [ ] `internal/model/provider.go` 定义 Provider(json tag 按 design.md;APIKey `json:"-"`)。
3. [ ] `cmd/server/main.go` mustOpenDB 的 AutoMigrate 登记 Provider。
4. [ ] `internal/service/provider.go` 实现 List/Create/Update/Delete + MaskKey + 校验。
5. [ ] `internal/service/provider_test.go` 单测(design.md 列的五类场景)。
6. [ ] `internal/handler/provider.go` 五端点 + `router.go` 挂载。
7. [ ] 验证:`go vet ./... && go test ./...`。

## 前端

8. [ ] `web/src/api/http.ts` 补 httpPut/httpDelete(如缺)。
9. [ ] `web/src/types/provider.ts` + `web/src/api/provider.ts`。
10. [ ] `web/src/views/provider/ProviderList.vue`(表格/对话框/确认/空状态,全中文)。
11. [ ] 路由 `/providers` + MainLayout 菜单「接口配置」。
12. [ ] 验证:`cd web && npm run lint && npm run build`。

## 冒烟(收尾前)

13. [ ] `go run ./cmd/server` 后人工/脚本走查:新增 → 列表脱敏 → 编辑留空 key → 删除;重启后数据仍在;日志无 key 明文。

## 回滚点

- 步骤 1–7 与 8–12 各为一个提交粒度;异常时按文件粒度回退(新增文件直接删,挂载点还原)。
