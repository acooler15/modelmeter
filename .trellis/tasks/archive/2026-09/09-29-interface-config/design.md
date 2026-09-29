# 接口配置管理 — 技术设计

共享决策(路由、错误码段、脱敏规则、数据模型)见父任务 design.md,本文只写本任务落点。

## 后端

### model 层

`internal/model/provider.go`:

```go
// Provider 接口配置:用于访问某个 LLM 服务站点。
type Provider struct {
    ID        uint   `gorm:"primaryKey" json:"id"`
    Name      string `gorm:"uniqueIndex;size:100" json:"name"`
    BaseURL   string `gorm:"size:500" json:"base_url"`
    APIKey    string `gorm:"size:500" json:"-"` // 永不序列化输出
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

- `APIKey` 的 json tag 必须为 `-`,从源头防止明文外泄。
- main.go `mustOpenDB` 中 AutoMigrate 登记 `&model.Provider{}`。

### service 层

`internal/service/provider.go`:

- 输入 DTO:`ProviderInput{Name, BaseURL, APIKey string}`(json tag snake_case)。
- 输出 DTO:`ProviderView{ID, Name, BaseURL, APIKeyMasked, CreatedAt, UpdatedAt}`;`APIKeyMasked` 由 `MaskKey` 生成:`len<=8` 全 `*`,否则前 3 + `****` + 后 4。
- 函数:`ListProviders(ctx, db) ([]ProviderView, error)`、`CreateProvider(ctx, db, in)`、`UpdateProvider(ctx, db, id, in)`、`DeleteProvider(ctx, db, id)`。
- 校验:名称非空、BaseURL 可被 `net/url.ParseRequestURI` 解析且 scheme 为 http/https、新增/改名时唯一性查库(`Name` 命中即报 1402)。更新时 `APIKey == ""` 保留原值。
- 错误(codes.go 登记):`CodeProviderInvalid=1401`、`CodeProviderDuplicate=1402`、`CodeProviderNotFound=1404`;消息中文,如「名称已存在」「Base URL 不是合法的 http(s) 地址」。
- `gorm.ErrRecordNotFound` 统一转 1404。

### handler 层

`internal/handler/provider.go`:五个端点函数,只做 ShouldBindJSON / 解析 id(`strconv.ParseUint`,非法报 1401)/ 调 service / `response.OK|Fail`。router.go 挂载:

```go
p := api.Group("/providers")
p.GET("", ProviderList(db))
p.POST("", ProviderCreate(db))
p.PUT("/:id", ProviderUpdate(db))
p.DELETE("/:id", ProviderDelete(db))
```

### 测试

`internal/service/provider_test.go`(内存 sqlite `gorm.Open(sqlite.Open(":memory:"))`):覆盖 校验失败三类、唯一性冲突、脱敏格式(普通/短 key)、编辑空 key 保留原值、删除不存在。

## 前端

- `web/src/types/provider.ts`:`ProviderView`、`ProviderInput`(与后端 json tag 对齐)。
- `web/src/api/provider.ts`:`listProviders / createProvider / updateProvider / deleteProvider`;`http.ts` 如缺 `httpPut / httpDelete` 先补齐(风格与 httpGet/httpPost 一致)。
- `web/src/views/provider/ProviderList.vue`:el-table + el-dialog 表单 + 删除二次确认;空状态 el-empty 引导;反馈用 ElMessage(成功)与 useRequest 错误提示。
- `web/src/router/index.ts` 增加 `/providers` 路由;`MainLayout.vue` 菜单加「接口配置」。
- 页面文案全中文;表单校验用 el-form rules(必填、URL 格式),服务端错误以 ApiError 消息展示。

## 兼容与回滚

- 纯新增文件 + main.go/router.go 两处挂载点,无迁移风险;回滚即删除新增文件并还原两处挂载。
