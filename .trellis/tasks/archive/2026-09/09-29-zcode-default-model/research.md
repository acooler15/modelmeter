# 勘探记录:ZCode 模型配置真实结构(第二期,2026-09-29)

本文是本任务的实现依据,基于对 ZCode 桌面端打包产物 `zcode.cjs` 的代码分析与本机真实配置文件验证。**所有凭据值一律脱敏(前 6 后 4),本文不引用任何真实密钥。**

## 0. 勘误:推翻前任务 research.md 的一处结论

前任务 `.trellis/tasks/09-29-agentconf-realign/research.md` 第 17 行记录:**"没有'当前选中模型'字段"**。

**本期结论:该结论有误。** 真实情况:

- ZCode 的 zod schema 中存在 `config.defaultModelSelection`(CLI 登录流程 `saveConfiguredDefault({providerId, modelId})` 会写它);
- 本机文件之所以没有该字段,只是因为用户从未设置过默认模型——"文件里暂时没有"被误判成了"schema 里不存在"。
- 因此前任务 prd.md 中"'当前选中模型'明确出局"的决策需要部分回调:**默认模型本次纳入管理范围**(读 + 设置/清除)。

## 1. 权威来源

| 来源 | 路径 | 说明 |
|---|---|---|
| ZCode 打包产物 | `C:\Users\<用户名>\AppData\Local\Programs\ZCode\resources\glm\zcode.cjs`(14.8MB 单文件 CJS) | Electron 桌面端内置 CLI,模型配置的读写逻辑都在这里 |
| 个人配置(读/写目标) | `~/.zcode/v2/provider_config.json` | 用户可写,本任务唯一操作目标 |
| 凭据存储 | `~/.zcode/v2/credentials.json` | `enc:v1:` AES-256-GCM 加密,**零接触** |
| 内置目录缓存 | `~/.zcode/v2/runtime/provider/<平台>/<版本>/endpoint-*/zcode-builtin.json` | 只读,与个人文件合并出最终模型能力 |

注意:`~/.zcode/cli-bin/` 是用户自己的代码,与 ZCode 官方实现无关(用户 2026-09-29 明确),任何调研都应忽略。

## 2. `provider_config.json` 权威 schema(zcode.cjs 中的 zod 定义)

```
根: { schemaVersion: z.literal(1), config: z.object({...}).strict() }.strict()
config:
  providerOrder?:        string[](min 1)
  providerConfigRules:   { providerRules: [...] }          // 个人文件只写 providerRules
  modelConfigRules:      { providerModelRules: [...],
                           manualProviderModelRules: [...] }
  defaultModelSelection: { providerId, modelId, options?: { reasoningLevel? } }   // ★ 本次新增支持
```

关键约束:

1. **schemaVersion 必须是 1**(`z.literal(1)`)。本机真实文件实测为 1。任何 != 1 的文件都不能当作 ZCode 配置编辑。
2. **根对象与 config 均 `.strict()`**——schema 之外的顶层键会导致 ZCode 解析失败。我们只做树内白名单编辑,天然安全;但 fixture 与测试要守住"不新增未知顶层键"。
3. 个人 provider 规则(`providerRules[]`):`providerId`(唯一,不得以 `account:` 开头)、`templateId?`、`providerName?`、`enabled?`、`config.group`(个人固定 `standard-personal`,可省略)、`config.access`(个人必须 `type:"api-key"` 且 apiKey 必填——本机存在缺 apiKey 的残缺条目,ZCode 自己也容忍)、`config.api{type, baseUrl?}`、`config.modelOrder[]`、`config.personalModelIds[]`。
4. `config.api.type` 枚举:**`anthropic-messages` | `openai-chat-completions` | `openai-responses`**(三种协议)。baseUrl 对个人 provider 可为 null/缺省。
5. `providerModelRules[]` 与 `manualProviderModelRules[]` 的 (providerId, modelId) **不得跨数组重复**(zod superRefine 校验)——往 providerModelRules 追加新节点前必须先查 manual 数组,否则写出 ZCode 拒收的文件。
6. 本机文件当前无 `defaultModelSelection` 字段(未设置过);设置后出现在 `config` 下。

## 3. 模型规则节点完整能力集(个人文件 `Uj`)

