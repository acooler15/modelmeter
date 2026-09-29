# research.md — CodeBuddy 模型配置勘探

调研日期:2026-09-30。调研对象:本机安装的 CodeBuddy IDE("CodeBuddy CN",
Tencent,VS Code 分支),程序代码位于
`%LOCALAPPDATA%/Programs/CodeBuddy CN/resources/app/`。以下结论全部提取自
其打包产物中的真实程序代码,是本任务实现的权威依据。

## 1. 目标文件与归属

- 路径:`~/.codebuddy/models.json`(Windows 下 `%USERPROFILE%\.codebuddy\models.json`)。
- 写入方有两处,共享同一 schema:
  - **IDE 主进程**(`out/main.js`):IPC `codebuddy:getLocalCustomModels` /
    `saveLocalCustomModel` / `deleteLocalCustomModel`,服务于设置面板
    "自定义模型"(settings.models.*,界面文案:"管理写入到 ~/.codebuddy/models.json
    的本地自定义模型配置")。
  - **内置 Agent 运行时**(`extensions/genie/out/extension/index.js`):
    `LocalCustomModelStoreImpl`,供聊天模型选择器消费;并用文件监听
    (1s debounce)在文件变化后热同步,无需重启。
- 目录名判定:`productName/appName 含 "workbuddy" → .workbuddy,否则
  USER_DATA_DIR_NAME(.codebuddy)`——CodeBuddy 与 WorkBuddy 是同一套代码的
  两个发行形态,schema 同源。

## 2. 顶层数据形态

- 权威缺省文件 `DEFAULT_LOCAL_MODELS_FILE = {"models": []}`(2 空格缩进)。
- 读侧(`loadFile` / `LocalCustomModelStoreImpl.loadFile`):ENOENT 或空文件
  视为 `{}`;JSON 解析后要求 `typeof === "object"`,否则抛
  `Invalid models.json format`。顶层是**对象形态**:
  `{ models: Model[], availableModels?: string[], ...未知键 }`。
- **裸数组形态不受支持**:JS 里数组也过 `typeof "object"` 检查,但
  `array.models` 为 undefined → 模型清单读作空;且 `JSON.stringify(数组)`
  会丢弃非索引属性,save 写入的模型会丢失。即 CodeBuddy 只有对象形态可用,
  这一点与 WorkBuddy 存储层"双形态皆合法"不同。
- `availableModels`:顶层可选字符串数组,语义是"模型选择器中可见的自定义
  模型 id 集合"(设置面板保存时按 visible 开关增删)。它只影响可见性,
  不影响模型是否被运行时加载;本适配器不读不改,原样保留。
- 写侧(`persistFile` / `persistLocalCustomModelsFile`):
  `mkdir -p` 后 `JSON.stringify(obj, null, 2) + "\n"` 整体写回;除 models
  数组与 availableModels 过滤外的顶层未知键原样保留。
- 另有**项目级**配置 `<workspace>/.codebuddy/models.json`(工作区第一目录),
  用户级优先、项目级合并;本适配器只管用户级文件,与 ModelMeter 其他
  适配器口径一致。
- 安全机制佐证:models.json 在 CodeBuddy 的"受保护配置路径"清单中
  (`isProtectedConfigPath`:settings.json/settings.local.json/models.json/
  mcp.json),Agent 写该文件需用户审批——第三方编辑器直接改文件是产品
  预期内的路径(设置面板本身也提供"打开本地配置文件"入口)。

## 3. 模型条目 schema(genie `normalize` 白名单 + 运行时消费键)

`LocalCustomModelStoreImpl.normalize`(save 时对条目整体重写的白名单):

| 文件键 | 类型 | 必填 | 缺省语义(读侧) |
| --- | --- | --- | --- |
| `id` | string | 是(trim 后非空,运行时按此过滤) | — |
| `name` | string | 否 | 回退 id |
| `vendor` | string | 否 | — |
| `url` | string | 否(项目级配置运行时要求非空) | — |
| `apiKey` | string | 否 | **凭据,绝不读取/回传/写回** |
| `maxInputTokens` | number | 否 | 文件未写该键不出现 |
| `maxOutputTokens` | number | 否 | 同上 |
| `temperature` | number | 否 | 同上 |
| `supportsToolCall` | boolean | 否 | falsy |
| `supportsImages` | boolean | 否 | falsy |
| `supportsReasoning` | boolean | 否 | falsy |

normalize 之外的运行时消费键(save 白名单不含,但读侧生效,IDE 面板保存
会丢掉它们——本适配器按红线原样保留):

- `disabled`:boolean。运行时加载为每条补 `{disabled: false, ...entry}`,
  条目可显式覆盖为 true 禁用该模型。
- `onlyReasoning`:boolean。`isReasoningCapableModel = supportsReasoning ||
  onlyReasoning || reasoning.supportedEfforts.length > 0`;
  `onlyReasoning === true` 时思考恒开。
- `reasoning`:object(非数组时**原样透传保留**,键内字段不做白名单):
  - `defaultEffort`:string,默认推理档位。
  - `supportedEfforts`:string[],合法档位集;
    `resolveReasoningEffort`:请求档位在列表内则用之,否则兜底
    `reasoning.effort`(legacy)再 `defaultEffort`。
  - `canDisableThinking`:boolean,仅显式 `false` 时思考恒开(缺省即 true,
    与 WorkBuddy 面板语义一致)。
  - legacy `effort`:string,`defaultEffort` 缺失时的兜底读取键(与
    WorkBuddy 适配器 `default_effort ?? effort` 口径相同)。
  - `summary` 等其他键:透传,不编辑。

## 4. CodeBuddy 特有差异(相对 WorkBuddy)

1. **仅对象形态**:裸数组不构成可用的 CodeBuddy 配置(读作空、写即丢失),
   适配器应拒绝(4402)而不是兼容。
2. **无 `useCustomProtocol` 键**:CodeBuddy 全部程序代码中该键零出现,
   不进入白名单与列描述。
3. **无"默认模型"概念**:models.json 不承载默认模型选择(选择态存
   会话/全局状态),能力位恒 false,不实现 DefaultModelSetter。
4. IDE 面板保存有损(normalize 丢弃未知键与 disabled 等),ModelMeter
   的树编辑写回比它保守,不受影响;文件被 CodeBuddy 重写后未知键丢失是
   CodeBuddy 自身行为。
5. 文件变更被 genie 运行时 watch(1s debounce)自动热加载,改后无需重启
   (提示文案仍建议生效性检查,不写死"必须重启")。

## 5. 本机真实文件佐证(2026-09-30,凭据已脱敏)

`~/.codebuddy/models.json` 顶层为 `{"models": [...]}`,两条 deepseek 条目:
首条含 id/name/vendor/url/apiKey/supportsToolCall/supportsImages/
supportsReasoning/reasoning{defaultEffort,supportedEfforts};次条另带
maxInputTokens/maxOutputTokens,reasoning 仅 defaultEffort。与第 3 节
schema 完全吻合。

## 6. 实现口径结论

- 白名单编辑键(WorkBuddy 适配器去掉 `use_custom_protocol`,其余一致):
  name、url、disabled、supports_tool_call、supports_images、
  supports_reasoning、only_reasoning、default_effort、supported_efforts、
  can_disable_thinking、max_input_tokens、max_output_tokens、temperature。
- 只读展示:model_id(id)、vendor;api_key 永不出现。
- 顶层形态:仅对象;`models` 数组缺失或非数组时按空清单处理(对齐
  `buildResponse` 的 `Array.isArray(ir.models)?...:[]`),顶层其余键
  (availableModels 等)写回零丢失。
- 条目遍历跳过非对象元素;无字符串 id 的条目按运行时口径跳过。
- 定位:按 `id` 首个匹配;`disabled`/`only_reasoning`/能力键缺省 false,
  `can_disable_thinking` 缺省 true(仅 false 才有意义),数字键文件未写
  不出现,`default_effort` 兜底 legacy `effort`。
