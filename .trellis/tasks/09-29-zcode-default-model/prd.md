# ZCode 默认模型读写与模型能力白名单对齐真实 schema

## Goal

基于对 ZCode 官方打包代码(`zcode.cjs` zod schema)的第二轮深挖,补齐 ModelMeter ZCode 适配的三块缺口:① 读取并支持设置/清除"默认模型"(`config.defaultModelSelection`,推翻前任务"没有当前选中模型字段"的结论);② 模型规则可编辑白名单扩展至真实文件中已出现的能力键(`inputFormat.supportsImage`、`supportsJsonSchemaOutput`、`reasoningLevel.values`);③ schemaVersion 对齐真实约束(必须为 1)并加防护。文档随码勘误。

## Background(勘探勘误)

前任务 `09-29-agentconf-realign` 的 research.md 断言"**没有**'当前选中模型'字段",prd 也据此将"当前选中模型"明确出局。本轮反编译 `zcode.cjs` 证实:`config.defaultModelSelection = {providerId, modelId, options?: {reasoningLevel?}}` 是 schema 的一等字段,CLI `saveConfiguredDefault` 会写入;本机文件暂无该字段只是因为用户从未设置过默认模型。"文件里暂时没有"≠"schema 里不存在"——该决策需要回调。

## Requirements

### 功能

1. **默认模型读取**:`Snapshot` 新增 `supports_default_model`(能力位,ZCode 恒 true、WorkBuddy 恒 false)与 `default_model`(`{provider_id, model_id, reasoning_level?}` 或 null)。ZCode 从 `config.defaultModelSelection` 解析,字段缺失/未设置 → null。
2. **默认模型设置/清除**:
   - agentconf 新增可选能力接口 `DefaultModelSetter`(仅 ZCode 实现);handler 能力探测,未实现的 Agent 报新错误码 **4405**(CodeAgentUnsupported,"该工具不支持设置默认模型")。
   - 新端点 `PUT /api/agents/:name/default-model`,body `{provider_id, model_id, reasoning_level?}`:两者均空 → 清除(删除该字段,不存在则零变化直接返回);均非空 → 设置;一空一非空 → 4403。
   - 设置校验:(providerId, modelId) 必须存在于该供应商模型清单,否则 4404;`reasoning_level` 非空且目标模型规则定义了 `optionSpecs.reasoningLevel.values` 时必须取值于其中,否则 4403;未定义 values 则放行(schema 层面任意字符串合法)。
   - 写回语义:`defaultModelSelection` 节点**整体替换为规范形状**(schema `.strict()` 不允许未知键,节点内未知键只能是坏文件,保留反而会导致 ZCode 拒载整份配置——此处是"未知键零丢失"红线的显式例外,代码注释说明);写回前备份、原子写,返回最新 Snapshot。
3. **模型能力白名单扩展**(ZCode 模型规则,真实文件已观测到的键):
   - `supports_image`(bool)→ `config.properties.inputFormat.supportsImage`;
   - `supports_json_schema_output`(bool)→ `config.properties.supportsJsonSchemaOutput`;
   - `reasoning_levels`(字符串数组)→ `config.optionSpecs.reasoningLevel.values`;FieldSpec.Type 新增 `"list"`,Columns 的 Options 取全部规则 values 的保序并集(空则前端自由输入)。
   - Columns、读侧 Fields、写侧白名单校验/落点同步扩展;`FieldSpec` 类型注释更新。
4. **manual 规则冲突守护**:追加新规则节点前先查 `manualProviderModelRules`——同 (providerId, modelId) 已在 manual 数组时改编辑该节点,严禁在两个数组重复落键(zod superRefine 会拒收整份文件)。manual 数组本身不展示、不主动编辑,原样保留。
5. **schemaVersion 防护**:`readTree` 校验 `schemaVersion` 存在且必须为数字 1,否则 4402 拒绝读写("schemaVersion 不受支持");测试 fixture 由 2 修正为 1。
6. **WorkBuddy 对称小修**:`supported_efforts` 白名单键已有但无列(不可见不可编辑)——补 `list` 列使其可用;其余 WorkBuddy 行为不变,且不实现 DefaultModelSetter。
7. **前端**:类型与 API 同步(`supports_default_model`、`default_model`、PUT default-model);模型卡片新增"默认模型"区(供应商/模型/推理档位三级选择 + 清除,选项来自模型清单,保存后刷新卡片并提示重启生效);表格支持 `list` 列渲染(多选、可自由添加);全中文。
8. **文档勘误**:README 第 6 节"当前选中模型不在管理范围"改写为支持读取/设置/清除;可编辑范围清单更新;新任务 research.md 留存勘误记录(不改写历史任务的 research.md)。

### 非功能

9. **安全红线不变**:绝不读取/回传/写日志/写回凭据(access.apiKey、credentials.json 零接触);白名单外的键逐字节原样保留(唯一例外见需求 2 的 defaultModelSelection 整体替换);写回前备份(10 份滚动)、原子写;日志只记键名不记值。
10. 解析容忍:defaultModelSelection 残缺(缺 options、缺字段)、规则节点缺失、未知键,均不报错、无损保留。
11. `go vet` / `go test ./internal/... ./cmd/...`;`cd web && npm run lint && npm run build` 全绿。

## Acceptance Criteria

- [ ] Snapshot 携带 `supports_default_model` 与 `default_model`;对真实结构 fixture:无 defaultModelSelection → null,有 → 正确解析(含 reasoning_level)
- [ ] PUT /api/agents/zcode/default-model:设置/清除生效且写回后除目标节点外语义零变化(apiKey、templateId、manualProviderModelRules、兄弟键原样);错误 4403(混合参数/档位越界)、4404(模型不存在)、4405(WorkBuddy)中文提示
- [ ] 新白名单三键读写正确:不存在节点时创建最小节点,既有节点只动目标路径;白名单外键仍 4403
- [ ] 同 (providerId, modelId) 在 manual 数组已存在时,patch 落在 manual 节点而非重复追加
- [ ] schemaVersion=2 的文件拒绝读写(4402);fixture 修正为 1 且写回后保持 1
- [ ] WorkBuddy:supported_efforts 列可编辑;不实现 DefaultModelSetter;能力位与接口实现一致性有测试
- [ ] 前端默认模型区可用(设置/清除/档位选择),list 列多选可编辑;`npm run lint` + `npm run build` 通过
- [ ] README、agents 包注释勘误完成;检查命令全绿

## Notes

- 勘探依据(zcode.cjs schema 结论、defaultModelSelection 结构、真实文件样例)见 research.md;技术方案见 design.md。
- 前任务 `09-29-agentconf-realign` 尚未归档(其实现已提交),本任务不含归档动作。
