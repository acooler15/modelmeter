# ZCode 默认模型读写与白名单扩展 — 技术设计

勘探依据与安全红线见 research.md。共享决策:错误码沿用 4401-4404、新增 4405;backup.go / WriteJSONFile / 树编辑机制不动。

## 1. agentconf.go 接口扩展

```go
// DefaultModel "默认模型"现状:对应 ZCode config.defaultModelSelection。
type DefaultModel struct {
    ProviderID     string `json:"provider_id"`
    ModelID        string `json:"model_id"`
    ReasoningLevel string `json:"reasoning_level,omitempty"`
}

// Snapshot 新增两个字段(其余不动):
type Snapshot struct {
    // ...既有字段...
    SupportsDefaultModel bool          `json:"supports_default_model"`           // 能力位:该工具是否有"默认模型"概念
    DefaultModel         *DefaultModel `json:"default_model,omitempty"`          // 当前值,nil=未设置/不支持
}

// DefaultModelPatch 默认模型修改:provider_id+model_id 均空=清除,均非空=设置,混合=4403。
type DefaultModelPatch struct {
    ProviderID     string `json:"provider_id"`
    ModelID        string `json:"model_id"`
    ReasoningLevel string `json:"reasoning_level,omitempty"` // 仅设置时有效,空=不写 options
}

// DefaultModelSetter 可选能力接口:支持读写"默认模型"的 Agent 额外实现。
// handler 按类型断言探测;未实现 → 4405。能力位(Snapshot.SupportsDefaultModel)
// 与本接口实现必须一致,由 agents 包一致性测试兜底。
type DefaultModelSetter interface {
    // ApplyDefaultModel 读取-校验-备份-树编辑-原子写,返回写回后的最新 Snapshot。
    ApplyDefaultModel(ctx context.Context, patch DefaultModelPatch, dataDir string) (Snapshot, error)
}
```

- 读取走 Snapshot(不打新 GET 端点):ZCode 的 Snapshot 组装时解析 `config.defaultModelSelection`(解析失败/缺失 → nil,不报错,维持 Snapshot 不返回错误的约定)。
- WorkBuddy:SupportsDefaultModel=false、DefaultModel=nil、不实现接口——不新增死代码方法。

## 2. apperr 新增

```go
CodeAgentUnsupported = 4405 // 该 Agent 不支持所请求的操作(能力接口未实现)
```

codes.go 注释同步;httpStatusFor 中 4405 与其余 agentconf 码同走 400(实现时按 response 包既有映射规则落位)。

## 3. ZCode 适配(agents/zcode.go)

### 3.1 读侧扩展

- `readTree()` 增加防护:`schemaVersion` 存在且非数字 1 → 4402"配置 schemaVersion 不受支持(仅支持 1),请升级 ModelMeter 或在 ZCode 中重新生成";缺失则容忍(残缺文件仍可编辑)。
- `zcodeEntryFields()` 新增:
  - `supports_image` ← `config.properties.inputFormat.supportsImage`(bool,缺失不出现在 Fields);
  - `supports_json_schema_output` ← `config.properties.supportsJsonSchemaOutput`;
  - `reasoning_levels` ← `config.optionSpecs.reasoningLevel.values`([]string,保序)。
- `zcodeColumns()` 在 max_output_tokens 之后插入三列:`supports_image`(bool)、`supports_json_schema_output`(bool)、`reasoning_levels`(list,Options=全部规则 reasoningLevel.values 的保序去重并集,收集方式对齐 WorkBuddy collectEfforts)。
- Snapshot 组装:`SupportsDefaultModel: true`(found 与 not_found 均置位,能力是工具属性);found 时解析 `config.defaultModelSelection` → `DefaultModel`(providerId/modelId 取字符串,options.reasoningLevel 取字符串,残缺容忍)。

### 3.2 写侧扩展(ApplyModels)

白名单键与校验/落点新增:

| patch 键 | 类型校验 | 树落点 |
|---|---|---|
| supports_image | bool | `config.properties.inputFormat.supportsImage`(中间节点 ensureMap 创建) |
| supports_json_schema_output | bool | `config.properties.supportsJsonSchemaOutput` |
| reasoning_levels | []string(元素非空字符串) | `config.optionSpecs.reasoningLevel.values`([]any 落盘) |

- 校验错误文案沿用 4403 风格,默认值错误信息里列全白名单键。
- **manual 冲突守护**:`applyZCodePatch` 追加新节点前,先在 `config.modelConfigRules.manualProviderModelRules[]` 中按 (providerId, modelId) 查找——命中则直接编辑该节点(其 config 能力集与 providerModelRules 相同,可安全落白名单键);未命中才追加到 providerModelRules。

### 3.3 新增 ApplyDefaultModel

