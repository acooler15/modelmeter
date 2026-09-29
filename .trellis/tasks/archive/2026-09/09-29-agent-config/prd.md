# Agent 模型配置管理

## Goal

P1-6:读取并展示本机 Agent 工具(ZCode、WorkBuddy)当前的模型配置,支持在界面修改后写回;修改前自动备份、支持一键还原;保留扩展点便于接入其他 Agent 工具。

## Requirements

### 功能

1. `GET /api/agents`:列出支持的 Agent 工具及状态(是否找到配置文件、当前模型配置摘要)。
2. `GET /api/agents/:name`:返回当前模型配置的可编辑视图;Key 类字段脱敏。
3. `PUT /api/agents/:name`:写回修改;**写回前自动备份原文件**到 `data/agent-backups/<name>/<时间戳>.bak`,每工具保留最近 10 份,超限自动清理。
4. `POST /api/agents/:name/restore`:从最近一份备份还原(带二次确认)。
5. 适配目标(本机 Windows;实现前先探明真实路径与格式,记录进 design):
   - ZCode:`~/.zcode/` 下配置(具体文件实现时确认)
   - WorkBuddy:本机配置文件(路径与格式实现时确认)
   - JSON 配置做结构化局部更新(只改模型相关键,其余字段原样保留);非 JSON 按文本最小替换处理并在界面说明。
6. 扩展点:`service/agentconf` 定义 Agent 接口(Name、Locate、Read、Write),注册表机制,新增工具只需注册实现。
7. 错误中文提示(4xxx 段):不支持的名称(4401)、配置文件不存在(4404)、读写或备份失败(4402)。

### 非功能

8. 敏感字段(API Key 等)脱敏展示、不写日志;写回时若用户留空敏感字段则沿用原值。
9. 全中文界面;`go vet` / `go test`;`npm run lint` / `npm run build` 通过。

## Acceptance Criteria

- [ ] 列表页展示 ZCode / WorkBuddy 配置现状;修改后写回成功且界面刷新
- [ ] 每次写回前自动生成备份;一键还原可恢复原文件;超过 10 份自动清理
- [ ] 配置文件缺失 / 非法时中文提示明确,服务不崩溃
- [ ] 扩展点以接口 + 注册表体现,新增 Agent 无需改调用方
- [ ] 敏感字段脱敏、不进日志;留空沿用原值
- [ ] 检查命令全部通过(同上)

## Notes

- 实现前先用本机真实文件做一次路径与格式勘探(结果记入本任务 design.md);无法找到 WorkBuddy 配置时,以占位实现 + 明确「未找到配置文件」状态交付,不阻塞任务。
- 本任务不读 SQLite(配置是本机文件);备份目录归 DATA_DIR 管。
