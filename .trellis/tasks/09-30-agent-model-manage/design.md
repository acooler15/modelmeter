# design.md — Agent 配置页 tab 化与模型清单管理增强

> 技术设计。需求与决策见 `prd.md`(D1 凭据写入 / D2 ZCode 挂已有或新建供应商 / D3 行内+操作列 / D4 重复跳过)。schema 依据:归档勘探 `09-29-agentconf-realign`、`09-30-workbuddy-schema-realign`、`09-30-codebuddy-agent-conf` 的 research.md。

## 总体

后端在 `agentconf` 的 Agent 接口上新增"删除模型"与"添加模型"两个能力(三个适配器均实现),handler 新增两个路由;"添加"由 handler 从数据库装配来源接口(含凭据)传给适配器,凭据只进不出。前端 tab 化 + 操作列 + 两个弹窗,复用现有 `useRequest`/列驱动渲染模式。API 全部为新增端点,无破坏性变更。

## 后端

### 1. 类型与接口扩展(`internal/service/agentconf/agentconf.go`)

```go
// ModelRef 删除目标定位:provider_id 仅 ZCode 有,WorkBuddy/CodeBuddy 为空。
type ModelRef struct {
    ProviderID string `json:"provider_id,omitempty"`
    ModelID    string `json:"model_id"`
}

// AddTarget ZCode 添加模型的可挂靠供应商(非敏感字段,Snapshot 携带)。
type AddTarget struct {
    ProviderID   string `json:"provider_id"`
    ProviderName string `json:"provider_name,omitempty"`
}

// ModelSource 添加模型的来源接口信息;handler 从 DB 装配,凭据只进不出。
type ModelSource struct {
    ProviderName string // vendor/显示名缺省值
    BaseURL      string // ZCode 新建供应商的 api.baseUrl
    EndpointURL  string // WorkBuddy/CodeBuddy 新条目的 url(完整 endpoint,handler 派生)
    APIKey       string // 仅"新增写入凭据"路径落盘;ZCode 挂已有供应商不使用
}

// AddTargetSpec ZCode 落点规格;WorkBuddy/CodeBuddy 整体忽略。
type AddTargetSpec struct {
    Mode         string // "existing" | "new"(空按 existing 处理)
    ProviderID   string // Mode=existing 必填
    ProviderName string // Mode=new 显示名,空回退 Source.ProviderName
    APIType      string // Mode=new 取 "openai-chat-completions"|"openai-responses",空缺省前者
}

// AddModelsRequest 一次添加请求;ModelIDs 为显式清单("全部"由前端全选产生)。
type AddModelsRequest struct {
    ModelIDs []string
    Source   ModelSource
    Target   AddTargetSpec
}

// AddModelsResult 添加结果:最新清单 + 逐项成败,前端据此反馈"成功 N/跳过 M"。
type AddModelsResult struct {
    Entries []ModelEntry `json:"entries"`
    Added   []string     `json:"added"`
    Skipped []string     `json:"skipped"` // 已存在而跳过
}
```

`Agent` 接口新增两个方法(三个实现都必须支持,故进主接口而非可选能力接口):

```go
// RemoveModels 按定位批量删除:整体校验(不存在报 4404)→ 一次备份 → 树编辑 → 原子写回;
// 空 refs 只读返回现状(不备份不落盘),口径同 ApplyModels 空提交。
RemoveModels(ctx context.Context, refs []ModelRef, dataDir string) ([]ModelEntry, error)
// AddModels 逐条添加,已存在跳过;全部跳过时不备份不落盘。
AddModels(ctx context.Context, req AddModelsRequest, dataDir string) (AddModelsResult, error)
```

`Snapshot` 新增能力位与 ZCode 挂靠候选:

```go
SupportsAddModels    bool        `json:"supports_add_models"`    // 工具属性,not_found 时同样置位
SupportsRemoveModels bool        `json:"supports_remove_models"` // 同上
AddTargets           []AddTarget `json:"add_targets,omitempty"`  // 仅 ZCode 且 found 时填充
```

能力位与适配器实现的一致性由 agents 包既有测试模式兜底(参照 `SupportsDefaultModel`)。

