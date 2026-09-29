# agentconf 实现下沉子目录 + ZCode/WorkBuddy 配置对齐真实文件

## Goal

修正 agentconf 包的两个定位错误并理顺结构:① 实现代码统一移入唯一子目录 `agents/`,不再与接口/备份文件平级;② ZCode 适配目标从废弃的 cli-bin 文件重写为 `~/.zcode/v2/provider_config.json`,只管理模型配置本身(不做"当前选中模型");③ WorkBuddy 占位实现替换为 `~/.workbuddy/models.json` 的真实读写;文档随码同步。

## Requirements

### 功能

1. **目录调整**:`internal/service/agentconf/` 根目录只保留 Agent 接口+注册表(agentconf.go)与备份还原(backup.go);zcode.go、workbuddy.go 及对应测试统一移入唯一子目录 `agents/`(包名 `agents`)。注册从根包 init 改为子包 init + main 空导入触发(避免循环导入),handler 与前端 API 路径除下述清单端点外不变。
2. **接口演化**:现有"单值动态表单"(Snapshot.Fields/Values + Apply(values))无法表达清单型配置,且两个实现均为清单型——Agent 接口改为模型清单形态:
   - `Snapshot` 保留 Name/DisplayName/Status/ConfigPath/Message,移除 Fields/Values,新增 Columns(列描述,复用 FieldSpec);
   - 新增 `Models(ctx) ([]ModelEntry, error)`:返回模型清单(凭据脱敏),ModelEntry 含 provider 维度(ZCode 有、WorkBuddy 无)、modelId、展示名与非凭据字段;
   - `ApplyModels(ctx, patches []ModelPatch, dataDir) ([]ModelEntry, error)`:白名单局部修改,写回前备份,原子写,返回最新清单。
3. **ZCode 适配重写**(目标 `~/.zcode/v2/provider_config.json`):
   - 读:供应商清单(id/名称/协议/baseUrl)与模型清单(合并 modelOrder 与 personalModelIds,保序去重),模型规则(enabled、contextWindow、maxOutputTokens.max 等)并入条目;无规则节点的模型照常列出(可编辑字段取缺省);
   - 写(白名单):模型规则的 enabled、properties.contextWindow、optionSpecs.maxOutputTokens.max;规则节点不存在时允许创建最小节点;
   - 不做:新增/删除供应商、修改 apiKey/baseUrl/协议——界面文案引导到 ZCode 原生工具管理;
   - 删除旧"切换当前模型"(cli-bin)全部逻辑。
4. **WorkBuddy 实装**(目标 `~/.workbuddy/models.json`):
   - 读:模型清单(id/name/vendor/url/能力开关/reasoning);apiKey 不读不传;
   - 写(白名单):name、url、supportsToolCall、supportsImages、supportsReasoning、reasoning.defaultEffort;apiKey、id、vendor 及未知字段逐条原样保留;
   - 不做:新增/删除模型条目(后续任务可扩)。
5. **API**:
   - `GET /api/agents/:name/models`:返回模型清单(新增);
   - `PUT /api/agents/:name/models`:批量白名单修改,body `{patches:[...]}`,返回最新清单(新增);
   - `PUT /api/agents/:name`:删除(表单写回废弃);
   - `GET /api/agents`、`GET /api/agents/:name`、`POST /api/agents/:name/restore`:保留(payload 瘦身);
   - 错误码沿用 4401(未知名称)/4402(文件 IO 或非法 JSON)/4403(提交值非法,含白名单外键)/4404(配置文件不存在),中文提示。
6. **前端**:Agent 配置页改为"卡片 + 模型表格":卡片展示状态/路径/说明;表格列由 Columns 驱动(开关/数字/下拉/文本),批量保存、写回后刷新并提示"建议重启对应工具使配置生效";全中文。
7. **文档同步**(随码,不后补):README 第 6 节、agentconf 与 agents 包注释、`.trellis/spec/backend/quality-guidelines.md` 中涉及 agentconf 旧行为的表述。

### 非功能

8. **安全红线**:两份文件均含明文 apiKey——绝不读取、回传、写日志、写回;白名单外的任何键(含 apiKey 与未知键)逐字节原样保留;写回前自动备份(沿用 backup.go,10 份滚动),原子写(临时文件+改名)。
9. 解析容忍:残缺条目(ZCode 的空供应商、缺 apiKey/api 节点)、缺失的可选字段、未知键全部无损保留。
10. 写回时若目标工具正在运行,其内存态可能在退出时覆盖外部修改——界面提示重启生效。
11. `go vet` / `go test ./...`;`npm run lint` / `npm run build`(web/)通过。

## Acceptance Criteria

- [ ] `agents/` 子目录承载两个实现(含测试),agentconf 根仅接口+注册表+backup;`go build ./...` 通过,handler 未感知目录变化
- [ ] ZCode 模型清单来自 `~/.zcode/v2/provider_config.json`;对本机真实文件执行一次写回后,除白名单字段外内容语义零变化(测试用含 apiKey 与未知键的 fixture 证明)
- [ ] WorkBuddy 模型清单来自 `~/.workbuddy/models.json`;写回后每条记录的 apiKey 与未知键原样保留(fixture 测试证明)
- [ ] 批量写回前自动备份、一键还原可用;错误 4401/4402/4403/4404 中文提示
- [ ] 前端模型表格可编辑并保存成功;全中文界面
- [ ] README、包注释、spec 中 agentconf 相关表述已同步;不出现"切换当前模型""cli-bin"残留描述
- [ ] 检查命令全部通过(同非功能第 11 条)

## Notes

- "当前选中模型"明确出局:ZCode v2 文件中没有该字段,用户也不需要。
- 勘探结论(两份文件完整结构、安全红线)见 research.md;技术方案(接口演化、树编辑写回、注册方式)见 design.md。
- P1-6 归档任务:`.trellis/tasks/archive/2026-09/09-29-agent-config/`。
