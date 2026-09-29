# design.md — CodeBuddy 模型配置适配

## 边界与复用

- 新文件仅 `internal/service/agentconf/agents/codebuddy.go` +
  `codebuddy_test.go`;`register.go` 加一行注册。handler、service 根包、
  前端全部零改动(Agent 接口 + Columns/Fields 通用渲染已覆盖)。
- 结构复刻 `workbuddy.go`(同源 schema,实现形态最接近),差异点显式化:
  - 路径:`~/.codebuddy/models.json`。
  - 顶层形态:**仅对象**。`codeBuddyModelsOf(root)`:root 非对象 → 4402
    (对齐程序 `Invalid models.json format`);对象取 `models` 键,缺失或
    非数组返回空切片 + ok=false 语义合并——按 research 第 6 节结论,缺失
    按空清单处理(对齐 `buildResponse`),仅"顶层非对象"报错。
  - 白名单:**去掉 `use_custom_protocol`**,其余同 WorkBuddy。

## 数据结构映射

```
Column key             → 文件键                                类型/缺省
model_id               → id                                    text 只读(无字符串 id 的条目跳过)
name                   → name                                  text
url                    → url                                   text
disabled               → disabled                              bool(缺省 false)
supports_tool_call     → supportsToolCall                      bool(缺省 false)
supports_images        → supportsImages                        bool(缺省 false)
supports_reasoning     → supportsReasoning                     bool(缺省 false)
only_reasoning         → onlyReasoning                         bool(缺省 false)
default_effort         → reasoning.defaultEffort(兜底 effort) text/select(选项=supportedEfforts 全文件并集)
supported_efforts      → reasoning.supportedEfforts            list
can_disable_thinking   → reasoning.canDisableThinking          bool(缺省 true)
max_input_tokens       → maxInputTokens                        number(文件未写不出现)
max_output_tokens      → maxOutputTokens                       number(同上)
temperature            → temperature                           number(同上)
vendor                 → vendor                                text 只读
```

`api_key` 不设列、不读取;`availableModels` 与其余未知键零接触。

## 关键流程

- `Snapshot`:文件缺失 → not_found + 指引;found 时 Columns + 管理边界
  提示(CodeBuddy 运行时 watch 热加载,提示改为"修改后通常自动生效,
  若未生效请重启 CodeBuddy");`SupportsDefaultModel` 恒 false。
- `Models`:readDoc(UseNumber)→ codeBuddyModelsOf → 遍历映射;跳过
  非对象元素与无字符串 id 元素(对齐运行时过滤)。
- `ApplyModels`:readDoc → 取数组 → validateWorkBuddyPatches 同款整体
  校验(复用现有函数?否——其错误文案/白名单分支绑定 WorkBuddy 常量,
  CodeBuddy 独立实现同构函数,复用 treeutil)→ 空 patch 直接返回 →
  Backup(`codebuddy` 目录)→ 逐条 apply(按 id 首个匹配定位,
  reasoning 节点缺失时创建)→ `obj[models] = models` 回填 → WriteJSONFile
  → 重新 Models 返回。日志 `slog.Info` 只记 patches 数与改动键名。

## 错误码沿用

4402 文件非法(JSON/顶层非对象)、4403 白名单外/类型错、4404 文件缺失/
模型不存在;Snapshot 不返回错误。与 handler 现有约定一致,无新错误码。

## 权衡与回滚

- 为什么不做裸数组兼容:CodeBuddy 程序读裸数组得空清单、写则丢数据,
  兼容它只会制造"看似成功实则无效"的假象;拒绝(4402)更诚实。
- 为什么缺 models 键不报错:程序 `loadFile` 对 ENOENT/空文件返回 `{}`,
  `buildResponse` 对缺键返回空数组——缺键是"新建后未添加模型"的正常态;
  且空清单下 patch 必然 4404,无误写风险。
- 回滚:删除 codebuddy.go/codebuddy_test.go 与注册行即可,无数据迁移。
