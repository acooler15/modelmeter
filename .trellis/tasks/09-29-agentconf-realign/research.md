# 勘探记录:ZCode / WorkBuddy 模型配置真实结构(2026-09-29,本机 Windows)

本文是本任务的实现依据。**两份文件均含明文 API Key,本文一律不引用任何真实凭据值。**

## 勘误:推翻 P1-6 的勘探结论

P1-6 归档 design.md 认定 ZCode 配置在 `~/.zcode/cli-bin/`(model-selection.json + provider_config.cli.json),并把 `~/.zcode/v2/provider_config.json` 记为"同构副本(桌面端)"。

**用户已确认(2026-09-29):结论反了。**

- `~/.zcode/v2/provider_config.json` 才是 ZCode 实际使用的唯一模型配置。
- `~/.zcode/cli-bin/` 下两份文件是用户此前手工修改的遗留物,ZCode 不再使用。
- 因此 P1-6 交付的 zcode.go(读写 cli-bin/model-selection.json)目标文件整体失效;"切换当前模型"功能随之废弃(用户明确不需要)。

## ZCode:`~/.zcode/v2/provider_config.json`

顶层:`schemaVersion`(数字)+ `config`。**没有**"当前选中模型"字段。

```
config.providerOrder[]                       # 供应商显示顺序(providerId 列表)
config.providerConfigRules.providerRules[]   # 供应商定义
config.modelConfigRules
  .providerModelRules[]                      # 按 (providerId, modelId) 的模型级规则
  .manualProviderModelRules[]                # 当前为空数组
```

providerRules 条目:

| 字段 | 说明 |
|---|---|
| providerId | 唯一键。UI 新建为 new-provider、new-provider-2… 递增;内置账户型用 `account:` 前缀(只出现在模型规则,不在 providerRules) |
| templateId | 仅模板型供应商有(deepseek、xiaomi-mimo);有它时 api.baseUrl 可省略 |
| providerName | 展示名 |
| enabled | 可选布尔,部分条目缺失,缺省视为启用 |
| config.access | `{type:"api-key", apiKey:"<敏感>"}` |
| config.api | `{type:"openai-responses"\|"openai-chat-completions", baseUrl?}` |
| config.personalModelIds[] | 自定义模型清单 |
| config.modelOrder[] | 模型显示顺序 |

providerModelRules 条目(按 providerId+modelId 定位):

| 字段 | 说明 |
|---|---|
| config.enabled | 布尔 |
| config.properties | contextWindow(数字)、inputFormat.supportsImage、supportsJsonSchemaOutput |
| config.optionSpecs | maxOutputTokens.max、reasoningLevel.values[](样例 ["low","medium","high","xhigh","max"]) |

- 规则节点可以不存在(该模型无任何覆盖)。
- 存在残缺条目:new-provider-7("新供应商")无 apiKey、无 api 节点——解析必须容忍。
- 本机样例规模:8 个供应商、约 30 条模型规则。

## WorkBuddy:`~/.workbuddy/models.json`

顶层是**裸数组**,无包装对象、无 schemaVersion。每个元素一个模型:

| 字段 | 类型 | 说明 |
|---|---|---|
| id | string | 模型标识(本机样例 deepseek-v4-flash) |
| name | string | 显示名 |
| vendor | string | 供应商标签 |
| url | string | **完整 endpoint**(含路径,如 https://api.deepseek.com/chat/completions),不是 baseUrl |
| apiKey | string | **每条独立的明文 key**(与 ZCode 的集中式不同) |
| supportsToolCall / supportsImages / supportsReasoning | 布尔 | 能力开关 |
| reasoning | object | defaultEffort + supportedEfforts[](本机样例 ["high","max"]) |

- 本机样例仅 1 条记录。
- `~/.workbuddy/settings.json` 是应用偏好(沙箱规则、插件开关),与模型无关,不纳入本任务。

## 与 P1-6 实现的差异总览

| 维度 | P1-6 实现 | 本任务 |
|---|---|---|
| ZCode 目标文件 | cli-bin/model-selection.json(已废弃) | v2/provider_config.json |
| ZCode 编辑语义 | 切换当前模型(两个键) | 编辑模型配置(enabled/上下文窗口/最大输出等) |
| WorkBuddy | 占位(探测路径全错) | models.json 真实读写 |
| 接口形态 | 单值动态表单(Fields/Values) | 模型清单(列表读取 + 白名单局部修改) |

## 安全红线(沿用 P1-6 并适配新文件)

1. 两份文件均含明文 apiKey:绝不读取、回传、写日志、写回凭据字段。
2. 读侧:解析结构刻意不声明凭据字段(json 忽略未声明键),或树遍历时凭据节点原样放回。
3. 写侧:白名单之外的键(含 apiKey 与未知键)逐字节原样保留;写回前备份、原子写。
4. ZCode 桌面端可能正在运行并持有配置内存态,退出时可能覆盖外部修改——写回成功后界面提示"建议重启 ZCode 使配置生效"。
