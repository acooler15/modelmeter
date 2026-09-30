# Design:Agent 配置页修复

## D1 添加结果数组恒非 null(问题 1)

**根因**:`splitAddIDs`(agents/zcode.go)中 `skipped` 声明后仅在有跳过时 append,nil 切片被 `encoding/json` 序列化为 `null`。

**改动**:初始化 `skipped = make([]string, 0, len(modelIDs))`(与 `added` 同款)。三个 Agent 的 AddModels 全部经由此函数,一处修复覆盖全部路径;`added`/`entries` 已恒非 nil,不动。

**契约**:后端保证 `AddModelsResult` 三个数组字段 JSON 恒为 `[]`(非 null)。以测试锁定:对每个 Agent 的 AddModels 成功路径做 JSON marshal 断言,禁止出现 `"skipped":null` / `"added":null` / `"entries":null`。前端类型(`AgentAddModelsResult`)不变。

## D2 思考强度标准五档(问题 2)

**取值**:`low / medium / high / xhigh / max`(ZCode `optionSpecs.reasoningLevel.values` 的标准值域,与本机 ZCode/WorkBuddy/CodeBuddy 实测一致)。

**后端改动**(workbuddy.go / codebuddy.go,共用常量放 treeutil.go):

- 新增 `standardEffortLevels = []string{"low","medium","high","xhigh","max"}` 与合并函数 `mergeStandardEfforts(custom []string) []agentconf.FieldOption`:标准档在前,custom 中不属于标准档的按原顺序追加,保序去重。
- `workBuddyColumns` / `codeBuddyColumns`:
  - `default_effort` 恒为 `select`,Options = mergeStandardEfforts(collectEfforts())——删除"并集为空降级为 text"的分支;
  - `supported_efforts` list 列 Options = 同一合并结果(此前恒 nil)。
- `collectEfforts` 保持只读并集语义,继续供合并使用。

**前端零改动**:`AgentFieldInput` 的 select/list 本就渲染 `col.options` 且 list 支持自由输入。

## D3 清空 = 移除选项(问题 3)

### 前端契约(AgentFieldInput / AgentModelCard)

- **number 列清空**由"发布 undefined(放弃修改)"改为"发布 null(显式清除)";`emitNum` 是唯一出口,`setCell`/`setEditField` 仅在 undefined 时删键,null 保留进草稿。
- **valueChanged 语义更新**(AgentModelCard):
  - draft undefined → 放弃修改(不变);
  - draft null → 仅当 orig 存在值(orig 非 undefined/null)时视为改动,patch 携带 `fields[key] = null`;
  - 其余(text ''、list [])不变。
- list 列清空已发布 `[]`,无需改动。

### 后端 ZCode(zcode.go)

校验(validateZCodePatches):

- `context_window` / `max_output_tokens`:接受数字或 `nil`;其余类型仍 4403。
- `reasoning_levels`:接受非空字符串数组(设置)、空数组或 `nil`(清除);含非字符串仍 4403。
- `enabled` / 两个 bool:仍必须布尔(不接受 null)。

应用(applyZCodePatch,值为 nil 时走清除分支):

- `context_window` → 删除 `properties.contextWindow`,若 `properties` 因此变空则一并删除;
- `max_output_tokens` → 删除整个 `optionSpecs.maxOutputTokens` 节点(节点内的 `map` 等键属于该选项本身,随节点移除),若 `optionSpecs` 变空则一并删除;
- `reasoning_levels`(空数组或 nil)→ 删除整个 `optionSpecs.reasoningLevel` 节点,空容器同上。

> 选择"整节点移除"而非"仅删叶子键":残留 `{}` 或仅剩 `map` 的半截节点可能不满足 ZCode zod 对该节点的形状要求;整节点移除等价于"选项从未写过",正是智能配置接管所需状态。该取舍与 `defaultModelSelection` 整体替换的例外逻辑同向:节点内的未知键只可能是坏文件或选项自身组成部分。

流程不变:整体校验 → 备份 → 树编辑 → 原子写回 → 返回最新清单(清除后的条目不再携带该键)。

### 后端 WorkBuddy/CodeBuddy(workbuddy.go / codebuddy.go)

- 校验:三个数字键(max_input_tokens / max_output_tokens / temperature)接受数字或 `nil`;其余键类型要求不变。
- 应用:数字键值为 nil → 从条目删除该键(恢复"文件未写"缺省态);其余行为不变。
- list/text/bool 不接受 null(前端也不会发出)。

## 兼容性

- patch 契约向后兼容:null 仅新增为"清除"语义,既有客户端不发 null 时行为不变。
- 响应契约向后兼容:`null` → `[]` 对前端是纯修复(此前该形状直接抛错)。
- 配置文件兼容:清除只删除本系统白名单写入过的选项节点,凭据/未知顶层键零接触。