### 2. 适配器实现(`internal/service/agentconf/agents/`)

#### zcode.go

- **RemoveModels**:`readTree` → 整体校验每个 ref 的 (providerId, modelId) 存在于该供应商 `modelOrder ∪ personalModelIds`(否则 4404)→ `Backup` → 逐 ref 树编辑:
  - 从对应供应商 `config.modelOrder` 与 `config.personalModelIds` 移除该 id(数组节点缺失则跳过对应步骤);
  - 从 `config.modelConfigRules.providerModelRules` 移除 (providerId, modelId) 命中的规则节点;
  - `manualProviderModelRules` 不动;**供应商节点一律不删**(清单变空也保留)。
  → `WriteJSONFile` 原子写回 → 返回最新清单。日志只记删除数量与 (providerId, modelId)。
- **AddModels**:`readTree` → 校验:`ModelIDs` 非空(4403);`Target.Mode` ∈ {"", "existing", "new"}(否则 4403)。
  - **Mode=existing**:`Target.ProviderID` 必须在 `providerOrder`/`providerRules` 中存在(否则 4404)→ 逐 id 判重(已在该供应商 `modelOrder ∪ personalModelIds` → skipped)→ 全部 skipped 零落盘 → `Backup` → 新 id 按输入顺序追加到该供应商 `config.personalModelIds`(缺失时创建)→ 原子写回。`Source` 整体不使用。
  - **Mode=new**:providerId 自动生成——按 ZCode 自身惯例取 `new-provider`,被占则 `new-provider-2`、`new-provider-3`…(扫描既有 providerRules 取最小未用序号);`Target.APIType` ∈ {"", "openai-chat-completions", "openai-responses"}(否则 4403,空缺省 chat-completions);→ 逐 id 全量判重(所有供应商的清单,同 id 已存在 → skipped)→ 全部 skipped 零落盘 → `Backup` → 在 `providerConfigRules.providerRules` 追加节点并把它追加进 `config.providerOrder`:
    ```json
    {
      "providerId": "new-provider-N",
      "providerName": "<Target.ProviderName 或 Source.ProviderName>",
      "enabled": true,
      "config": {
        "group": "standard-personal",
        "access": {"type": "api-key", "apiKey": "<Source.APIKey>"},
        "api": {"type": "<Target.APIType>", "baseUrl": "<Source.BaseURL>"},
        "modelOrder": [],
        "personalModelIds": ["<新增id按输入顺序>"]
      }
    }
    ```
    **只写以上键,绝不写 templateId 或其他键**——根对象与 config 为 zod strict,未知键会让 ZCode 拒载整份配置;规则节点不预置。
- **Snapshot**:found 分支补 `collectAddTargets`(从 `collectZCodeProviders` 取 id+name),填能力位与 `AddTargets`。

#### workbuddy.go / codebuddy.go(逻辑同构,分别实现)

- **RemoveModels**:`readDoc` → 取模型数组 → 整体校验每个 model_id 按"首个匹配"口径存在(否则 4404)→ `Backup` → 过滤数组移除命中元素(非对象元素原样保留)→ 对象形态:回填 `models`,顶层 `availableModels` 为数组时移除其中命中的 id → 原子写回。
- **AddModels**:校验 `ModelIDs` 非空(4403)→ 逐 id 判重(数组内已有同 id 条目 → skipped)→ 全部 skipped 零落盘 → `Backup` → 按 id 顺序追加新条目:
  ```json
  {"id": "<modelId>", "name": "<modelId>", "vendor": "<Source.ProviderName>", "url": "<Source.EndpointURL>", "apiKey": "<Source.APIKey>"}
  ```
  只写这 5 键(其余字段交给工具缺省语义与用户后续编辑;`url` 是完整 endpoint 语义,由 handler 派生)→ 对象形态:回填 `models`,顶层 `availableModels` 为数组且不含该 id 时追加(缺失不创建)→ 原子写回。
- WorkBuddy 裸数组形态无 `availableModels` 概念,自然跳过;CodeBuddy `readDoc` 已保证仅对象形态。
- 日志只记 added/skipped 数量,绝不记 url/key 值。