```
流程:readTree → 校验 patch → (有实际变化时)Backup → 树编辑 → WriteJSONFile → 返回 Snapshot()
校验:
  - 均空=清除;均非空=设置;混合 → 4403
  - 设置:(providerId, modelId) ∈ 模型清单 → 否则 4404;
    reasoning_level 非空且目标规则(或 manual 规则)定义 values 且不含该值 → 4403
清除:config.defaultModelSelection 存在才删除;不存在则不备份不落盘直接返回 Snapshot
设置:config.defaultModelSelection = {providerId, modelId}(+ reasoning_level 非空时 {"options":{"reasoningLevel":...}})
```

- **整体替换例外**:`defaultModelSelection` 节点(含 options 子节点)整体替换为规范形状,不保留节点内未知键——根 schema `.strict()`,节点内未知键会让 ZCode 拒载整份配置,保留坏键与零丢失目标相悖。代码注释必须说明这是红线清单上的显式例外。
- 日志:`slog.Info("ZCode 默认模型已更新", "action", "set"|"clear", "provider_id", ..., "model_id", ...)`——不记 reasoning_level 值?档位不是凭据,可记;但保持与"只记键名"基调一致,记 provider_id/model_id 即可。

## 4. WorkBuddy(agents/workbuddy.go)

- `workBuddyColumns()` 在 default_effort 后补 `{Key: "supported_efforts", Label: "支持档位", Type: "list"}`(白名单/apply 逻辑早已支持,纯展示层补列);Options=default_effort 同源并集可留空(前端自由输入)。
- Snapshot:SupportsDefaultModel=false;不实现 DefaultModelSetter。
- 测试补:设置 supported_efforts 的写回用例(行为此前已存在,补断言防回归)。

## 5. API 与前端

| 端点 | 变化 |
|---|---|
| GET /api/agents、GET /api/agents/:name | payload 增加 supports_default_model / default_model |
| PUT /api/agents/:name/default-model | 新增:body 即 DefaultModelPatch,返回最新 Snapshot(4401/4402/4403/4404/4405) |
| 其余端点 | 不变 |

前端:

- `types/agent.ts`:`AgentFieldSpec.type` 加 `'list'`;`AgentSnapshot` 加 `supports_default_model?: boolean`、`default_model?: AgentDefaultModel | null`;新增 `AgentDefaultModel`、`AgentDefaultModelPatch`(与后端 json 名逐字对齐)。
- `api/agent.ts`:`setDefaultAgentModel(name, patch): Promise<AgentSnapshot>`。
- `AgentModelCard.vue`:
  - `list` 列渲染:`el-select multiple filterable allow-create default-first-option`,选项=col.options,草稿值 string[],改动收集复用 valueChanged(数组走 JSON.stringify);新增 setList;
  - "默认模型"区(v-if `snapshot.supports_default_model && isFound`,置于表格上方):三个内联 el-select(供应商=条目去重 provider_id、模型=按供应商过滤、推理档位=所选条目 fields.reasoning_levels,均 allow-create)+ "保存" + "清除"(仅 default_model 存在时);清除二次确认;成功 `ElMessage` 提示"已写回,建议重启 ZCode 使配置生效"并 emit `defaultModelChanged`,父组件 runList() 刷新卡片;
  - 文案全中文。
- `AgentView.vue`:监听新事件刷新列表。

## 6. 测试要点

- `agents/zcode_test.go`:fixture `schemaVersion` 2→1;既有"原样保留"断言全部保留;新增:
  1. Snapshot 能力位/默认模型解析(无字段 → nil;有 → provider/model/level);
  2. ApplyDefaultModel 设置:树中新增规范节点、兄弟键与 apiKey 原样、产生备份;重复设置同值不产生第二次备份(无变化零落盘);
  3. ApplyDefaultModel 清除:节点删除;本无节点 → 不落盘;
  4. 错误:混合参数 4403、模型不存在 4404、档位越界 4403、schemaVersion=2 读写均 4402;
  5. 白名单三键:写回生效、缺失节点创建、既有兄弟键(reasoningLevel.map 等)保留、manual 冲突改 manual 节点;
  6. 能力一致性:`var _ agentconf.DefaultModelSetter = (*ZCodeAgent)(nil)`;ZCodeAgent 支持位 true。
- `agents/workbuddy_test.go`:supported_efforts 列存在且可写回;`ZCodeAgent` 断言反向:WorkBuddy 无能力位、不实现接口。
- `handler/agent_test.go`:PUT default-model 成功(返回 Snapshot)、未实现能力 4405、未知名称 4401、body 非法 400;fakeAgent 扩展实现/不实现两个变体。

## 7. 文档同步点

- README「6. Agent 模型配置管理」:管理范围表述改写("默认模型支持读取与设置/清除"),可编辑字段清单更新;
- agentconf.go / zcode.go / workbuddy.go 包注释:能力接口、整体替换例外、白名单变化;
- `.trellis/spec/backend/quality-guidelines.md`:涉及 agentconf 白名单的旧表述同步;
- 历史 research.md 不改写,勘误记录在本任务 research.md。
