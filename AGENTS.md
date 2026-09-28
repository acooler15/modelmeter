<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

# 项目速览

- **项目**:ModelMeter —— LLM 接口管理与模型测试工作台:管理 Base URL / API Key(可持久化),拉取模型列表,通过 OpenAI Chat Completions / OpenAI Responses / Anthropic Messages 三种协议实测模型,对接 New API 查询模型费率,并支持读取/设置 ZCode、WorkBuddy 等本地 Agent 工具的模型配置;提供全中文 Web 界面。功能需求详见 `README.md`。
- **技术栈**:后端 Go(Gin + GORM + SQLite,纯 Go 驱动),前端 Vue 3 + TypeScript + Vite + Element Plus + Pinia;前端构建产物经 `go:embed` 嵌入,单二进制部署。
- **语言约定**:文档与代码注释一律用中文,代码标识符用英文,界面文案用中文。
- **编码规范**:分层、命名、错误处理、日志等详细约定在 `.trellis/spec/backend/` 与 `.trellis/spec/frontend/`,按任务自动注入,写代码前遵循。
- **功能开发流程**:新功能先创建 Trellis 任务并在任务 `prd.md` 中写清需求,再进入实现;不要在本文件或 README 中堆功能细节。