### 3. Handler(`internal/handler/agent.go` + `router.go`)

```go
// POST /api/agents/:name/models/remove  body {"targets":[{"provider_id","model_id"}]}
AgentModelsRemove(dataDir string) gin.HandlerFunc
// POST /api/agents/:name/models/add
// body {"provider_id":1,"model_ids":["..."],"base_url":"...", "target":{"mode":"existing|new","provider_id":"...","provider_name":"...","api_type":"..."}}
AgentModelsAdd(db *gorm.DB, dataDir string) gin.HandlerFunc
```

- **AgentModelsAdd** 装配流程:`agentByName` → 绑定请求体(`model_ids` 空 → 4403)→ `db.First(&model.Provider, providerID)`(不存在 → 1404)→ 组装 `ModelSource`:
  - `BaseURL` = 请求 `base_url`(trim 后非空)否则 `p.BaseURL`;
  - `EndpointURL` = `llmclient.BuildURL(BaseURL, "/v1/chat/completions")`(复用既有归一:根地址/尾斜杠/带 `/v1` 三种形态都只拼出一段 `/v1`;ZCode 挂已有供应商不消费该值);
  - `APIKey` = `p.APIKey`;`ProviderName` = `p.Name`。
  → `a.AddModels(...)` → 返回 `AddModelsResult`。`target` 体缺省为零值(Mode 空 → 按 existing、ProviderID 空 → 由适配器报 4404)。
- **AgentModelsRemove**:绑定 `{"targets": []}`(空数组交由实现只读返回)→ `a.RemoveModels(...)` → 返回最新清单。
- 路由注册在 `router.go` 的 `/agents` 组内,与现有路由同风格;`AgentModelsAdd` 是该组唯一依赖 DB 的路由,装配时由 main 注入。

### 4. 错误码(全部复用,不新增)

| 场景 | 码 |
| --- | --- |
| 定位的模型/目标供应商不存在 | `CodeAgentNotFound`(4404) |
| `model_ids` 为空、`target.mode`/`api_type` 非法、请求体非法 | `CodeAgentInvalid`(4403) |
| 配置文件缺失/非法 JSON/schemaVersion 不支持 | `CodeAgentFileIO`(4402) |
| 接口记录不存在 | `CodeProviderNotFound`(1404) |
| 能力位不匹配(未来兜底) | `CodeAgentUnsupported`(4405) |

### 5. 安全与日志要点

