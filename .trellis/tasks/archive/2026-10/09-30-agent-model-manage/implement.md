# implement.md — Agent 配置页 tab 化与模型清单管理增强

> 执行计划。设计契约见 `design.md`,需求见 `prd.md`。按序执行,每步末尾的验证通过后再进入下一步。

## 前置

- [ ] `mise install`(若项目有工具链配置)并确认 Go/Node 可用;`go vet ./...` 基线通过。

## 阶段 A:后端能力

- [ ] A1 `agentconf.go`:新增 `ModelRef`/`AddTarget`/`ModelSource`/`AddTargetSpec`/`AddModelsRequest`/`AddModelsResult` 类型,`Agent` 接口新增 `RemoveModels`/`AddModels`,`Snapshot` 新增 `SupportsAddModels`/`SupportsRemoveModels`/`AddTargets` 字段;包注释同步细化安全红线(新增写入凭据的显式例外)。验证:`go build ./...`(三适配器未实现新方法前可先占位,或本步与 A2–A4 连续完成后统一 build)。
- [ ] A2 `agents/zcode.go`:实现 `RemoveModels`(双清单移除 + providerModelRules 节点清理 + manual 不动 + 供应商节点保留 + 整批校验)与 `AddModels`(**挂靠**模式:personalModelIds 追加 + 判重跳过;**新建供应商**模式:providerId 按 `new-provider-N` 惯例取最小未用序号、最小合法节点逐键落盘、api.type 校验、providerOrder 追加),Snapshot 填能力位与 `AddTargets`;补 `zcode_test.go` 用例(设计"测试设计"节全项)。验证:`go test ./internal/service/agentconf/...`。
- [ ] A3 `agents/workbuddy.go`:实现 `RemoveModels`/`AddModels`(两形态、availableModels 同步、新条目 5 键,url 取 `Source.EndpointURL`);补 `workbuddy_test.go`。验证:同上。
- [ ] A4 `agents/codebuddy.go`:实现 `RemoveModels`/`AddModels`(对象形态、availableModels 同步、缺失不创建);补 `codebuddy_test.go`;agents 包一致性测试补能力位断言。验证:`go test ./internal/service/agentconf/...`。
- [ ] A5 `handler/agent.go` + `router.go`:新增 `AgentModelsRemove`/`AgentModelsAdd`(含 DB 装配 `ModelSource`:`base_url` 覆盖、`llmclient.BuildURL` 派生 `EndpointURL`、1404/4403 分支)并注册路由;补 `agent_test.go` 路由级用例。验证:`go vet ./... && go test ./...`。

**回滚点 A**:后端全部为增量(新方法/新路由),此阶段完成后系统行为与现状完全一致(旧前端不调用新端点),可独立提交或整体回退。

## 阶段 B:前端

- [ ] B1 `types/agent.ts` + `api/agent.ts`:新增类型与 `removeAgentModels`/`addAgentModels` 封装。验证:`cd web && npx vue-tsc --noEmit`。
- [ ] B2 抽取 `components/agent/AgentFieldInput.vue`(列类型→受控输入),`AgentModelCard` 表格单元格改用之;行为与现状等价(纯重构)。验证:`npm run build` + 手动核对表格渲染不回归。
- [ ] B3 `AgentView.vue` tab 化(`el-tabs` + lazy pane)。验证:`npm run dev` 目检各 Agent tab。
- [ ] B4 `AgentModelCard.vue`:多选列、"批量删除"(二次确认)、"操作"列(编辑弹窗 + 单行删除)、编辑弹窗(按 editableColumns 渲染,确定即提交单行 patch)。
- [ ] B5 新增 `components/agent/AgentImportModelsDialog.vue`:选接口 → 拉模型 → 多选/全选 → 接口地址预填可改(WB/CB 提示派生 endpoint)→ ZCode 落点下拉(已有供应商 + "新建供应商…",新建展开显示名与 API 协议)→ 凭据警示确认 → 成功/跳过反馈;卡片头部接"从接口添加模型"按钮(能力位+found 控制)。
- [ ] B6 全量校验:`npm run lint && npm run build`;`go vet ./... && go test ./...` 收尾确认。

**回滚点 B**:前端独立于后端回退(新端点未被旧前端调用即无影响);B2 为纯重构步,若引入回归可单独还原该步。

## 收尾

- [ ] C1 手动冒烟(本机装有三工具时,重点):对真实配置执行"ZCode 新建供应商添加 1 个模型 → 确认 ZCode 可加载该配置 → 删除该模型与供应商下其余新增模型";WorkBuddy/CodeBuddy 各执行一次添加/删除;确认备份生成、清单正确、界面无凭据。若 ZCode 实测对新建供应商节点拒载,立即回退该分支并回到规划修订节点形状。
- [ ] C2 规范更新评估:安全红线细化(新增写入凭据的显式例外)是否需沉淀到 `.trellis/spec/backend/`(经 `trellis-update-spec` 判断)。
- [ ] C3 提交(遵循 3.4 提交规范),任务归档。

## 验证命令汇总

```bash
go vet ./... && go test ./...
cd web && npm run lint && npm run build
```

## 风险与注意

- **ZCode 新建供应商的 strict 校验风险最高**:节点只写设计所列键集,任何额外键(含空值键)都可能让 ZCode 拒载整份配置;C1 冒烟必须真实加载验证。拒绝场景的回退 = 还原备份 + 修订节点形状。
- WorkBuddy/CodeBuddy 的 `url` 是**完整 endpoint** 语义(勘探明确),由后端 `llmclient.BuildURL(BaseURL, "/v1/chat/completions")` 派生;写入后用户仍可在表格中编辑 url 纠偏。
- ZCode `personalModelIds` 语义依据勘探("自定义模型清单");若实测发现挂靠模式下 ZCode 对不在 `modelOrder` 的自定义模型不生效,回退方案为同时追加到 `modelOrder`(实现时以本机 ZCode 实测为准,写入测试注释)。
- 凭据只在 `AgentModelsAdd` handler→`ModelSource`→适配器落盘的单一通路上流动;评审时专项核对响应体、日志、错误信息三处无 key。
- 编辑弹窗与行内草稿并存:弹窗确定即提交并整体刷新,避免两套脏状态叠加(设计已定,实现勿改动草稿比对逻辑)。
