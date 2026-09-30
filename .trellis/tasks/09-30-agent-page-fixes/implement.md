# Implement:Agent 配置页修复

## 执行清单(按序)

1. [ ] **D1 后端**:`splitAddIDs` 初始化 `skipped` 为空切片(agents/zcode.go);注释同步说明"恒非 nil,JSON 为 [] 非 null"。
2. [ ] **D1 测试**:三个 Agent 的 AddModels 成功路径各补 JSON 形状断言(marshal 后不含 `"skipped":null`/`"added":null`/`"entries":null`);至少一处覆盖"全部新模型无跳过"路径。
3. [ ] **D2 后端**:treeutil.go 增 `standardEffortLevels` 与 `mergeStandardEfforts`;workbuddy.go / codebuddy.go 的 columns 函数接入(default_effort 恒 select,supported_efforts 补 Options);文件头注释同步。
4. [ ] **D2 测试**:更新 workbuddy_test.go / codebuddy_test.go 中 default_effort 列断言(候选含五档、不再有 text 降级分支);补 supported_efforts 列 Options 断言;补"自定义档位追加在标准档之后"用例。
5. [ ] **D3 前端**:AgentFieldInput.vue `emitNum` 发布 null;AgentModelCard.vue `valueChanged` 支持 null=清除语义、`setCell`/`setEditField` 保留 null;相关注释同步。
6. [ ] **D3 后端 ZCode**:validateZCodePatches 接受 number|null 与空档位数组;applyZCodePatch 清除分支(删节点 + 空容器回收);zcode.go 文件头注释补清除语义。
7. [ ] **D3 后端 WB/CB**:validate/apply 接受数字键 null → 删键;文件头注释同步。
8. [ ] **D3 测试**:zcode_test 补清除用例(number null → 节点移除且空容器回收、空档位数组 → reasoningLevel 移除、清除路径有备份、清除后条目不再携带该键);workbuddy_test / codebuddy_test 补数字键 null → 删键用例与"null 落在文本/布尔键仍 4403"用例。
9. [ ] **全量校验**:`gofmt -l`、`go vet ./...`、`go test ./...`;web 下 `npm run lint`、`npm run build`(含 vue-tsc)。

## 验证命令

```bash
gofmt -l ./internal ./cmd
go test ./...
cd web && npm run lint && npm run build
```

## 回滚点

- 每步独立可回滚;整体为一个 commit,异常时 `git checkout -- <files>`。
- 后端行为变更均有既有备份机制兜底(agent-backups 10 份滚动)。
