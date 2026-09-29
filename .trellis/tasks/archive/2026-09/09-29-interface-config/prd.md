# 接口配置管理

## Goal

P0-1:提供接口配置(名称、Base URL、API Key)的增删改查与 SQLite 持久化,是模型列表、模型测试等后续功能的基础层。

## Requirements

### 后端

1. 五个端点,统一 JSON 信封(路由已在父任务 design.md 登记):
   - `POST /api/providers` 新增:名称、Base URL、API Key 均必填。
   - `GET /api/providers` 列表:按创建时间正序返回。
   - `PUT /api/providers/:id` 编辑:**api_key 为空表示沿用原值**。
   - `DELETE /api/providers/:id` 删除。
   - 响应中的 Key 一律脱敏,任何接口不返回明文。
2. 持久化:providers 表,GORM AutoMigrate(在 main.go mustOpenDB 登记)。
3. 校验(全部中文错误提示,信封 code≠0):
   - 名称非空且全局唯一(重复时报「名称已存在」)。
   - Base URL 必须是合法的 http/https URL。
   - API Key 非空(编辑留空除外)。
   - id 非法或不存在时返回 404 语义的中文错误。
4. 脱敏规则(父任务 design.md):`前3位 + **** + 后4位`,长度不足 8 位时全部 `*`。
5. 日志不出现 API Key 明文。

### 前端

6. 新页面「接口配置」(`/providers`),加入 MainLayout 菜单:
   - 表格列:名称、Base URL、API Key(脱敏)、更新时间、操作(编辑/删除)。
   - 「新增配置」按钮打开对话框表单;编辑对话框中 Key 输入框留空表示不修改(占位文案说明)。
   - 删除需二次确认(ElMessageBox)。
   - 操作成功/失败给出中文反馈(ElMessage)。
7. 空状态引导:尚无配置时提示先新增。

## Acceptance Criteria

- [ ] 增删改查全流程走通;重启服务后数据仍在
- [ ] 名称重复、URL 非法、必填缺失均返回明确中文错误
- [ ] 任何接口响应与日志中无 API Key 明文;界面显示脱敏格式
- [ ] 编辑时 Key 留空则原值保留
- [ ] `go vet ./...`、`go test ./...` 通过;`cd web && npm run lint && npm run build` 通过

## Notes

- 列表页本地状态即可,Pinia store 提升留给 model-list 子任务(跨页共享时再提)。
- 编辑用 PUT 语义;前端 http.ts 如缺 httpPut/httpDelete 需补齐。
