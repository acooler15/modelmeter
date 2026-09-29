# agentconf 对齐真实配置 — 技术设计

勘探结论与安全红线见 research.md。共享决策:错误码沿用 4401-4404;备份/还原机制(backup.go)不动。

## 目录与注册

```
internal/service/agentconf/
├── agentconf.go        # 包文档 + Agent/ModelEntry/ModelPatch/Snapshot/FieldSpec + 注册表
├── backup.go           # 不动(Backup/RestoreLatest/copyFile)
└── agents/             # 唯一子目录:具体实现
    ├── register.go     # init():os.UserHomeDir() → 注册两个实现
    ├── zcode.go        # ZCodeAgent
    ├── zcode_test.go
    ├── workbuddy.go    # WorkBuddyAgent
    └── workbuddy_test.go
```

- **循环依赖约束**:agents → agentconf(取类型与 Backup),agentconf 不 import agents。
- 注册保持既有"实现包 init 自注册"约定:agents/register.go 的 init() 取 homeDir 并 `agentconf.Register(...)`(homeDir 获取失败置空,实现内按 not_found 降级);cmd/server/main.go 增加空导入 `_ ".../internal/service/agentconf/agents"` 触发;原 agentconf 根包的 init() 删除。
- 原根包的 fileExists/writeJSONFile/marshalPlainString 被 backup.go 与实现共用 → 留在根包导出为 `FileExists` / `WriteJSONFile`(marshalPlainString 成为 WriteJSONFile 内部细节);backup.go 改用导出名,行为不变。
- handler 与前端 API 路径除 models 两个新端点外不变。

## 接口演化(单值表单 → 模型清单)

```go
// FieldSpec 沿用,Type 扩展,新增 Readonly:既描述"列"也承载可编辑性。
type FieldSpec struct {
    Key      string        `json:"key"`
    Label    string        `json:"label"`
    Type     string        `json:"type"`   // "text"|"select"|"number"|"bool"
    Options  []FieldOption `json:"options,omitempty"`
    Required bool          `json:"required"`
    Help     string        `json:"help,omitempty"`
    Readonly bool          `json:"readonly,omitempty"`
}

// Snapshot 现状视图:去掉 Fields/Values,新增 Columns。
type Snapshot struct {
    Name        string      `json:"name"`
    DisplayName string      `json:"display_name"`
    Status      string      `json:"status"`
    ConfigPath  string      `json:"config_path"`
    Columns     []FieldSpec `json:"columns"`
    Message     string      `json:"message,omitempty"`
}

// ModelEntry 清单中的一个模型(凭据绝不包含)。
type ModelEntry struct {
    ProviderID   string         `json:"provider_id,omitempty"` // ZCode 有供应商层;WorkBuddy 为空
    ProviderName string         `json:"provider_name,omitempty"`
    ModelID      string         `json:"model_id"`
    DisplayName  string         `json:"display_name,omitempty"`
    Fields       map[string]any `json:"fields"` // 非凭据字段:展示 + 可编辑,键集=Columns 的 key 子集
}

// ModelPatch 对一个模型条目的白名单局部修改。
type ModelPatch struct {
    ProviderID string         `json:"provider_id,omitempty"` // ZCode 定位键;WorkBuddy 为空
    ModelID    string         `json:"model_id"`
    Fields     map[string]any `json:"fields"` // 仅白名单键;含白名单外键 → 4403
}

type Agent interface {
    Name() string
    DisplayName() string
    Snapshot(ctx context.Context) Snapshot
    Models(ctx context.Context) ([]ModelEntry, error)
    ApplyModels(ctx context.Context, patches []ModelPatch, dataDir string) ([]ModelEntry, error)
}
```

- 前端表格列由 Snapshot.Columns 驱动:bool→el-switch、number→el-input-number、select→el-select、text→el-input,Readonly 列只展示。新增 Agent 无需改前端。
- ApplyModels 收到多 patch 时**一次备份、一次读、一次写**;返回重新读取的最新清单。
- 定位语义:ZCode 用 (providerId, modelId);WorkBuddy 用 modelId(取 id 首个匹配,找不到 → 4404)。

## ZCode 实现(树编辑,零丢失写回)

目标:`~/.zcode/v2/provider_config.json`。红线:凭据字段不读不传不写不记日志。

**读侧 Models()**:
- providerOrder 保序遍历,providerRules 建 id→rule 映射(缺名回退 id);残缺条目(无 config/api)容忍为空值;
- 模型清单 = config.modelOrder ∪ config.personalModelIds(保序去重);与 providerModelRules 按 (providerId, modelId) 关联;
- Fields:`enabled`(规则节点缺省 true)、`context_window`、`max_output_tokens`(取 optionSpecs.maxOutputTokens.max)、`provider_enabled`(只读)、`api_type`、`base_url`(只读);数字一律 json.Number 透出;
- Columns:provider_name(readonly)、model_id(readonly)、enabled(bool)、context_window(number)、max_output_tokens(number)、provider_enabled(readonly)、api_type(readonly)、base_url(readonly)。

