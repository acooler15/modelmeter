# 勘探记录:WorkBuddy 模型配置真实结构(2026-09-30)

本文是本任务的实现依据,基于对 WorkBuddy 桌面端打包产物 `app.asar` 的代码分析与本机真实配置文件验证。**所有凭据一律不引用,本文不出现任何真实密钥。**

## 1. 权威来源

| 来源 | 路径 | 说明 |
|---|---|---|
| WorkBuddy 桌面端 | `C:\Users\<用户名>\AppData\Local\Programs\WorkBuddy\` | Electron 应用,版本 5.6.2-wb |
| 打包产物 | `resources\app.asar`(317MB,20553 个文件) | 主进程代码在 `/main/*.js`,渲染层在 `/renderer/assets/*.js` |
| 自定义模型校验 | asar 内 `main/module.app-server.js`(源自 `packages/agent-provider/src/backend/local-custom-model-validation.ts`) | `isValidLocalCustomModel` / `coerceOptionalNumber`,权威 schema 校验 |
| 存储层 | asar 内 `main/credential-protection.js`(源自 `packages/core/src/node/credential-protection/models-json-storage.ts`) | `ModelsJsonStorage` 读改写、宽容解析、原子写 |
| 设置面板 | asar 内 `renderer/assets/ModelsSettingsPanel-*.js` | UI 可编辑字段与保存序列化口径 |
| 配置文件(读/写目标) | `~/.workbuddy/models.json` | 用户可写,本任务唯一操作目标 |

WorkBuddy 是 CodeBuddy(腾讯)的换壳发行版,代码内部仍大量使用 `codebuddy` 命名;运行时数据目录为 `~/.workbuddy/`(可用 `WORKBUDDY_CONFIG_DIR` / `CODEBUDDY_CONFIG_DIR` 环境变量重定向,ModelMeter 不跟随该重定向,与 ZCode 适配器口径一致)。

## 2. models.json 文件形态(两种皆合法)

存储层注释与 `DesktopModelsRepo.readModelsFile`("读本地 models.json,兼容数组和 **{ models: [...] }** 两种格式")明确:

```
形态 A(当前主流):  [ {model}, {model}, ... ]           // 裸数组
形态 B(legacy 兼容): { "models": [ {model}, ... ],
                       "availableModels": ["id", ...] }  // 对象包装
```

- `availableModels?`: string[],对象形态下的可选项,按 id 过滤可用模型。
- 自定义模型加载后 id 会被加 `custom-local:` 前缀(可配置的产品开关)、tags 自动补 `custom`,均为内存行为,**不回写文件**;ModelMeter 无需关心。
- 坏文件处理:存储层宽容解析(剥 BOM、删结构位尾随逗号,故意不去 `//` 注释,因 url 含 `//`);不可修复时备份为 `models.json.bak-<内容哈希12>` 后当空配置。
- 写回:`JSON.stringify(doc, null, 2)` 2 空格缩进 + 原子写 + 文件锁——与 ModelMeter `WriteJSONFile` 格式一致。
- ⚠️ WorkBuddy 自己的桌面端保存路径(`saveLocalCustomModel`)会把对象形态整体重写回裸数组(丢失 `availableModels`);ModelMeter 更保守:**保持原形态**,对象形态只原地替换 `models` 数组、其余顶层键零丢失。

## 3. 单条模型权威 schema(`isValidLocalCustomModel` + `normalizeCustomModel` + 设置面板)

```
model = {
  id:                string   必填非空(定位键)
  name?:             string
  vendor?:           string
  url?:              string
  apiKey?:           string   凭据,零接触(见 §5)
  maxInputTokens?:   number   兼容数字字符串(手改 "10"),normalize 时强转,转不动则删除
  maxOutputTokens?:  number   同上
  temperature?:      number   同上(UI 未暴露但 schema 合法)
  supportsToolCall?:   bool
  supportsImages?:     bool
  supportsReasoning?:  bool
  onlyReasoning?:      bool
  useCustomProtocol?:  bool
  disabled?:           bool   缺省 false(normalizeCustomModel 内存补 false)
  tags?:               string[](自动补 'custom',ModelMeter 不编辑不展示)
  reasoning?: {
    defaultEffort?:      string   // 读取兜底:legacy 键 reasoning.effort
    supportedEfforts?:   string[]
    canDisableThinking?: bool     // 仅 false 时才写盘(缺省即 true)
  }
  ...未知键原样保留
}
```

- 档位词表(`EXTENDED_THINKING_LEVELS`):`off | minimal | low | medium | high | xhigh | max`;校验层对 reasoning 只查"是对象",档位值不设枚举限制。
- `useCustomProtocol` 语义(连通性测试注释):false 时确保 URL 以 `/chat/completions` 结尾(自动补,仅请求期不改文件);true 时按用户填写原样请求。
- `reasoning.effort` 是 legacy 键,面板读取顺序 `defaultEffort ?? effort`;写入一律用 `defaultEffort`。

## 4. 与现有适配器的差距(本期修改依据)

现有 `workbuddy.go`(2026-09-29 4c5f938)已对齐:裸数组、id 定位、白名单 name/url/supportsToolCall/supportsImages/supportsReasoning/reasoning.defaultEffort/reasoning.supportedEfforts、apiKey 零接触、2 空格缩进原子写。

差距清单:

1. **读侧缺字段**:`disabled`、`onlyReasoning`、`useCustomProtocol`、`maxInputTokens`、`maxOutputTokens`、`temperature`、`reasoning.canDisableThinking` 均未读取;`reasoning.effort` legacy 兜底未做。
2. **写侧白名单缺字段**:同上 7 个键不可编辑。
3. **对象形态误报 4402**:`readArray` 只接受裸数组,合法的 `{models: [...]}` 文件被当作"顶层不是数组"报 4402(现有测试 `{"id": "m"}` → 4402 的断言对无 models 数组的对象仍应保持)。
4. **`canDisableThinking` 缺省语义**:文件缺省即 true(UI 表单缺省 true,仅 false 落盘),读侧需显式缺省 true 而非 false。
5. 其余(数字字段兼容字符串、坏文件兜底、availableModels)行为对齐后自然覆盖:ModelMeter 读写仅白名单键,数字以 json.Number 零精度透传;对象形态保持后 `availableModels` 天然零丢失。

## 5. 凭据与安全

- `apiKey` 支持 WBEF1 字段级加密(策略门槛,策略下调时自动重写回明文;解不开的密文逐字节保留)。**ModelMeter 永不读写该键,明文/密文两种状态均天然安全。**
- 真实文件实测为明文 apiKey(测试夹具沿用虚构值)。
- `url`/`apiKey` 支持 `${ENV}` 环境变量展开(内存行为,不改文件);ModelMeter 写回不展开、不改写,保持原样即可。
- 日志红线:沿用现有 `patchChangedKeys` 只记键名。

## 6. 默认模型能力

WorkBuddy 无"默认模型"概念(内置 auto/fast/balanced/deep 是另一套 tier 机制,不落 models.json),`SupportsDefaultModel` 恒 false、不实现 `DefaultModelSetter` 的现状**维持不变**。

## 7. 前端影响

`AgentModelCard.vue` 按 `snapshot.columns` 通用渲染(text/select/number/bool/list 全部已支持,ZCode 列已在用),**本期零前端改动**。
