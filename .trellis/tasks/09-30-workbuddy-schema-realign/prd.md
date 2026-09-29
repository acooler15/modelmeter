# WorkBuddy 模型配置适配对齐真实 schema

## Goal

ModelMeter 的 WorkBuddy 适配器(`internal/service/agentconf/agents/workbuddy.go`)按 WorkBuddy 桌面端真实代码(勘探见 research.md)补齐模型配置的结构参数:读侧补齐 7 个缺失字段,写侧白名单同步扩展,支持对象形态 `{models, availableModels}` 文件的零丢失读写。功能语义与 ZCode 适配器对齐:白名单局部修改、备份先于写回、凭据零接触。

## Requirements

1. **读侧字段补齐**:Models/Snapshot 返回的非凭据字段补齐 `disabled`、`only_reasoning`、`use_custom_protocol`、`max_input_tokens`、`max_output_tokens`、`temperature`、`can_disable_thinking`;`default_effort` 读取兜底 legacy 键 `reasoning.effort`。
2. **缺省语义**:`disabled`/`only_reasoning`/`use_custom_protocol` 缺省 false;`can_disable_thinking` 缺省 true(WorkBuddy 仅 false 才落盘);`max_input_tokens`/`max_output_tokens`/`temperature` 文件未写时该键缺省不出现(与 ZCode 数字列口径一致)。
3. **写侧白名单扩展**:上述 7 键纳入 ApplyModels 白名单;布尔键必须为布尔、数字键必须为数字,类型不符报 4403;白名单外的键(含 apiKey/id/vendor/tags)仍报 4403。
4. **对象形态支持**:配置文件为 `{models: [...], availableModels?: [...]}` 对象形态时与裸数组同等读写;写回保持原顶层形态,对象形态下 `availableModels` 等其余顶层键零丢失;对象缺少 models 数组(或 models 不是数组)仍报 4402。
5. **不改变既有约定**:定位仍按 id 首个匹配;空 patch 只读不落盘;备份先于写回落 `dataDir/agent-backups/workbuddy/`;2 空格缩进原子写;日志只记键名;`SupportsDefaultModel` 恒 false 不实现 DefaultModelSetter;错误码沿用 4402/4403/4404。

## Constraints

- 纯后端改动:`workbuddy.go` + `workbuddy_test.go`;前端 `AgentModelCard.vue` 按 Columns 通用渲染,零改动。
- 凭据红线:apiKey 不读取进任何输出结构、不写日志、不改写(WBEF1 密文或 `${ENV}` 占位符逐字节保留)。
- 不跟随 `WORKBUDDY_CONFIG_DIR` 环境变量重定向,固定 `~/.workbuddy/models.json`(与 ZCode 适配器口径一致)。

## Acceptance Criteria

- [ ] 夹具含全部新字段的文件,Models 返回映射正确的完整字段集;apiKey 及未知键不出现在任何输出。
- [ ] ApplyModels 可成功写回 7 个新键(布尔/数字类型各自校验),对象形态文件写回后 `availableModels` 与未知顶层键原样保留。
- [ ] 裸数组形态既有行为全部不回归(现有测试改造后全绿)。
- [ ] 白名单外键、类型错误报 4403;定位不存在报 4404;文件缺失 4404;对象无 models 数组 4402。
- [ ] `go build ./...`、`go vet ./...`、`go test ./internal/service/agentconf/...` 全部通过。
