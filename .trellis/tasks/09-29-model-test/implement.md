# 模型测试 — 执行清单

## 后端

1. [ ] `internal/model/test_record.go` + main.go AutoMigrate 登记。
2. [ ] codes.go 登记 2401/2500/2502(2xxx 段)。
3. [ ] `internal/service/llmclient/sse.go` 通用 SSE 解析器(先行,供三协议复用)。
4. [ ] `internal/service/llmclient/protocol.go`:DoChat/StreamChat + 三协议实现(chat_completions → responses → anthropic 逐个实现逐个测)。
5. [ ] `internal/service/llmclient/protocol_test.go`:三协议全量/流式/错误分类/anthropic 默认 max_tokens。
6. [ ] `internal/service/modeltest.go`:TestInput 校验、RunTest、StreamTest、200 条清理(成败均落库)。
7. [ ] `internal/service/modeltest_test.go`:落库与清理。
8. [ ] `internal/handler/modeltest.go` 四端点 + router 挂载。
9. [ ] 验证:`go vet ./internal/... ./cmd/... && go test ./internal/... ./cmd/...`。

## 前端

10. [ ] `web/src/types/test.ts` + `web/src/api/modelTest.ts`。
11. [ ] `web/src/composables/useTestStream.ts`(分帧缓冲、错误信封、abort)。
12. [ ] `web/src/views/test/ModelTestView.vue`(表单/结果/记录三区,全中文)。
13. [ ] 路由 `/test` + 菜单「模型测试」。
14. [ ] 验证:`cd web && npm run lint && npm run build`。

## 冒烟

15. [ ] 假上游(httptest 脚本或 node)模拟三协议 + SSE:流式逐字渲染、非流式完整返回、401/拒连/坏结构文案区分;真实服务落库后重启,记录仍在;日志无 Key;失败测试也产生记录。

## 回滚点

- 步骤 1–9 与 10–14 各一个提交粒度;protocol/sse 为新增文件可整体删除。
