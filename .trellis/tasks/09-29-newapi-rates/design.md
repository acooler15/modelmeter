# New API 模型费率 — 技术设计

共享决策见父任务 design.md;本文写本任务落点。已确认 New API(one-api 系)`GET {base}/api/pricing` 返回结构:`{"data":[{"model_name","quota_type","model_ratio","completion_ratio","model_price",...}]}`(quota_type:0=倍率计费,1=按次计费;不同版本可能多 `cache_ratio`/分组字段,解析器必须容忍未知字段与缺失字段)。

## 后端

### internal/model/setting.go

```go
// Setting 通用键值配置,值存 JSON 文本;首用于 New API 配置。
type Setting struct {
    Key   string `gorm:"primaryKey;size:100" json:"key"`
    Value string `gorm:"type:text" json:"-"`
}
```

- `TableName()="settings"`;main.go AutoMigrate 登记。Value 不序列化输出(内容可能含令牌)。

### internal/service/llmclient 小扩展(错误码参数化,不动既有行为)

- 新增 `GetJSONWithCodes(ctx, cfg, path, out, codes ErrCodes)`,`ErrCodes{Network, Auth, BadResponse int}`;现有 `GetJSON` 改为其传 1xxx 默认值的薄封装(签名不变,既有调用与测试不受影响)。错误文案模板共用,消息里的服务名词由调用方语义决定时用通用「上游服务」+ 具体码。

### internal/service/newapi.go

- `NewAPIConfigInput{BaseURL, Token string}`;存储键 `newapi`,值为 JSON `{"base_url","token"}`。
- `GetNewAPIConfig(db) (NewAPIConfigView, error)`:视图含 `token_masked`(复用 `MaskKey`),不含明文。
- `SaveNewAPIConfig(db, in)`:BaseURL 必须合法 http(s);**Token 留空表示沿用原值**(与 Provider 编辑语义一致);首次保存 Token 必填 → 3401。
- `ListRates(ctx, db) ([]RateEntry, error)`:
  1. 读配置,缺失/不完整 → `CodeNewAPINotConfigured=3401`「请先配置 New API 地址与令牌」;
  2. `GetJSONWithCodes` 调 `{base}/api/pricing`(带 Bearer,公开接口多余头无害);错误码 `CodeNewAPINetwork=3500` / `CodeNewAPIAuth=3501` / `CodeNewAPIBadResponse=3502`,文案明确「New API 服务」;
  3. `RateEntry{ModelName, QuotaType int, ModelRatio, CompletionRatio, ModelPrice float64}`;`data` 缺失/非数组 → 3502;条目容忍缺字段(零值),`model_name` 空的跳过;按 model_name 排序。
- `EstimateCost(ctx, db, modelName string, promptTokens, completionTokens int) (CostEstimate, bool)`:
  - 命中返回 `(estimate, true)`;未配置/请求失败/未命中/用量为 0 一律 `(CostEstimate{}, false)` 并只记日志(不外抛,满足「不显示、不报错」)。
  - 计算(one-api 口径):
    - 倍率型(quota_type=0):`quota = model_ratio×prompt + model_ratio×completion_ratio×completion`;`USD = quota / 500000`
    - 按次型(quota_type=1):`USD = model_price`(model_price 为美元/次)
  - `CostEstimate{Model, QuotaType int, USD float64, Quota float64, Formula string}`;Formula 为中文口径说明(前端直接展示)。

### handler/newapi.go + router

- `GET /api/newapi/config`(脱敏视图)、`PUT /api/newapi/config`、`GET /api/newapi/rates`、`POST /api/newapi/estimate`(body `{model, prompt_tokens, completion_tokens}`,信封 data 为 `{available:bool, estimate?}`;available=false 也是 code=0)。

### 测试

- `service/newapi_test.go`:配置保存/脱敏/留空沿用/校验;ListRates httptest(成功解析含缺字段降级、3401、3501、3502);EstimateCost(倍率型、按次型、未命中、未配置、零用量)。

## 前端

- `types/newapi.ts` + `api/newapi.ts`(四个封装)。
- `views/rate/RateView.vue`:
  - 配置卡片:地址 + 令牌(password 输入、占位「留空则沿用原令牌」)+ 保存;已配置时展示地址与脱敏令牌。
  - 费率表格:模型、计费类型(倍率/按次)、输入倍率、完成倍率、按次价格;关键字过滤 + 「刷新」;未配置时 el-empty 引导去配置卡片。
- `views/test/ModelTestView.vue` 集成:测试成功且有用量后调 estimate;结果区成本行显示 `约 $x.xxxxxx`(含 Formula 口径);available=false 时该行维持占位不显示。
- 路由 `/rates` + 菜单「模型费率」。

## 兼容与回滚

- GetJSON 重构为薄封装属内部等价变换,既有测试守护;新增文件为主,回滚按文件粒度。
