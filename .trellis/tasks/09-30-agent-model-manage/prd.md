# Agent 配置页 tab 化与模型清单管理增强

## Goal

Agent 配置页改为每个 Agent 单独 tab;支持编辑/删除单个模型配置与批量删除;支持将接口的全部或指定模型添加到 Agent 模型清单。

## 背景与现状

- Agent 配置页(`web/src/views/agent/AgentView.vue`)目前是卡片纵向列表,每个 Agent 一张 `AgentModelCard`。
- 模型表格已支持行内草稿编辑 + 统一保存(`PUT /api/agents/:name/models` 白名单 patch),但**没有删除、没有新增**;后端 `agentconf` 的 Agent 接口只有 `ApplyModels`(修改)能力。
- 三个适配器的写侧现状(依据归档勘探 `09-29-agentconf-realign`、`09-30-workbuddy-schema-realign`、`09-30-codebuddy-agent-conf`):
  - ZCode(`~/.zcode/v2/provider_config.json`):模型清单来自供应商 `config.modelOrder`(显示顺序)∪ `config.personalModelIds`(自定义模型清单);个人供应商节点含 `config.access={type:"api-key", apiKey}` 与 `config.api={type:"openai-chat-completions"|"openai-responses", baseUrl}`;ZCode 自身 UI 新建供应商的 providerId 惯例为 `new-provider`、`new-provider-2`…递增;根对象与 config 有 strict 校验,未知键会导致拒载。
  - WorkBuddy(`~/.workbuddy/models.json`):平铺数组(或兼容对象形态),每个条目自带明文 `apiKey`,**`url` 是完整 endpoint(含路径,如 `https://api.deepseek.com/chat/completions`),不是 baseUrl**;对象形态顶层 `availableModels` 按 id 过滤可用模型。
  - CodeBuddy(`~/.codebuddy/models.json`):仅对象形态 `{"models": [...]}`,条目 schema 与 WorkBuddy 同源,同样自带 `apiKey` 与完整 endpoint 语义的 `url`;顶层 `availableModels` 只影响选择器可见性。
- 三个适配器的包注释均声明"模型条目的新增/删除与 apiKey 管理一律引导用户到原生工具操作"——本任务按用户要求突破"新增/删除"部分,凭据边界见决策 D1/D2。

## 需求决策(已经用户确认)

- **D1 凭据写入**:允许在"从接口添加模型"时把所选接口(ModelMeter provider 记录)的 API Key 写入 WorkBuddy/CodeBuddy 的新增条目(不写入则新条目不可用);写入前弹窗明确警示;界面、日志、API 响应永不回显明文 key。对**既有条目**的凭据仍零接触(不读取、不回传、不修改)。
- **D2 ZCode 落点**(用户确认:支持新建供应商):支持两种模式——
  - **挂已有供应商**:模型 id 追加到所选供应商的 `personalModelIds`,不写任何凭据;
  - **新建供应商**:按 ZCode 自身惯例自动生成 providerId,写入最小合法个人供应商节点(含接口的 API Key,属 D1 显式例外);既有供应商的改名/换 key/删除仍归 ZCode 原生工具。
- **D3 编辑交互**:保留现有行内编辑;新增"操作"列(每行"编辑"按钮弹窗集中编辑该模型全部可编辑字段 + "删除"按钮);表头多选行 + "批量删除"。
- **D4 重复处理**:添加时目标清单已存在的 model_id 逐条跳过,结果中汇报"成功 N 个、跳过 M 个"。

## Requirements

### R1 Agent 配置页 tab 化

- 页面改为 tab 布局,每个 Agent 一个 tab(tab 标题为 Agent 展示名),tab 内功能与现有卡片一致。
- 未找到配置文件的 Agent 同样保留 tab,展示现有指引文案,不渲染表格与增删改入口。

### R2 模型配置的编辑与删除

- 每个模型行提供"编辑"入口:弹窗集中展示该模型全部可编辑字段(由 `Snapshot.columns` 驱动,只读列不出现),保存走现有白名单 patch 通道。
- 每个模型行提供"删除"入口:二次确认后从 Agent 配置移除该模型。
- 表格支持多选行;"批量删除"在选中数 > 0 时可用,二次确认后一次请求删除全部选中。
- ZCode 删除模型:从该供应商 `modelOrder` 与 `personalModelIds` 移除该 id,并同步删除 `providerModelRules` 中对应规则节点;`manualProviderModelRules` 不动;**不删除供应商节点**(即使其清单变空)。
- WorkBuddy/CodeBuddy 删除模型:从模型数组移除该条目;对象形态下顶层 `availableModels` 若含该 id 则同步移除。
- 所有删除动作:一次请求一次备份一次写回;先整体校验(定位不存在报 4404)再落盘,避免半批生效;成功后刷新清单。

### R3 从接口添加模型