- 凭据流向唯一:`providers` 表 → `ModelSource` → 适配器落盘;不出现在任何响应、日志、错误信息中。
- 删除/添加的日志只记数量与定位键(providerId/modelId)与落点模式,与现有 `slog.Info` 口径一致。
- 备份文件即配置原文(WorkBuddy/CodeBuddy 本就明文存 key),无新增暴露面;备份/还原机制(`backup.go`)零改动。
- ZCode 新建供应商写入的 apiKey 属用户显式确认的例外(PRD 红线 #2);挂已有供应商路径不触碰 `Source.APIKey`。

## 前端

### 1. 类型与 API(`web/src/types/agent.ts` + `web/src/api/agent.ts`)

- 类型:`AgentModelRef`、`AgentAddTarget`、`AgentAddTargetSpec`、`AgentAddModelsResult`、`AgentAddModelsPayload`;`AgentSnapshot` 增 `supports_add_models`、`supports_remove_models`、`add_targets?`。
- API:`removeAgentModels(name, targets)` → `POST .../models/remove`;`addAgentModels(name, payload)` → `POST .../models/add`。均沿用 `useRequest` 失败统一提示。

### 2. 字段输入组件抽取(`web/src/components/agent/AgentFieldInput.vue`,新增)

- 把 `AgentModelCard` 表格单元格的按列类型渲染分支(bool→switch / number→input-number / list→多选 / select→下拉 / text→输入)抽成受控组件:`props: { col: AgentFieldSpec; modelValue: unknown }`,`emit: update:modelValue`。
- 表格单元格与编辑弹窗共用,消除渲染分支重复(组件规范复用要求);取值/写回辅助函数(`boolOf/numOf/strOf/listOf`)随迁。

### 3. `AgentView.vue` tab 化

- `el-card` 内改 `el-tabs`:`v-for` 生成 `el-tab-pane`(`label`=display_name,`name`=name,`lazy` 惰性挂载),pane 内放 `AgentModelCard`;默认激活第一个 tab,保留"刷新"按钮。未找到配置的 Agent 照常有 tab(卡片内部已有指引分支)。

### 4. `AgentModelCard.vue` 增强

- 表格首列加 `type="selection"` 多选;工具栏新增"从接口添加模型"(能力位+found 控制)与"批量删除"(选中>0 可用,二次确认)。
- 新增固定右侧"操作"列:行内"编辑""删除"按钮;删除单行复用批量删除通道(单元素 targets)。
- 编辑弹窗(`el-dialog`):按 `editableColumns` 用 `AgentFieldInput` 渲染表单,确定时生成单行 patch 走现有 `updateAgentModels`,成功后以响应重置草稿。
- 与行内草稿的关系:弹窗确定即提交该行 patch 并整体刷新,避免两套脏状态叠加(不改动现有草稿比对逻辑)。

### 5. 添加弹窗(`web/src/components/agent/AgentImportModelsDialog.vue`,新增)

- 步骤:选择接口(下拉,`GET /api/providers`,展示名称与 base_url)→ 拉取模型列表(复用现有模型列表 API)→ 表格多选 + "全选" → 确认。
- 接口地址输入框:预填所选接口 base_url,可改;WorkBuddy/CodeBuddy 下方差框提示将派生的完整 endpoint(`base 去尾斜杠 + /v1/chat/completions`,`/v1` 不重复);ZCode 忽略 endpoint 概念。
- ZCode 落点选择:下拉含 `snapshot.add_targets` 各供应商 + 特殊项"新建供应商…";选新建时展开"显示名"(预填接口名)与"API 协议"下拉(`openai-chat-completions` 默认 / `openai-responses`)。
- 确认二次弹窗:WorkBuddy/CodeBuddy 提示"将把所选接口的 API Key 写入 <Agent> 配置文件,请确认";ZCode 新建供应商同样提示写入 API Key;ZCode 挂已有供应商为常规写回确认。
- 成功后按 `added/skipped` 反馈"成功添加 N 个,跳过 M 个(已存在)",emit 刷新清单。

## 兼容性

- 新端点/新字段均为增量;`PUT /models`、默认模型、还原等既有通道零改动。
- `Snapshot` 新字段对旧前端无感(可选字段);旧后端对新前端无感(能力位缺省 false 时前端隐藏入口,不误伤)。

## 测试设计

- **适配器单测**(沿用既有 `*_test.go` 风格,夹具为临时目录 + 虚构配置):
  - zcode 删除:双清单移除、规则节点清理、manual 不动、供应商节点保留、4404 整批拒绝、空 refs 零落盘、未知键与 apiKey 零丢失、UseNumber 精度。
  - zcode 添加-挂靠:personalModelIds 追加与保序、目标供应商 4404、已存在 skip、全 skip 零落盘。
  - zcode 添加-新建:providerId 生成与最小未用序号(`new-provider`、`new-provider-2`…)、节点形状逐键断言(恰为设计所列键集,无 templateId)、api.type 非法 4403、providerOrder 追加、全 skip 零落盘。
  - workbuddy:裸数组与对象两形态的增删、availableModels 追加/移除、新条目 5 键正确(url=完整 endpoint)、既有 apiKey 零接触、4404/4403。
  - codebuddy:对象形态增删、availableModels 同步、顶层非对象 4402 维持、availableModels 缺失时不创建。
  - agents 包一致性测试:能力位与接口实现一致(参照 SupportsDefaultModel 模式)。
- **handler 测试**:remove/add 路由成功与失败分支(provider 不存在 1404、model_ids 空 4403、未知 agent 4401、非法 target 4403)。
- **前端**:`npm run lint` + `npm run build`(vue-tsc)通过;不引入新依赖。
