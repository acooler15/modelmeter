// agent.go Agent 模型配置处理器:列表/详情/模型清单/批量写回/默认模型设置/还原。
// 不依赖数据库;备份目录由 main 装配时注入的 dataDir 决定,
// 具体的读写与备份逻辑全部在 internal/service/agentconf 及其 agents/ 子包。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/handler/response"
	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// agentByName 按路径参数取已注册 Agent;未知名称统一报 4401。
func agentByName(c *gin.Context) (agentconf.Agent, bool) {
	a, ok := agentconf.Get(c.Param("name"))
	if !ok {
		response.Fail(c, apperr.New(apperr.CodeAgentUnknown, "不支持的 Agent 名称"))
		return nil, false
	}
	return a, true
}

// AgentList GET /api/agents 列出全部支持的 Agent 工具及配置现状(含未找到项)。
func AgentList() gin.HandlerFunc {
	return func(c *gin.Context) {
		registered := agentconf.List()
		snaps := make([]agentconf.Snapshot, 0, len(registered))
		for _, a := range registered {
			snaps = append(snaps, a.Snapshot(c.Request.Context()))
		}
		response.OK(c, snaps)
	}
}

// AgentGet GET /api/agents/:name 单个 Agent 配置视图;未知名称报 4401。
func AgentGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := agentByName(c)
		if !ok {
			return
		}
		response.OK(c, a.Snapshot(c.Request.Context()))
	}
}

// AgentModelsGet GET /api/agents/:name/models 返回模型清单(凭据脱敏);
// 配置文件缺失报 4404,非法 JSON 报 4402。
func AgentModelsGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := agentByName(c)
		if !ok {
			return
		}
		entries, err := a.Models(c.Request.Context())
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, entries)
	}
}

// AgentModelsApply PUT /api/agents/:name/models 批量白名单修改模型配置。
// body 形如 {"patches": [{"provider_id": "...", "model_id": "...", "fields": {...}}]};
// 实现内部先备份再原子写回,返回写回后的最新清单。
func AgentModelsApply(dataDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := agentByName(c)
		if !ok {
			return
		}
		var req struct {
			Patches []agentconf.ModelPatch `json:"patches"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, apperr.New(apperr.CodeAgentInvalid, "请求参数格式错误"))
			return
		}
		if req.Patches == nil {
			req.Patches = []agentconf.ModelPatch{} // 空提交交由实现返回现状清单
		}
		entries, err := a.ApplyModels(c.Request.Context(), req.Patches, dataDir)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, entries)
	}
}

// AgentDefaultModelSet PUT /api/agents/:name/default-model 设置/清除默认模型。
// 仅实现 DefaultModelSetter 能力接口的 Agent 可用(当前为 ZCode),未实现的
// 工具报 4405;body 即 DefaultModelPatch(provider_id+model_id 均空=清除),
// 成功返回写回后的最新 Snapshot。
func AgentDefaultModelSet(dataDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := agentByName(c)
		if !ok {
			return
		}
		// 能力探测:按类型断言判断工具是否支持默认模型,未实现报 4405
		setter, supported := a.(agentconf.DefaultModelSetter)
		if !supported {
			response.Fail(c, apperr.New(apperr.CodeAgentUnsupported, "该工具不支持设置默认模型"))
			return
		}
		var patch agentconf.DefaultModelPatch
		if err := c.ShouldBindJSON(&patch); err != nil {
			response.Fail(c, apperr.New(apperr.CodeAgentInvalid, "请求参数格式错误"))
			return
		}
		snap, err := setter.ApplyDefaultModel(c.Request.Context(), patch, dataDir)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, snap)
	}
}

// AgentRestore POST /api/agents/:name/restore 从最近一份备份还原配置文件;
// 无备份或配置路径未知时报 4404,成功后返回还原完成的最新视图。
func AgentRestore(dataDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := agentByName(c)
		if !ok {
			return
		}
		// 还原目标路径取自 Agent 自身;未找到配置的工具 config_path 为空,无从还原
		snap := a.Snapshot(c.Request.Context())
		if snap.ConfigPath == "" {
			response.Fail(c, apperr.New(apperr.CodeAgentNotFound, "未找到配置文件,无法还原"))
			return
		}
		if _, err := agentconf.RestoreLatest(snap.ConfigPath, a.Name(), dataDir); err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, a.Snapshot(c.Request.Context()))
	}
}
