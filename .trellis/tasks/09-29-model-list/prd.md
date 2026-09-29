# 模型列表

## Goal

P0-2:基于已保存的接口配置,通过后端代理拉取 OpenAI 兼容 `/v1/models` 并展示;失败时区分网络错误与鉴权错误,给出中文提示。

## Requirements

### 后端

1. `GET /api/providers/:id/models`:用该配置的 Base URL + API Key 调用 `GET {base}/v1/models`,解析 `data[]`(模型 id、owned_by、created 等)透传前端;不落库。
2. 错误归类与中文提示(父任务 design.md 错误码 1500/1501/1502):
   - 网络错误(连接失败 / 超时 / DNS):「无法连接上游服务」并附根因摘要。
   - 鉴权错误(HTTP 401/403):「鉴权失败,请检查 API Key」。
   - 其他非 2xx:附上游状态码;响应结构不符合预期时报解析失败。
   - 配置 id 不存在:中文 404。
3. URL 拼接遵循父任务 design.md 的 BaseURL 归一化规则(尾部 `/`、`/v1` 双习惯)。
4. API Key 不进日志;上游请求由后端代理,浏览器不直连。

### 前端

5. 新页面「模型列表」(`/models`),加入菜单:
   - 顶部下拉选择接口配置 + 「拉取模型」/「刷新」按钮。
   - 表格:模型 ID、owned_by、created 等;支持本地关键字过滤。
   - 加载中 loading;错误用 el-alert / ElMessage 展示后端中文提示。
6. providers 列表跨页共享(本任务与后续测试页都要用):按 state-management 规范落地 Pinia store。
7. 尚无接口配置时给出引导(链接到 /providers)。

## Acceptance Criteria

- [ ] 选择配置可拉取并展示模型列表,手动刷新可用,过滤可用
- [ ] 网络错误与鉴权错误给出不同的中文提示
- [ ] 无配置、配置不存在时中文引导/报错
- [ ] 日志无 API Key 明文
- [ ] `go vet ./...`、`go test ./...` 通过;`cd web && npm run lint && npm run build` 通过

## Notes

- 上游响应结构以 OpenAI 标准 `{object:"list", data:[...]}` 为主;字段缺失时容忍降级(仅展示 id)。
- service 单测用 httptest 假上游覆盖:成功、401、连接失败、结构异常四类。
