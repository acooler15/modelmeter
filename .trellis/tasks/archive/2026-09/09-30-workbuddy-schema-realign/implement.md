# 执行计划

## 步骤

1. [ ] `workbuddy.go`:头部注释更新(新字段、双形态、canDisableThinking 缺省语义)。
2. [ ] 读侧重构:`readArray` → `readDoc`(返回 root any)+ `workBuddyModelsOf`;`collectEfforts`、`Models`、`ApplyModels` 改走新读取;`workBuddyEntry` 补 7 字段映射(含 `can_disable_thinking` 缺省 true、`default_effort` 兜底 `effort`)。
3. [ ] 写侧扩展:白名单常量补 7 键;`validateWorkBuddyPatches` 补布尔/数字校验;`applyWorkBuddyPatch` 补落点(disabled/onlyReasoning/useCustomProtocol 直落,三个数字键经 numberValue,canDisableThinking 落 reasoning 节点);ApplyModels 写回保持原顶层形态。
4. [ ] 列描述:`workBuddyColumns` 补 7 列(中文标签、类型如设计)。
5. [ ] 测试:
   - 夹具首条补新字段(disabled/onlyReasoning/useCustomProtocol/maxInputTokens/maxOutputTokens/temperature/reasoning.canDisableThinking),次条含 legacy `reasoning.effort`。
   - 新增:对象形态读 + 写回后 availableModels/未知顶层键保留;对象无 models 数组 4402;新字段读映射与缺省语义;新键写回与类型 4403;effort 兜底读取。
   - 改造:原"顶层是对象"4402 用例改为无 models 数组形态。
6. [ ] 全量检查:`go build ./...` && `go vet ./...` && `go test ./internal/service/agentconf/...`(按后端规范禁止裸 `go test ./...`)。

## 验证命令

```bash
go build ./...
go vet ./...
go test ./internal/service/agentconf/...
```

## 回滚点

- 步骤 1-4 任一步失败:`git checkout -- internal/service/agentconf/agents/workbuddy.go`。
- 用户配置安全:任何写回前已有 `agent-backups/workbuddy/` 自动备份,适配器只动白名单键。
