# Agent 模型配置管理 — 技术设计

共享决策见父任务 design.md;本文写本任务落点。**实现前勘探已完成(2026-09-29,Windows 本机)**,结论如下,路径与格式以此为准。

## 勘探结论

- **ZCode 模型选择**:`~/.zcode/cli-bin/model-selection.json`,结构 `{"providerId","modelId","options":{...}}`——这是「当前用什么模型」的权威文件,体积小,写回风险可控。
- **ZCode 供应商定义**:`~/.zcode/cli-bin/provider_config.cli.json`,结构 `{"schemaVersion":1,"config":{"providerOrder":[...],"providerConfigRules":{"providerRules":[{"providerId","providerName","config":{"api":{"baseUrl"...},"access":{"apiKey":<敏感>}...}}]}}}`;`~/.zcode/v2/provider_config.json` 为同构副本(桌面端)。**含明文 API Key,严禁整体回传或写回。**
- **WorkBuddy**:`%APPDATA%\WorkBuddy`、`%LOCALAPPDATA%\WorkBuddy` 均为空,`~/WorkBuddy` 下只有会话工作目录,未找到模型配置文件 → 按 prd 预案以「未找到」占位状态交付,不阻塞。

## 范围(安全优先的 MVP)

- ZCode **读**:当前模型选择 + 可选 provider 清单(仅 providerId/providerName,不含 apiKey)。
- ZCode **写**:仅 `model-selection.json` 的 providerId/modelId;`options` 及其他未知键原样保留;**不写 provider_config**(避免误改凭据),界面注明「供应商与密钥请在其原生工具中管理」。
- WorkBuddy:探测候选路径(见 workbuddy.go),全部不存在 → Status=not_found + 中文指引;Apply 报 4404。

## 后端

### internal/service/agentconf(新包)

```go
// FieldSpec 字段描述,驱动前端动态表单。
type FieldSpec struct {
    Key      string        `json:"key"`
    Label    string        `json:"label"`
    Type     string        `json:"type"` // "text" | "select"
    Options  []FieldOption `json:"options,omitempty"` // select 用
    Required bool          `json:"required"`
    Help     string        `json:"help,omitempty"`
}
type FieldOption struct{ Value, Label string }

// Snapshot Agent 配置现状视图(不回传任何凭据)。
type Snapshot struct {
    Name        string            `json:"name"`
    DisplayName string            `json:"display_name"`
    Status      string            `json:"status"` // "found" | "not_found"
    ConfigPath  string            `json:"config_path"`
    Fields      []FieldSpec       `json:"fields"`
    Values      map[string]string `json:"values"`
    Message     string            `json:"message,omitempty"` // not_found 时给指引
}

// Agent 扩展点:新工具实现并 Register 即可接入。
type Agent interface {
    Name() string
    DisplayName() string
    Snapshot(ctx context.Context) Snapshot
    Apply(ctx context.Context, values map[string]string) (Snapshot, error)
}
```

- `registry.go`:`Register(Agent)` + `Get(name)` + `List()`(名称排序);包内 `init()` 注册 ZCode 与 WorkBuddy 实例。
- `zcode.go`:
  - 路径基于 `homeDir`(构造注入,包级默认 `os.UserHomeDir()`,便于测试)。
  - Snapshot:model-selection.json + provider_config.cli.json 都存在 → found;Fields = providerId(select,选项来自 providerOrder+providerRules 解析,显示 providerName)、modelId(text);Values 取当前值。
  - Apply:校验 providerId 在选项内、modelId 非空(4402 语义的校验失败走 4401?不——参数校验失败用 `CodeAgentInvalid=4403` 新增)→ 备份 → 读原文件(JSON 解析到 map 或结构体+RawMessage)→ 只改 providerId/modelId → 写回(缩进 2 空格保持可读)。
  - 文件缺失 → 4404;JSON 解析失败 → 4402「配置文件不是合法 JSON」。
- `workbuddy.go`:候选路径依次探测(`%APPDATA%\WorkBuddy\config.json`、`%APPDATA%\WorkBuddy\settings.json` 等);均不存在 → Status=not_found + Message「未在本机找到 WorkBuddy 配置文件,请确认其已安装并至少运行过一次;找到后可扩展此实现接入」;Apply → 4404。
- `backup.go`:Backup(path, agentName, dataDir) → `data/agent-backups/<name>/<yyyymmdd-HHMMSS>.bak`(复制原文件);RestoreLatest(agentName, dataDir) → 最近一份写回;KeepLatestN=10 清理。备份/还原目录基于 cfg.DataDir(handler 注入)。

### 错误码(codes.go 新增 4xxx)

`CodeAgentUnknown=4401`(不支持的名称)、`CodeAgentFileIO=4402`(读写/备份失败)、`CodeAgentNotFound=4404`(配置文件不存在)、`CodeAgentInvalid=4403`(提交值非法,如 providerId 不在选项内)。

### handler/agent.go + router

- `GET /api/agents` → []Snapshot(含 not_found 项)。
- `GET /api/agents/:name` → Snapshot;未知 name → 4401。
- `PUT /api/agents/:name`,body `{values: {...}}` → Apply(内部备份)→ 新 Snapshot。
- `POST /api/agents/:name/restore` → 还原最近备份 → Snapshot;无备份 → 4404「暂无可还原的备份」。
- dataDir 从 config 注入(NewRouter 增加 dataDir 参数或经 db 旁路——按 main.go 现有装配方式最小改动,在 router 装配时传入)。

### 测试

- t.TempDir 伪造 home:zcode Snapshot(含 provider 选项解析)、Apply 写回(providerId/modelId 改、options/未知键保留、缩进)、文件缺失 4404、坏 JSON 4402、非法值 4403;
- 备份/还原/保留 10 份清理;
- workbuddy not_found;handler 层 4401/4404 语义。

## 前端

- `types/agent.ts`(Snapshot/FieldSpec)+ `api/agent.ts`。
- `views/agent/AgentView.vue`:
  - 卡片列表:名称、状态 tag(已找到/未找到)、配置路径、当前值摘要(Values 按 Label 拼接);未找到时展示 Message 且按钮禁用。
  - 「修改配置」对话框:按 Fields 动态渲染(el-select / el-input,Required 标记,Help 说明);保存二次提示「修改前会自动备份原配置」。
  - 「还原上次备份」按钮(二次确认);成功后刷新卡片。
  - ZCode 卡片 Help 文案注明:供应商与 API Key 请在 ZCode 原生工具中管理,本页仅切换当前模型。
- 路由 `/agents` + 菜单「Agent 配置」。

## 兼容与回滚

- 全新增文件 + router 一处挂载 + main.go 传 dataDir 小改;不新增数据表;回滚按文件粒度。
