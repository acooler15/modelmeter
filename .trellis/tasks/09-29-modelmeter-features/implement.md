# 父任务执行计划

父任务不直接承载实现,按子任务顺序推进;每个子任务走完自身 Phase 1–3 后归档,再启动下一个。

## 清单

- [x] 1. 创建任务树与共享规划文档
- [x] 2. 子任务 1:`09-29-interface-config`(实现→检查→归档)
- [x] 3. 子任务 2:`09-29-model-list`(实现→检查→归档)
- [x] 4. 子任务 3:`09-29-model-test`(实现→检查→归档)
- [x] 5. 子任务 4:`09-29-newapi-rates`(实现→检查→归档)
- [x] 6. 子任务 5:`09-29-agent-config`(实现→检查→归档)
- [x] 7. 集成回归:跨子任务验收标准逐项核验通过(假上游全旅程 + 重启持久化 + 日志无凭据)
- [x] 8. 规范沉淀:留空沿用 TrimSpace、原子写文件、SSE 信封例外、UTC NowFunc、
      凭据模型不嵌 gorm.Model、go test 范围、Element Plus 中文 locale 回写 .trellis/spec
- [x] 9. 提交与收尾

## 每个子任务的通用验证命令

```bash
go vet ./... && go test ./...
cd web && npm run lint && npm run build
```

## 集成冒烟(收尾时)

`go run ./cmd/server` 后按父任务 `prd.md` 的用户旅程逐项目验;同时检查日志无 API Key 明文、重启后数据仍在。

## 回滚点

每个子任务归档前保留一个完整提交粒度;子任务内部问题直接修复,不跨子任务回滚。
