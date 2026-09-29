# 设计:WorkBuddy 适配器对齐真实 schema

## 边界

- 只改 `internal/service/agentconf/agents/workbuddy.go` 与 `workbuddy_test.go`;`agentconf.go`、`treeutil.go`、`zcode.go`、handler、前端零改动(现有接口 `Agent`/`Snapshot`/`FieldSpec` 表达力足够)。
- 唯一操作目标 `~/.workbuddy/models.json`;备份/原子写/错误码等公共设施全部复用。

## 数据流

```
readDoc() ──► root any (json.Decoder+UseNumber)
             │  ├─ []any            形态 A:裸数组
             │  └─ map["models"][]any 形态 B:对象包装;其余顶层键(availableModels 等)不触碰
             │     └─ 两者皆非 → 4402
             ▼
workBuddyEntries(models) ──► 脱敏清单(读侧字段映射)
ApplyModels: validate(整体) → Backup → 逐 patch 树编辑 → WriteJSONFile(root 保形态) → Models
```

## 关键决策

1. **形态抽象最小化**:不引入 doc 结构体。`readDoc` 返回 `root any`;新增辅助 `workBuddyModelsOf(root) ([]any, error)`(数组→root;对象→`root["models"]` 必须是数组否则 4402)。写回时若 root 是 map,把(可能因 append 扩容的)数组回填 `root["models"]`,再整体 `WriteJSONFile(root)`——顶层未知键天然零丢失,与 WorkBuddy 自身"对象重写回裸数组"的行为相比更保守。
2. **字段映射(读)**:白名单常量即映射;`disabled`/`only_reasoning`/`use_custom_protocol` 用 `boolDefault`(缺省 false);`can_disable_thinking` 显式缺省 true(`boolOf` 不命中时落 true);三个数字键命中才透出 json.Number;`default_effort` 取 `defaultEffort` 兜底 `effort`。
3. **字段映射(写)**:布尔键直接落;数字键经 `numberValue` 规范(整型不写科学计数法);`can_disable_thinking` 落 `reasoning.canDisableThinking`(节点缺失创建);`default_effort` 落 `reasoning.defaultEffort`(不清理 legacy `effort` 键,零丢失)。
4. **列描述**:新列全部加进 `workBuddyColumns`——`disabled`("禁用",bool)、`only_reasoning`("仅推理",bool)、`use_custom_protocol`("URL 原样请求",bool)、`can_disable_thinking`("可关闭思考",bool)、`max_input_tokens`("最大输入 Token",number)、`max_output_tokens`("最大输出 Token",number)、`temperature`("采样温度",number)。排序:标识/名称类 → 禁用 → 能力开关组 → 推理组 → 高级数字组 → vendor(只读)。
5. **错误信息**:`readDoc` 对"非 JSON 文本"维持 4402 文案;对象形态无 models 数组给独立文案("顶层对象缺少 models 数组"),避免用户把合法形态当坏文件。

## 兼容与回滚

- 裸数组文件行为与改造前完全一致(测试全量保留并扩展);对象形态此前直接 4402,属纯增益。
- 单包单文件改动,回滚即 revert 该 commit;备份机制(`agent-backups/workbuddy/`)在写回前自动留底,用户侧可还原。
