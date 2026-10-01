# 接口配置页 API Key 明文显示

## Goal

接口配置列表的 API Key 列由脱敏值改为明文显示:后端 ProviderView 返回明文
api_key,前端表格直接展示完整 Key,便于用户随时查看已保存凭据。

## 背景与决策

现状:后端 `internal/service/provider.go` 的 `MaskKey` 把 API Key 脱敏为
`sk-****abcd` 形式,经 `ProviderView.APIKeyMasked` 下发,前端接口配置页只展示
脱敏值;规范 `.trellis/spec/backend/quality-guidelines.md` 亦约定"对外一律脱敏
视图"。

本项目为本地单用户工具,凭据是用户自己录入、自己查看,脱敏反而妨碍使用。
产品决策(用户 2026-10-01 明确):**接口配置的 API Key 对用户明文可见**。
该决策与既有规范冲突,属于有意的例外:收尾时必须同步修订规范条目,明确
ProviderView 明文回显的例外地位;llmclient 请求头、日志、错误信息仍不得
出现凭据之外的泄端口径不变,New API 令牌与 Agent 模型凭据的脱敏展示不变
(不在本次范围)。

## Requirements

1. 后端:`ProviderView` 的 `api_key_masked` 字段替换为明文 `api_key`
   (json tag `api_key`),列表/新增/编辑三个接口共用该视图,一并生效。
2. 后端:provider 相关写路径日志维持现状(`key_updated` 布尔值),不输出
   Key 明文;llmclient、agentconf、newapi 的凭据处理逻辑零改动。
3. 前端:`types/provider.ts` 的 `ProviderView` 字段同步为 `api_key`,注释
   同步;"后端永不输出明文 Key"等过时注释一并修正。
4. 前端:`ProviderList.vue` 表格 API Key 列展示完整明文 Key;`isProviderRow`
   类型收窄同步。
5. 前端:编辑弹窗交互不变——Key 输入框留空表示沿用原 Key,不回填明文;
   新增仍必填。
6. 测试:`internal/service/provider_test.go` 中脱敏相关断言改为明文断言;
   `MaskKey` 及其测试保留(New API 令牌仍用它)。

## Acceptance Criteria

- [ ] `GET /api/providers`、`POST /api/providers`、`PUT /api/providers/:id`
      响应含明文 `api_key` 字段,不再含 `api_key_masked`。
- [ ] 接口配置页表格 API Key 列显示完整 Key 明文。
- [ ] New API 页令牌仍显示脱敏值(`token_masked`);Agent 模型清单凭据仍脱敏。
- [ ] 日志与错误信息中不出现任何 API Key 明文。
- [ ] `go test ./...` 全部通过;前端 `npm run build`(含 vue-tsc)通过。
- [ ] `.trellis/spec/backend/quality-guidelines.md` 的"对外一律脱敏视图"
      条目已修订,写明接口配置视图的用户自见明文例外及适用边界。

## Out of Scope

- New API 令牌明文显示。
- Agent 模型清单凭据明文显示。
- 编辑弹窗回填明文 Key。
