# 模型测试

## Goal

P0-3:对选中模型发起对话式实测,支持三种协议、常用参数与流式开关,展示回复与性能 / token 指标,保留最近测试记录便于横向对比。

## Requirements

### 功能

1. 三种协议任选(协议差异由后端处理,前端单选):
   - OpenAI Chat Completions(`POST /v1/chat/completions`)
   - OpenAI Responses(`POST /v1/responses`)
   - Anthropic Messages(`POST /v1/messages`)
2. 可自定义参数:system 提示词、user 消息、temperature、max_tokens、流式开关。
3. 结果展示:回复全文(Markdown 纯文本即可)、**首字延迟(ms)**、**总耗时(ms)**、token 用量(prompt / completion / total;上游未返回时显示「未返回」)。
4. 流式:后端代理上游 SSE 并转发,前端逐字渲染;结束后同样展示计时与用量。
5. 测试记录:每次测试(含失败)落库,保留最近 200 条;测试页下方记录列表(时间、配置、模型、协议、状态、耗时、tokens),点击可查看完整回复;支持一键清空。
6. 测试时选择「接口配置 + 模型」:模型 ID 可手输,也可从所选配置拉取模型列表后下拉选择(复用 /api/providers/:id/models)。

### 非功能

7. API Key 不进日志、不回前端;请求全部后端代理。
8. 参数缺失、上游 401、上游网络失败、响应解析失败分别给出不同中文提示(错误码 2xxx)。
9. 全中文界面。

## Acceptance Criteria

- [ ] 三种协议均能完成一次非流式测试,正确展示回复、首字延迟、总耗时与用量
- [ ] 流式测试逐字渲染,结束后展示指标并落记录
- [ ] 失败场景(参数缺失 / 401 / 网络失败 / 解析失败)中文提示明确且互可区分
- [ ] 记录保留最近 200 条,重启后仍在;可查看历史完整回复;可清空
- [ ] temperature / max_tokens 留空时不随请求发送(使用上游默认)
- [ ] 日志与界面无 API Key 明文
- [ ] `go vet ./...`、`go test ./...` 通过;`cd web && npm run lint && npm run build` 通过

## Notes

- 三协议报文差异与流式事件类型见父任务 design.md 第 4 节,由 internal/service/llmclient 统一适配。
- SSE 转发格式:开始推送后不再套统一信封(见父任务 design.md 第 2 节例外)。
- Anthropic 协议 max_tokens 必填:用户未填时后端补默认值 1024 并在界面说明。
