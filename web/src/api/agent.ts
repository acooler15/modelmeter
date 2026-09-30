// Agent 模型配置接口封装:列表/详情/模型清单/批量写回/增删模型/默认模型设置/还原。
import { httpGet, httpPost, httpPut } from './http'

import type {
  AgentAddModelsPayload,
  AgentAddModelsResult,
  AgentDefaultModelPatch,
  AgentModelEntry,
  AgentModelPatch,
  AgentModelRef,
  AgentSnapshot,
} from '@/types/agent'

/** 列出支持的 Agent 工具及配置现状(含未找到项)。 */
export function listAgents(): Promise<AgentSnapshot[]> {
  return httpGet<AgentSnapshot[]>('/api/agents')
}

/** 获取单个 Agent 的配置视图。 */
export function getAgent(name: string): Promise<AgentSnapshot> {
  return httpGet<AgentSnapshot>(`/api/agents/${encodeURIComponent(name)}`)
}

/** 获取模型清单(凭据脱敏);配置文件缺失报 4404,非法 JSON 报 4402。 */
export function listAgentModels(name: string): Promise<AgentModelEntry[]> {
  return httpGet<AgentModelEntry[]>(`/api/agents/${encodeURIComponent(name)}/models`)
}

/** 批量白名单修改模型配置;后端写回前自动备份,返回写回后的最新清单。 */
export function updateAgentModels(
  name: string,
  patches: AgentModelPatch[],
): Promise<AgentModelEntry[]> {
  return httpPut<AgentModelEntry[]>(`/api/agents/${encodeURIComponent(name)}/models`, { patches })
}

/** 批量删除模型;后端先整体校验(定位不存在报 4404 整批拒绝)再一次备份一次写回,返回最新清单。 */
export function removeAgentModels(
  name: string,
  targets: AgentModelRef[],
): Promise<AgentModelEntry[]> {
  return httpPost<AgentModelEntry[]>(
    `/api/agents/${encodeURIComponent(name)}/models/remove`,
    { targets },
  )
}

/**
 * 从接口添加模型:后端按 provider_id 从接口记录装配来源(含凭据),凭据
 * 不经过前端、不进响应;已存在的 model_id 逐条跳过,返回最新清单与成败明细。
 */
export function addAgentModels(
  name: string,
  payload: AgentAddModelsPayload,
): Promise<AgentAddModelsResult> {
  return httpPost<AgentAddModelsResult>(
    `/api/agents/${encodeURIComponent(name)}/models/add`,
    payload,
  )
}

/**
 * 设置/清除默认模型(patch 均空=清除);仅支持该能力的工具可用,未实现报 4405。
 * 后端写回前自动备份,返回写回后的最新视图。
 */
export function setDefaultAgentModel(
  name: string,
  patch: AgentDefaultModelPatch,
): Promise<AgentSnapshot> {
  return httpPut<AgentSnapshot>(`/api/agents/${encodeURIComponent(name)}/default-model`, patch)
}

/** 从最近一份备份还原配置文件,返回还原后的最新视图。 */
export function restoreAgent(name: string): Promise<AgentSnapshot> {
  return httpPost<AgentSnapshot>(`/api/agents/${encodeURIComponent(name)}/restore`)
}
