# 放宽 ZCode 新建供应商落点的模型判重

## Goal

ZCode「新建供应商」添加模型改为不再全量判重:同 ID 模型可挂到不同供应商,与 ZCode 原生能力对齐;WorkBuddy/CodeBuddy 扁平清单判重保持不变。

## 背景与调查结论(2026-10-02)

用户反馈:ZCode 原生允许不同供应商挂同一个模型 ID(模型规则按 `(providerId, modelId)` 二元组定位,`personalModelIds` 每供应商各一份),但本工具「从接口添加模型 → 新建供应商」时,同 ID 已存在于任何供应商即被跳过。逐一核查三个适配器后的结论:

- **ZCode 挂「已有供应商」**:按目标供应商自身清单判重(`zcode.go` `addModelsToExistingProvider`),跨供应商同 ID 本就允许——**不改**。
- **ZCode「新建供应商」**:全量判重(`zcode.go` `addModelsByNewProvider`),ID 在任何供应商下已存在即 skipped——新建供应商自身 `personalModelIds` 为空,不存在内部冲突,该限制无技术必要性,属人为产品取舍——**本次放宽对象**。
- **WorkBuddy / CodeBuddy**:无供应商层,配置为单个 models 扁平数组,条目自带 `url`/`apiKey`,模型 ID 即主键(编辑按首个同 id 条目定位、`availableModels` 为纯 id 列表、运行时按 id 选模型),同 ID 不同端点在其数据模型里无法表达,按 id 判重是语义必需——**不改**。

## Requirements

1. `ZCodeAgent.addModelsByNewProvider` 不再把「所有供应商清单」预填进判重集合:所选 `model_ids` 全部追加进新建供应商的 `personalModelIds`;请求内重复 id 仍由 `splitAddIDs` 兜底去重(传空 `existing` 即可)。
2. Mode=new 下不再有「跨供应商已存在 → skipped」路径,原「全部已存在 → 零落盘」分支随之移除;请求内重复 id 由共用函数 `splitAddIDs` 去重,其第二次出现计入 `skipped`(与 WorkBuddy/CodeBuddy 同口径,前端选择器不会产生重复 id,仅兜底)。
3. 挂「已有供应商」模式(`addModelsToExistingProvider`)判重行为与文案不变。
4. WorkBuddy/CodeBuddy 的 AddModels 判重逻辑与注释不变。
5. `zcode.go` 包级文档注释中关于添加路径「已存在跳过」的描述同步更新(仅 Mode=new 语义变化;Mode=existing 描述保留)。

## Constraints

- 新建供应商节点的键集与形状完全不变(providerId 惯例、`access.apiKey` 写入例外、`providerOrder` 追加等均维持现状)。
- 备份先于写回、原子写回、日志不记 URL/Key 值等红线不变。
- 前端无需改动:`skipped` 恒空时「跳过 N 个(已存在)」提示自然不出现。

## Acceptance Criteria

- [x] ZCode 配置中供应商 A 已有模型 `glm-x` 时,经「新建供应商」落点从另一接口添加 `glm-x`:新供应商节点创建成功,`glm-x` 进入其 `personalModelIds`,结果 `added` 含 `glm-x`、`skipped` 为空。(`TestZCode_AddModels_新建供应商`,deepseek-chat 同 id 跨供应商场景)
- [x] 同一请求内重复的 `model_ids` 只添加一次。(`TestZCode_AddModels_新建供应商_请求内重复只加一次`)
- [x] 挂「已有供应商」模式判重不变(目标供应商内已存在仍跳过),既有测试保持通过。(`TestZCode_AddModels_挂已有供应商` 等挂靠系列)
- [x] WorkBuddy/CodeBuddy AddModels 行为不变,其既有测试全部通过。(两文件零改动,全量测试通过)
- [x] `go build ./...` 与 `go test ./...` 全绿。(-count=1 强制重跑确认)
