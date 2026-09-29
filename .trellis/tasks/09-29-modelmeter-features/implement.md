# 父任务执行计划

父任务不直接承载实现,按子任务顺序推进;每个子任务走完自身 Phase 1–3 后归档,再启动下一个。

## 清单

- [x] 1. 创建任务树与共享规划文档
- [ ] 2. 子任务 1:`09-29-interface-config`(启动前补齐 design/implement → 实现 → 检查 → 归档)
- [ ] 3. 子任务 2:`09-29-model-list`(同上)
- [ ] 4. 子任务 3:`09-29-model-test`(同上)
- [ ] 5. 子任务 4:`09-29-newapi-rates`(同上)
- [ ] 6. 子任务 5:`09-29-agent-config`(同上)
- [ ] 7. 集成回归:按父任务 `prd.md` 跨子任务验收标准逐项核验
- [ ] 8. 规范沉淀:实现中的新约定回写 `.trellis/spec/`(trellis-update-spec)
- [ ] 9. 提交与收尾

## 每个子任务的通用验证命令

```bash
go vet ./... && go test ./...
cd web && npm run lint && npm run build
```

## 集成冒烟(收尾时)

`go run ./cmd/server` 后按父任务 `prd.md` 的用户旅程逐项目验;同时检查日志无 API Key 明文、重启后数据仍在。

## 回滚点

每个子任务归档前保留一个完整提交粒度;子任务内部问题直接修复,不跨子任务回滚。
