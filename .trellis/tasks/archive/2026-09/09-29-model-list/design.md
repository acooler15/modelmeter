# 模型列表 — 技术设计

共享决策见父任务 design.md(路由表、错误码 1xxx 段、llmclient 约定);本文写本任务落点。

## 后端

### internal/service/llmclient(新建包,后续测试/费率任务复用)

`client.go`:

```go
// UpstreamConfig 上游访问配置,来源于已保存的接口配置。
type UpstreamConfig struct{ BaseURL, APIKey string }

// NormalizeBaseURL 去除尾部 '/'
// BuildURL(base, path) string:path 以 "/v1/" 开头;base 已以 "/v1" 结尾时去掉 path 的 "/v1" 前缀
// GetJSON(ctx, cfg, path, out any) error:Bearer 鉴权,http.Client{Timeout: 30s},
//   错误归类:网络类(net.Error/连接拒绝/超时/TLS)→ 1500;
//   HTTP 401/403 → 1501;其余非 2xx → 1502(消息含状态码);JSON 解析失败 → 1502
```

- 错误码(codes.go 新登记):`CodeUpstreamNetwork=1500`「无法连接上游服务」、`CodeUpstreamAuth=1501`「鉴权失败,请检查 API Key」、`CodeUpstreamBadResponse=1502`「上游服务返回异常」。
- 网络错误消息附根因摘要(`err` 链保留,文案用中文 + 简短原因)。
- 本任务只用 GetJSON;POST 能力留到模型测试任务扩展,不提前抽象。

### internal/service/modelcatalog.go

- `ModelInfo{ID, Object, OwnedBy string; Created int64; Raw json.RawMessage}`(Raw 透传上游原始条目,前端可展示完整元信息)。
- `ListModels(ctx, db, providerID uint) ([]ModelInfo, error)`:
  1. 查 Provider,`ErrRecordNotFound` → 1404「接口配置不存在」;
  2. `GetJSON(ctx, cfg, "/v1/models", &resp)`,resp 按 `{data: [...]}` 解析;字段缺失容忍降级(仅 ID 必有,空 ID 条目跳过);
  3. data 非数组/整体结构异常 → 1502「上游返回的模型列表格式异常」。

### handler / router

- `internal/handler/provider.go` 增加 `ProviderModels(db)`;router 在 providers 组挂 `p.GET("/:id/models", ProviderModels(db))`。
- id 解析沿用现有 1401 语义。

### 测试(httptest)

`llmclient/client_test.go`:BuildURL 三形态(根地址、带尾斜杠、带 /v1);GetJSON 归类 200/401/500/拒连/坏 JSON。
`modelcatalog_test.go`:成功解析(含字段缺失降级)、404 配置不存在、1501/1500 透传。

## 前端

- **Pinia store(本任务落地)**:`web/src/stores/providers.ts`,setup store:`list、loading、loaded、load(force?)`;ProviderList.vue 与 ModelListView.vue 共用(把 ProviderList.vue 的数据源迁到 store,删除本地重复请求逻辑——改动小、收益是单一数据源)。
- `web/src/types/model.ts`:`ModelInfo`;`web/src/api/modelCatalog.ts`:`fetchModels(providerId)`。
- `web/src/views/model/ModelListView.vue`:
  - 顶部:el-select 选配置(store 数据)+ 「拉取模型」/「刷新」按钮 + 关键字过滤输入。
  - 表格:模型 ID、owned_by、created;点击行可展开查看 Raw 元信息(JSON)。
  - 未选配置禁用按钮;store 为空时 el-empty + 「去接口配置页」引导链接。
  - 错误经 useRequest/ApiError 展示后端中文提示(网络/鉴权文案直接来自后端)。
- 路由 `/models` + MainLayout 菜单「模型列表」。

## 兼容与回滚

- 新增文件为主;既有 ProviderList.vue 改数据源属小改;回滚按文件粒度还原。