```
config: {
  enabled: bool
  properties?: {
    contextWindow: int > 0
    inputFormat?: { supportsImage?: bool }        // ★ 本次纳入白名单
    supportsJsonSchemaOutput?: bool               // ★ 本次纳入白名单
    supportsNativeWebSearch? / supportsMidConversationSystem?   // 本次不做(未在本机文件观测到使用)
  }
  optionSpecs?: {
    reasoningLevel?: { values?: string[], map?: string }    // values ★ 本次纳入白名单
    maxOutputTokens?: { max: int > 0, map?: string }
  }
}
```

本机真实文件中出现过的写法(样例,已脱敏无关值):

- `{"enabled":true,"properties":{"contextWindow":360000},"optionSpecs":{"maxOutputTokens":{"max":8192}}}`
- `{"enabled":true,"properties":{"contextWindow":1000000,"inputFormat":{"supportsImage":true}},"optionSpecs":{"reasoningLevel":{"values":["low","medium","high","xhigh","max"]},"maxOutputTokens":{"max":8192}}}`
- `{"enabled":true,"properties":{"contextWindow":1000000,"inputFormat":{"supportsImage":true},"supportsJsonSchemaOutput":false},...}`
- 规则节点可整体不存在(无任何覆盖);`reasoningLevel.values` 样例值域:`["low","medium","high","xhigh","max"]`(自定义模型)、内置兜底规则为 `["disabled","enabled"]`。

`manualProviderModelRules[]` 是"手动添加的模型"规则,能力集同上(本机为空数组)。其条目结构与 providerModelRules 相同,仅 zod 校验差异。

## 4. `defaultModelSelection` 详解

- 位置:`config.defaultModelSelection`,结构 `{providerId, modelId, options?: {reasoningLevel?}}`。
- 写入者:CLI 的 `saveConfiguredDefault({providerId, modelId})`;模型选择标识在 UI 层格式为 `providerId/modelId$reasoningLevel`(`parseModelPickerValue` 解析),落盘时拆回结构化字段。
- 读取:ZCode 启动时合并 schemaVersion 校验后产出 `{providers, models, providerOrder, defaultModelSelection?}`。
- providerId 取值:个人供应商为 `new-provider-N` 等自定义 id;内置账户型为 `account:bigmodel-individual-coding-plan` 等(本机模型规则中已出现 `account:` 前缀的规则条目,但 providerRules 中没有——内置供应商不落个人 providerRules)。
- **注意**:reasoningLevel 是纯字符串,无枚举限制(zod 层面);语义上应取该模型规则 `optionSpecs.reasoningLevel.values` 中的值,但规则未定义 values 时不应拒绝。

## 5. 环境变量与其他路径(备忘,本期不做)

- 路径覆盖:`ZCODE_PERSONAL_PROVIDER_CONFIG_FILE` + `ZCODE_BUILTIN_PROVIDER_CONFIG_FILE`(必须成对)、`ZCODE_DATA_BASE_DIR`(默认 `~`)。本工具继续按默认路径 `~/.zcode/v2/provider_config.json` 处理。
- 无 `ZCODE_MODEL` 这类模型选择环境变量;模型选择只落 `defaultModelSelection`。
- credentials.json 加密格式:`enc:v1:` + base64url(IV) + `.` + base64url(tag) + `.` + base64url(密文),key = sha256(密钥种子)。**本期不读取不写回**。

## 6. 安全红线(沿用前任务)

1. 绝不读取、回传、写日志、写回 `config.access.apiKey` 与 credentials.json 的任何内容。
2. 白名单之外的键(含 apiKey、templateId、manualProviderModelRules、未知键)逐字节原样保留。
3. 写回前自动备份(backup.go,10 份滚动)、原子写(临时文件 + 改名)。
4. `defaultModelSelection` 节点是例外:严格 schema 不允许未知键,该节点整体替换为规范形状(`{providerId, modelId, options?}`),不保留其内部未知键——存在于该节点的未知键只可能是坏文件,保留反而会让 ZCode 拒载。
5. ZCode 桌面端可能持有内存态并在退出时覆盖外部修改——写回成功后界面提示"建议重启 ZCode 使配置生效"(沿用既有文案)。