- 卡片(tab 内)提供"从接口添加模型"入口,弹窗流程:
  1. 选择接口(ModelMeter 已配置的 provider);
  2. 拉取该接口的模型列表(复用现有模型列表通道);
  3. 勾选要添加的模型(支持全选=该接口的全部模型);
  4. 确认接口地址(预填该接口 Base URL,可修改)。WorkBuddy/CodeBuddy 实际写入的是由它派生的**完整 endpoint**(`/v1` 去重后拼 `/chat/completions`,弹窗内提示派生结果;url 在模型表格中仍可后续编辑);ZCode 新建供应商写入的是该 Base URL 本身;
  5. ZCode 需选择落点:已有供应商,或"新建供应商"——显示名预填接口名(可改),API 协议二选一(`openai-chat-completions` 默认 / `openai-responses`);
  6. WorkBuddy/CodeBuddy 与 ZCode 新建供应商模式的确认弹窗均含凭据写入警示(见 D1/D2)。
- 添加行为:
  - WorkBuddy/CodeBuddy:新增条目 `{id, name: 模型id, vendor: 接口名, url: 派生endpoint, apiKey}`,追加到模型数组尾部;对象形态下顶层 `availableModels` 存在且不含该 id 时追加(不存在则不创建)。
  - ZCode 挂已有供应商:模型 id 按输入顺序追加到该供应商 `personalModelIds`(数组缺失时创建;已存在则跳过);不写凭据,不创建规则节点。
  - ZCode 新建供应商:providerId 沿用 ZCode 惯例自动生成(`new-provider`、`new-provider-2`…取最小未用序号),追加到 `config.providerOrder`,并在 `providerRules` 追加最小合法节点:`providerId`、`providerName`(用户可改,空则回退接口名)、`enabled=true`、`config.group="standard-personal"`、`config.access={type:"api-key", apiKey:接口Key}`、`config.api={type:所选协议, baseUrl:接口地址}`、`config.modelOrder=[]`、`config.personalModelIds=新增模型id`;**不使用 templateId、不写节点外任何键**(strict 校验防拒载)。
  - 已存在的 model_id 逐条跳过(D4);全部被跳过时不备份、不落盘。
- 能力位:`Snapshot` 新增"支持添加/支持删除模型"能力位与 ZCode 挂靠候选清单,前端按位显隐入口;配置文件未找到时不显示任何增删改入口。

## 安全红线(细化后全量口径)

1. **既有凭据零接触**(不变):不读取、不回传、不修改任何既有条目的凭据字段(ZCode `access.apiKey`、WorkBuddy/CodeBuddy 条目 `apiKey`);日志不出现任何凭据内容。
2. **新增写入凭据**(本任务显式例外,仅限 R3 路径):凭据来源为 ModelMeter 自身数据库的 provider 记录,由后端装配,不出现在任何 GET 响应中;WorkBuddy/CodeBuddy 新增条目与 ZCode **新建供应商**写入凭据;ZCode **挂已有供应商**永不写凭据。
3. `availableModels` 仅在添加(存在则追加)与删除(存在则移除)时按 id 同步,不做其他改写。
4. 所有写回(编辑/删除/添加)沿用"先备份、树最小化编辑、原子写回、未知键零丢失"既有机制;ZCode 新建供应商节点只含已知键,防 strict 拒载。

## Acceptance Criteria

- [ ] Agent 配置页按 Agent 分 tab,tab 内能力与原卡片一致;未找到配置的 Agent 有 tab 且只有指引
- [ ] 每行可弹窗编辑全部可编辑字段(Columns 驱动),保存走白名单 patch;行内编辑保持可用
- [ ] 单个删除与批量删除均可用:二次确认 → 一次请求 → 清单刷新;ZCode 规则节点同步清理且不动供应商节点,WorkBuddy/CodeBuddy 对象形态 availableModels 同步移除
- [ ] 删除定位不存在的模型整批拒绝(4404),不半批生效
- [ ] "从接口添加模型"全流程可用:选接口 → 拉模型 → 全选/勾选 → (ZCode 选落点)→ 确认 → 成功 N/跳过 M 反馈 → 清单刷新
- [ ] WorkBuddy/CodeBuddy 新增条目带完整 endpoint url/apiKey/vendor 且开箱可用;url 由 Base URL 归一派生(`/v1` 不重复拼接)
- [ ] ZCode 挂已有供应商:不写凭据,供应商不存在报 4404;ZCode 新建供应商:providerId 自动命名且唯一、api.type 取值合法(否则 4403)、节点为最小合法形状,ZCode 可正常加载
- [ ] 全部跳过时不产生新备份、不落盘
- [ ] 凭据红线:任何响应、日志不出现明文 key;编辑/删除/添加不丢失既有条目的 apiKey/vendor/未知键;UseNumber 数字零精度丢失
- [ ] `go vet ./...`、`go test ./...` 通过;前端 `npm run lint`、`npm run build`(含 vue-tsc)通过
- [ ] 三个适配器的新增/删除均有单测覆盖(两形态、availableModels 同步、ZCode 两种落点、manual 冲突守护、4404/4403、未知键保留);handler 有路由级测试

## Out of Scope

- ZCode 既有供应商的改名/换 key/删除/停用(仍归 ZCode 原生工具;新建在本任务范围内)。
- 接口(provider)本身的增删改(已有功能,本任务只读)。
- 配置文件不存在时的"创建配置"能力(not_found 只展示指引)。
