# ModelMeter 功能实现(P0+P1)

## Goal

按 README《功能需求》完整实现 ModelMeter 的 P0 核心功能与 P1 进阶功能。本任务为父任务,负责需求集、任务地图、共享技术设计与最终集成验收;具体实现工作在各子任务中进行。

## 需求集(源自 README.md《功能需求》)

### P0 核心功能

1. **接口配置管理** — 添加/编辑/删除接口配置(名称、Base URL、API Key),SQLite 持久化,支持保存多份并在测试时切换;API Key 界面脱敏、不写日志。→ 子任务 `09-29-interface-config`
2. **模型列表** — 用已保存配置调用 OpenAI 兼容 `/v1/models`,拉取并展示模型 ID 与元信息;支持手动刷新;失败时给出中文提示并区分网络错误与鉴权错误。→ 子任务 `09-29-model-list`
3. **模型测试** — 对选中模型发起对话式测试,同一模型任选三种协议之一(OpenAI Chat Completions / OpenAI Responses / Anthropic Messages);可自定义 system 提示词、user 消息、temperature / max_tokens、流式开关;展示回复全文、首字延迟、总耗时、token 用量(prompt/completion/total);保留最近测试记录便于横向对比。→ 子任务 `09-29-model-test`
4. **Web 操作界面** — 以上功能全部通过全中文 Web 界面完成;对 LLM 服务的请求一律由后端代理转发,浏览器不直连(规避 CORS 与 Key 暴露)。→ 横切要求,不单列子任务,落在各子任务验收标准中

### P1 进阶功能

5. **New API 模型费率** — 对接 New API(one-api 系)网关拉取模型倍率/价格表并展示;模型测试时按费率估算单次调用成本。→ 子任务 `09-29-newapi-rates`
6. **Agent 模型配置管理** — 读取并展示本机 Agent 工具(ZCode、WorkBuddy)当前模型配置,支持界面修改后写回;修改前自动备份、支持一键还原;保留扩展点便于接入其他 Agent 工具。→ 子任务 `09-29-agent-config`

## 任务地图与推进顺序

| 顺序 | 子任务 | 依赖 |
|------|--------|------|
| 1 | `09-29-interface-config` 接口配置管理 | 无(基础层) |
| 2 | `09-29-model-list` 模型列表 | 子任务 1 |
| 3 | `09-29-model-test` 模型测试 | 子任务 1、2 |
| 4 | `09-29-newapi-rates` New API 费率 | 子任务 3(成本估算挂在测试页) |
| 5 | `09-29-agent-config` Agent 配置管理 | 无(可独立,按序推进) |

子任务按序「实现 → 检查 → 归档」;设计滚动细化:各子任务的 `design.md` / `implement.md` 于其启动前补齐,继承并遵守父任务 `design.md` 的共享决策。

## 跨子任务约束

- 全中文界面;文档与注释中文,标识符英文(项目语言约定)。
- 对 LLM 服务与 New API 的请求一律由后端代理转发,浏览器不直连。
- API Key / 令牌不写入日志;界面脱敏展示。
- 遵循 `.trellis/spec/backend/` 与 `.trellis/spec/frontend/` 全部规范。
- API 路由总表、错误码分段、数据模型、上游代理客户端约定以父任务 `design.md` 登记为准,子任务不得与其冲突;新增条目先回父任务登记。

## 跨子任务验收标准(集成验收,父任务收尾逐项核验)

- [ ] 五个子任务全部完成并归档
- [ ] `go vet ./...`、`go test ./...` 通过;`cd web && npm run lint && npm run build` 通过
- [ ] `go run ./cmd/server` 一行启动后,浏览器走通完整用户旅程:添加接口配置 → 拉取模型列表 → 三协议各测一次(含流式)→ 查看测试记录 → 配置 New API 并查看费率 → 修改 ZCode 模型配置并还原
- [ ] SQLite 数据落盘 `data/modelmeter.db`,重启后接口配置与测试记录仍在
- [ ] 全程日志与页面无 API Key 明文
- [ ] 非功能需求达标:单二进制、默认 `:8080`、全中文界面

## Notes

- README 中「后续可考虑」的能力(API Key 静态加密等)不在本期范围。
- 各子任务归档前必须通过自身验收标准;父任务在全部归档后做集成回归再收尾。
