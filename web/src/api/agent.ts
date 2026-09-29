// Agent 模型配置接口封装:列表/详情/模型清单/批量写回/还原。
import { httpGet, httpPost, httpPut } from './http'

import type { AgentModelEntry, AgentModelPatch, AgentSnapshot } from '@/types/agent'

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

/** 从最近一份备份还原配置文件,返回还原后的最新视图。 */
export function restoreAgent(name: string): Promise<AgentSnapshot> {
  return httpPost<AgentSnapshot>(`/api/agents/${encodeURIComponent(name)}/restore`)
}
