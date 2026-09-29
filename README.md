# ModelMeter

一站式 LLM 接口管理与模型测试工作台。

## 项目简介

ModelMeter 用于集中管理多个 LLM 服务接口(Base URL + API Key,可持久化保存),
基于已保存的配置拉取可用模型列表,并以多种协议对模型发起实测;同时提供
模型费率查询与本地 Agent 工具(ZCode、WorkBuddy 等)的模型配置管理能力。
所有功能通过本地 Web 界面操作,数据保存在本地。

适合需要同时对接多家/多个中转 LLM 服务、频繁验证模型可用性与质量的开发者。

## 功能需求

### P0 核心功能(MVP)

#### 1. 接口配置管理

- 添加 / 编辑 / 删除接口配置,每项包含:名称、Base URL、API Key。
- 配置持久化保存(本地 SQLite),支持保存多份并在测试时切换。
- API Key 在界面上脱敏显示,不写入日志。

#### 2. 模型列表

- 使用已保存的配置调用 OpenAI 兼容的 `/v1/models` 接口,拉取并展示模型列表。
- 支持手动刷新;展示模型 ID 及接口返回的元信息。
- 拉取失败时给出明确的中文错误提示,并区分网络错误与鉴权错误。

#### 3. 模型测试

- 对选中模型发起对话式测试,同一模型可任选以下三种协议之一:
  - OpenAI Chat Completions(`POST /v1/chat/completions`)
  - OpenAI Responses(`POST /v1/responses`)
  - Anthropic Messages(`POST /v1/messages`)
- 可自定义:system 提示词、user 消息、temperature / max_tokens 等常用参数、
  流式开关。
- 展示:回复全文、首字延迟与总耗时、token 用量(prompt / completion / total)。
- 保留最近的测试记录,便于横向对比不同模型。

#### 4. Web 操作界面

- 以上功能全部通过 Web 界面完成,界面文案为中文。
- 对 LLM 服务的请求一律由后端代理转发,浏览器不直连,规避 CORS 限制与
  API Key 暴露。

### P1 进阶功能

#### 5. New API 模型费率

- 对接 New API(one-api 系)网关的接口,拉取模型倍率 / 价格表并展示。
- 模型测试时按费率估算单次调用成本。

#### 6. Agent 模型配置管理

- 读取并展示本机 Agent 工具当前的模型配置,支持在界面修改后写回:
  - ZCode
  - WorkBuddy
- 修改前自动备份原配置,支持一键还原。
- 设计上保留扩展点,便于后续接入其他 Agent 工具。

### 非功能需求

- **部署**:单个静态二进制,一行命令启动,默认监听 `:8080`。
- **存储**:本地 SQLite(`data/` 目录),不依赖外部数据库。
- **安全**:API Key 仅本地保存、界面脱敏、不进日志;后续可考虑静态加密。
- **界面**:全中文。

## 技术栈

| 层 | 选型 |
|------|------|
| 后端 | Go 1.22+ / Gin / GORM / SQLite(glebarez 纯 Go 驱动,无 CGO) |
| 前端 | Vue 3(`<script setup>` + TypeScript)/ Vite / Element Plus / Pinia |
| 集成 | 前端构建产物经 `go:embed` 嵌入,单二进制交付 |

## 快速开始

环境依赖:Go 1.22+、Node.js 20+(含 npm)。

```bash
# 1. 构建前端产物(首次需先安装依赖)
cd web
npm install
npm run build
cd ..

# 2. 启动服务(单进程同时提供 API 与页面)
go run ./cmd/server
# 浏览器访问 http://localhost:8080
```

- **开发模式**:终端 A 运行 `go run ./cmd/server`,终端 B 运行 `cd web && npm run dev`;Vite 已把 `/api` 代理到 `:8080`,前端热更新无需重新编译。
- **环境变量**:`PORT`(默认 8080)、`DATA_DIR`(默认 data)、`LOG_LEVEL`(默认 info)、`LOG_FORMAT`(json/text,默认 json)。
- **检查与测试**:`go test ./...`、`go vet ./...`;前端 `cd web && npm run lint && npm run build`(含 vue-tsc 类型检查)。
- **数据**:SQLite 文件位于 `data/modelmeter.db`,已 gitignore,删除即重置。

## 文档导航

- 编码规范:`.trellis/spec/backend/`、`.trellis/spec/frontend/`(按任务自动注入给 AI 子代理)
- 功能详设:每个功能开工时创建 Trellis 任务,详细需求与验收标准写在对应任务的 `prd.md` 中
