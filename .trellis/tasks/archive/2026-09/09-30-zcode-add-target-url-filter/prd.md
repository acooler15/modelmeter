# ZCode 添加模型落点按 URL 过滤与一致性校验

## Goal

从接口添加模型到 ZCode 时,「添加到」下拉只显示与来源接口 URL 一致的既有供应商;
后端同步携带落点 baseUrl 并在 existing 落点做一致性校验,杜绝把模型挂到
URL 不同的供应商上(挂上去后 ZCode 会用落点自己的 URL/Key 调这些模型,调不通)。

## Background

现状:`AddTarget` 只有 `provider_id`/`provider_name` 两个字段,前端落点下拉
看不到目标供应商指向哪个 URL;`addModelsToExistingProvider` 把新模型 ID 直接
追加进目标供应商 `config.personalModelIds`,完全不消费 Source,也没有任何
URL 一致性校验。因此从接口 A(UrlA)拉取的模型可以挂到 baseUrl=UrlB 的
ZCode 供应商上,添加成功但必然调不通。

产品口径(用户拍板):**URL 不同的供应商根本不显示在下拉里**,从源头避免,
而不是事后警示。本任务同时在前端(过滤展示)与后端(强校验)两侧落实同一口径,
后端校验防止绕过前端的直连请求与前端状态竞争。

不在本任务范围:WorkBuddy/CodeBuddy 添加时选择接口类型(api_type);
ZCode 挂已有供应商时切换协议(协议属于落点供应商节点本身)。

## Requirements

### 后端(`internal/service/agentconf/agents/zcode.go` 等)

1. `AddTarget`(`agentconf.go`)增加 `BaseURL string` 字段
   (`json:"base_url,omitempty"`);`collectZCodeAddTargets` 从每个供应商节点
   的 `config.api.baseUrl` 读取填充。baseUrl 为非敏感字段,允许进 Snapshot
   (与 provider_name 同级);日志红线不变:URL 不进日志。
2. `addModelsToExistingProvider` 增加 URL 一致性校验(定位目标供应商之后、
   备份与写回之前):
   - 归一化口径:去除首尾空白 + 去除尾部 `/`,其余逐字符精确比较(不做
     大小写/协议归一,保守);前后端共用此口径。
   - 目标节点 `config.api.baseUrl` 归一化后与 `req.Source.BaseURL` 归一化后
     不相等 → 报 4403(CodeAgentInvalid),文案说明二者不一致、建议改用
     「新建供应商」落点;**不备份、不落盘**。
   - 目标 baseUrl 为空或来源 BaseURL 为空 → 同样报 4403(无法确认一致,
     与前端"空 URL 落点不显示"口径对齐),文案区分缺哪一侧。
3. Mode=new 路径不受影响;删除/编辑路径不受影响。
4. 测试(`zcode_test.go`,必要时同步 `consistency_test.go`):
   - Snapshot 的 add_targets 各条目带 base_url。
   - AddModels existing:URL 归一化相等(含尾斜杠差异)→ 成功追加且行为
     与现状一致(不写凭据);不一致 → 4403 且文件零改动(无备份产生);
     目标 baseUrl 缺失 → 4403;Source.BaseURL 为空 → 4403。

### 前端(`web/src/types/agent.ts`、`web/src/components/agent/AgentImportModelsDialog.vue`)

1. `AgentAddTarget` 类型增加 `base_url?: string`。
2. 落点候选过滤:新增 `candidateTargets` computed,只保留归一化
   `base_url === effectiveBase` 的既有供应商(effectiveBase 沿用现有定义:
   接口地址输入 trim 非空则覆盖接口记录 Base URL);「新建供应商…」选项恒在。
3. 状态联动:effectiveBase 变化(选择接口、修改接口地址输入)后,若当前
   targetKey 不在 candidateTargets 中,重置为候选第一项,候选为空则重置为
   NEW_TARGET;弹窗打开时 targetKey 初始化为 NEW_TARGET(不再预选
   add_targets[0]——未选来源接口前不存在"URL 一致"的事实)。
4. 落点下拉 option 展示该供应商的 baseUrl 副文本(复用 provider-url 样式),
   让用户可确认落点指向。
5. 归一化函数注释声明与后端口径一致。

## Constraints

- 凭据零接触红线不变:apiKey 不读不回传不进日志;baseUrl 非敏感但同样不进日志
  (沿用 WorkBuddy/CodeBuddy"日志只记数量与定位键,不记 url/key 值"的既有取舍)。
- 不改变 ZCode 配置文件 schema:只读 `config.api.baseUrl`,不新增不修改任何键。
- 归一化保守:仅 trim + 去尾斜杠;不引入 URL 解析器做语义级比较。
- 错误码沿用 4xxx 段现有 CodeAgentInvalid(4403),不新增错误码。
- 文档与代码注释用中文,标识符英文。

## Acceptance Criteria

- [ ] `/api/agents` 返回的 ZCode Snapshot 中 add_targets 各条目含 base_url。
- [ ] 前端:选中接口后落点下拉仅列出与有效 URL 一致的既有供应商,option 内
      可见其 URL;修改「接口地址」输入后候选实时刷新;已选落点被过滤掉时
      自动回落到「新建供应商…」。
- [ ] 后端:existing 落点在 URL 不一致或任一侧为空时报 4403,且不产生备份、
      配置文件零改动;归一化相等(仅尾斜杠差异)时正常添加。
- [ ] Mode=new(新建供应商)行为与现状一致,含 api_type 校验。
- [ ] `go build ./...`、`go vet ./...`、agents 包测试、前端
      `vue-tsc --noEmit`(或等价 build)全部通过。
