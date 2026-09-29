# CodeBuddy 模型配置适配

## Goal

新增 CodeBuddy 的模型配置适配器:读写 ~/.codebuddy/models.json,白名单字段编辑、凭据零接触、写回零丢失,对齐真实程序 schema

## Background

ModelMeter 已支持 ZCode、WorkBuddy 两个本机 Agent 工具的模型配置读写。
CodeBuddy(Tencent,与 WorkBuddy 同源代码的另一发行形态)同样把自定义模型
写在 `~/.codebuddy/models.json`,schema 勘探结论见 `research.md`。按既有
扩展点(agents 子包 + init 自注册)新增第三个适配器,前端零改动。

## Requirements

1. 新增 `CodeBuddyAgent`(`internal/service/agentconf/agents/codebuddy.go`):
   - Name `codebuddy`,DisplayName `CodeBuddy`,目标文件
     `~/.codebuddy/models.json`。
   - 读侧:对象形态顶层取 `models` 数组映射为模型清单;凭据(apiKey)
     绝不进入任何输出结构;缺省语义对齐 CodeBuddy 运行时(见 research.md
     第 6 节)。
   - 写侧:白名单批量局部修改,一次备份、树编辑、原子写回;白名单外的键
     报 4403,定位不存在的模型报 4404;id、vendor、apiKey、reasoning 内
     未涉及键、availableModels 及其他未知键原样保留。
   - 白名单:name、url、disabled、supports_tool_call、supports_images、
     supports_reasoning、only_reasoning、default_effort、supported_efforts、
     can_disable_thinking、max_input_tokens、max_output_tokens、temperature。
     (无 use_custom_protocol——CodeBuddy 无此键。)
   - 顶层仅对象形态:裸数组等非法形态报 4402;对象缺 `models` 数组按空
     清单处理(对齐程序 `buildResponse`),此时任何 patch 定位必然 4404。
   - 无"默认模型"概念:能力位恒 false,不实现 DefaultModelSetter。
2. 在 `agents/register.go` 注册;前端按 Columns/Fields 通用渲染,零改动。
3. 测试对齐 `workbuddy_test.go` 的覆盖口径(清单/脱敏、列描述、白名单写回
   零丢失、非法输入 4403/4404/4402、空 patch 不落盘、能力一致性)。

## Constraints

- 安全红线:绝不读取、回传、写日志或写回 apiKey;日志只记改动键名。
- 写回保持顶层对象形态,availableModels 等键零丢失;2 空格缩进原子写。
- 复用 treeutil.go 辅助函数,不重复造轮子。
- 文档与注释用中文。

## Acceptance Criteria

- [ ] `Snapshot`/`Models`/`ApplyModels` 按上述语义工作,注册后
      `/api/agents` 自动出现 codebuddy(与 zcode、workbuddy 并列)。
- [ ] 真实文件(本机 ~/.codebuddy/models.json)读出的清单字段正确、
      无凭据;对真实文件副本执行 patch 后字段生效且其余内容零丢失。
- [ ] 裸数组顶层、非法 JSON 报 4402;文件缺失报 4404(Snapshot 降级
      not_found 不报错)。
- [ ] `go build ./...`、`go vet ./...`、agents 包测试全部通过;Web 前端
      无需改动即可展示 CodeBuddy 卡片与模型表格。
