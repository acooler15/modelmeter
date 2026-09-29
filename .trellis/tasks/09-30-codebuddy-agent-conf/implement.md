# implement.md — CodeBuddy 模型配置适配

## 执行清单

1. [ ] `internal/service/agentconf/agents/codebuddy.go`
   - 文件头注释:目标文件、schema 勘探出处(本任务 research.md)、读写
     两侧口径与安全红线。
   - `CodeBuddyAgent`(homeDir 注入)+ `NewCodeBuddyAgent`。
   - `Name`/`DisplayName`/`modelsPath`/管理提示常量/白名单键常量
     (cb 前缀,与 wb/zc 隔离)。
   - `Snapshot`(found/not_found、Columns、collectEfforts 选项)。
   - `Models`/`ApplyModels`(校验→空 patch 短路→备份→apply→回填
     models→原子写→重读)。
   - `readDoc`/`codeBuddyModelsOf`(仅对象形态;缺 models 按空清单)/
     `codeBuddyEntries`/`codeBuddyEntry`(缺省语义+effort 兜底)/
     `locateCodeBuddy`/`validateCodeBuddyPatches`/`applyCodeBuddyPatch`。
2. [ ] `register.go`:追加 `agentconf.Register(NewCodeBuddyAgent(home))`
   并更新包注释中的工具清单。
3. [ ] `codebuddy_test.go`:夹具(对象形态、含 availableModels/version
   未知顶层键、完整字段条目+缺 reasoning 条目+无 id 条目)与用例:
   - 清单映射与脱敏(含 effort 兜底、数字键缺省不出现、无 id 条目跳过)。
   - Snapshot 列描述与下拉选项、文件缺失 not_found。
   - ApplyModels 白名单写回零丢失(apiKey/id/vendor/unknownKey/
     availableModels/未提交键/未命中条目)、新增字段写回、reasoning 节点
     创建、supported_efforts 写回([]any 与 []string 两形态)。
   - 白名单外键 4403、类型错 4403、id 不存在 4404、文件缺失 4404、
     非法 JSON 4402、**裸数组顶层 4402**、对象缺 models 按空清单
     (Models 返回空、patch 4404)、空 patch 不落盘。
   - 能力一致性(能力位 false、不实现 DefaultModelSetter)。
4. [ ] 更新 `web/src` 中提及"仅 ZCode、WorkBuddy"的注释(可选,仅注释)。

## 验证命令

```
mise exec -- go build ./...
mise exec -- go vet ./...
mise exec -- go test ./internal/service/agentconf/... 
mise exec -- go test ./internal/handler/...
```

(以 .trellis/spec/backend/quality-guidelines.md 的全量检查命令为准。)

## 审查关口

- 实现后跑 trellis-check 全量核对:research 第 6 节口径逐条对照、
  凭据零接触、日志无值。
- 提交信息:`feat: CodeBuddy 模型配置适配对齐真实 schema`。

## 回滚点

步骤 1-3 均为新增文件/一行注册,`git checkout --` 即回滚;无数据迁移。