**写侧 ApplyModels()**(整体流程):
1. 校验:Fields 键 ∈ {enabled, context_window, max_output_tokens},越键 → 4403;(providerId, modelId) 必须存在于供应商模型清单,否则 4404;
2. `Backup(...)` → dataDir/agent-backups/zcode/;
3. 读文件 → `json.Decoder` + `UseNumber()` → `map[string]any` 树(UseNumber 保证任意整数零精度丢失);
4. 定位 `config.modelConfigRules.providerModelRules[]` 中 (providerId, modelId) 条目:改 `config.enabled` / `config.properties.contextWindow` / `config.optionSpecs.maxOutputTokens.max`(中间节点缺失则创建);无条目则追加最小节点 `{providerId, modelId, config:{...}}`;树中其余节点(含 access.apiKey、templateId、manualProviderModelRules 等未知键)**原样放回**;
5. `WriteJSONFile` 原子写(2 空格缩进、SetEscapeHTML(false)、临时文件+改名)。map 键按字母序重排——沿用 P1-6 既有取舍(语义零变化),包注释说明;
6. 返回 `Models()` 最新清单;slog 只记 provider_id/model_id 与改动键名,不记值。

## WorkBuddy 实现(同法)

目标:`~/.workbuddy/models.json`(裸数组,每条独立明文 apiKey)。

- 读:数组遍历 → ModelEntry{ModelID: id, DisplayName: name, Fields: vendor/url/supports_tool_call/supports_images/supports_reasoning/default_effort/supported_efforts};reasoning 节点缺失时 Fields 相应键缺省;
- 写:按 modelId 首个匹配定位数组元素;Fields→文件键映射:name/url/supportsToolCall/supportsImages/supportsReasoning/reasoning.defaultEffort/reasoning.supportedEfforts(数组);id/vendor/apiKey/未知键原样;reasoning 节点缺失时创建;定位失败 → 4404;
- Columns:model_id(readonly)、display_name→name(text)、url(text)、三个能力开关(bool)、default_effort(select,选项=supportedEfforts,空则 text)、vendor(readonly)。

## API 与前端

| 端点 | 变化 |
|---|---|
| GET /api/agents | Snapshot 瘦身(+columns) |
| GET /api/agents/:name | 同上 |
| GET /api/agents/:name/models | 新增:[]ModelEntry(4401/4404) |
| PUT /api/agents/:name/models | 新增:`{patches:[...]}` → 最新 []ModelEntry(4401/4402/4403/4404) |
| PUT /api/agents/:name | 删除(表单写回废弃) |
| POST /api/agents/:name/restore | 不变 |

前端(web/src/types/agent.ts、api/agent.ts 重写,views/agent/AgentView.vue 重做):
- 卡片:显示名/状态/ConfigPath/Message;found 时加载模型表格;
- 通用表格组件按 Columns 渲染,行=ModelEntry;改动收集为 patches 批量 PUT;成功后刷新并提示"已写回,建议重启对应工具使配置生效";
- 还原按钮保留(确认框);错误经统一信封展示中文提示;
- 类型与后端 json 名严格对齐(provider_id/model_id/…),遵守前端 type-safety 规范。

## 测试要点

- agents/zcode_test.go(temp home + fixture,fixture 含 apiKey、templateId、manualProviderModelRules、残缺 provider):
  1. Models 输出与 Snapshot.Columns 均不含任何凭据;
  2. ApplyModels 改 enabled/context_window/max_output_tokens 后重新解析 fixture:apiKey 值不变、未知键仍在、manualProviderModelRules 原样、目标键生效、无规则节点的模型能创建最小节点;
  3. 白名单外键 → 4403;不存在的 (providerId, modelId) → 4404;文件缺失 → 4404;
- agents/workbuddy_test.go(数组 fixture 含 apiKey 与未知键):同上,外加 id 不存在 → 4404、reasoning 节点创建;
- handler/agent_test.go:models 两端点的路由/信封/错误码;删除旧 PUT /:name 用例;
- backup_test.go 不动。

## 文档同步点

- README「6. Agent 模型配置管理」:目标文件、可编辑范围、"当前选中模型不在管理范围";
- agentconf.go / agents 包注释:扩展点说明改为"实现 Agent 接口,在 agents/ 子包注册";
- `.trellis/spec/backend/quality-guidelines.md` 中引用旧 model-selection.json 行为处(约 39 行)更新。
