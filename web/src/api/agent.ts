// Agent 模型配置接口封装:列表/详情/写回/还原。
import { httpGet, httpPost, httpPut } from './http'

import type { AgentSnapshot } from '@/types/agent'

/** 列出支持的 Agent 工具及配置现状(含未找到项)。 */
export function listAgents(): Promise<AgentSnapshot[]> {
  return httpGet<AgentSnapshot[]>('/api/agents')
}

/** 获取单个 Agent 的配置视图。 */
export function getAgent(name: string): Promise<AgentSnapshot> {
  return httpGet<AgentSnapshot>(`/api/agents/${encodeURIComponent(name)}`)
}

/** 写回修改;values 为字段 key 到新值的映射,后端写回前自动备份原配置。 */
export function applyAgent(name: string, values: Record<string, string>): Promise<AgentSnapshot> {
  return httpPut<AgentSnapshot>(`/api/agents/${encodeURIComponent(name)}`, { values })
}

/** 从最近一份备份还原配置文件,返回还原后的最新视图。 */
export function restoreAgent(name: string): Promise<AgentSnapshot> {
  return httpPost<AgentSnapshot>(`/api/agents/${encodeURIComponent(name)}/restore`)
}
