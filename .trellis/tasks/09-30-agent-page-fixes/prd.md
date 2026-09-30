# Agent 配置页修复:添加弹窗不关闭、思考强度五档、ZCode 选项清除

## Goal

修复从接口添加模型成功后弹窗不关闭且清单不刷新的 bug;WorkBuddy/CodeBuddy 思考强度两列提供标准五档候选;ZCode 模型选项清空时从规则节点移除该选项,交由 ZCode 智能配置补默认值。

## Background(勘探结论)

1. **添加弹窗 bug**:三个 Agent 共用的 `splitAddIDs`(agents/zcode.go)中 `skipped` 切片初始为 nil,当本次添加无任何"已存在跳过"时 JSON 序列化为 `"skipped": null`;前端 `AgentImportModelsDialog.vue` 的 `result.skipped.length` 抛 TypeError,中断了成功提示、`imported` 事件(清单刷新)与 `update:visible=false`(关弹窗)。模型实际已落盘成功,用户侧表现为"点了添加没任何反应"。此前"全部已存在"路径 skipped 非空、added 恒为 `make(...)` 非 nil,故该路径未暴露。
2. **思考强度档位**:WorkBuddy/CodeBuddy 的"支持档位"(supported_efforts,list 列)目前无候选项只能手输;"默认推理强度"(default_effort,select 列)选项仅取文件内已有条目 supportedEfforts 的并集,新添加模型(无 reasoning 节点)文件并集为空时降级为纯文本输入。ZCode 的标准值域为 5 档(low/medium/high/xhigh/max,见 09-29-agentconf-realign 与 09-29-zcode-default-model 的 research.md)。
3. **ZCode 选项清除**:ZCode 模型行的可选列(上下文窗口/最大输出 Token 为 number,推理档位为 list)一旦写过值就无法恢复"未设置"状态——number 清空在前端被视为"放弃修改"(不产生 patch,list 清空为 `[]` 被后端 4403 拒绝)。选项节点残留会让 ZCode 不走"智能配置"补默认值。

## Requirements

### R1 添加模型成功后正常关弹窗并刷新清单

- `AddModelsResult` 的 `added`/`skipped` 数组字段在后端恒序列化为 `[]` 而非 `null`(成功路径与全部跳过路径均成立),`entries` 恒非 null。
- 前端成功分支不因响应形状抛错:成功提示、清单刷新、弹窗关闭按既有流程执行。

### R2 WorkBuddy/CodeBuddy 思考强度两列提供标准五档候选

- `supported_efforts`(list 列)与 `default_effort`(select 列)的候选项为标准五档 `low / medium / high / xhigh / max`,并合并文件内已出现的自定义档位(保序去重,标准档在前)。
- 保留自由输入能力(list 列 allow-create),不影响已有自定义档位的读写。
- `default_effort` 不再因文件并集为空降级为文本输入(候选恒非空)。

### R3 ZCode 可选列清空 = 移除选项节点

- 清空"上下文窗口 / 最大输出 Token"(number)与"推理档位"(list 清空为空数组)时,前端提交显式清除补丁,后端从该模型规则节点移除对应选项(`properties.contextWindow`、`optionSpecs.maxOutputTokens`、`optionSpecs.reasoningLevel`),并清掉因移除而变空的中间容器(如空的 `optionSpecs`),使文件回到"选项未写"状态,由 ZCode 智能配置补默认值。
- 清除动作同样先整体校验再一次备份一次写回,遵守既有安全红线(不触碰凭据、白名单外键原样保留、备份先于写回)。
- WorkBuddy/CodeBuddy 同口径支持 number 列显式清除(null → 移除该数字键),避免前端统一"清空=null"契约后这三列被 4403 拒绝;其文本/布尔/list 列行为不变。

## Out of Scope

- ZCode 布尔列(图像输入/JSON Schema 输出/启用)不支持清除——开关没有"空"态,保持 true/false 显式写入。
- WorkBuddy/CodeBuddy 的 list/text 列清除语义(维持现状:`[]` 照写、`''` 照写)。
- 前端单测(项目前端无测试基建,以 lint + vue-tsc 类型检查为门禁)。

## Acceptance Criteria

- [ ] 任一 Agent 添加全部新模型(无跳过)成功后:弹窗自动关闭、清单刷新、出现成功提示;回归测试锁定 `AddModelsResult` JSON 中 `added`/`skipped` 为 `[]` 而非 null。
- [ ] WorkBuddy 与 CodeBuddy 的 supported_efforts/default_effort 两列候选含标准五档(合并文件自定义档位);既有测试更新并通过。
- [ ] ZCode:对已写过 context_window/max_output_tokens 的模型提交清除 patch 后,规则节点中对应选项与空容器被移除;reasoning_levels 提交空数组后 `optionSpecs.reasoningLevel` 被移除;校验接受 number|null 与空档位数组;写回前有备份。
- [ ] WorkBuddy/CodeBuddy:对已写过数字键的模型提交 null patch 后该键被移除;文本/布尔/list 校验行为不变。
- [ ] `go test ./internal/... ./cmd/...`、`gofmt`、前端 `npm run lint` 与 `vue-tsc`(npm run build)全绿。
